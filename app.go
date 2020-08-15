package main

import (
	"fmt"
	"github.com/MissGod1/go-tun2socks/core"
	"github.com/google/gopacket/layers"
	"sync"

	"github.com/google/gopacket"
	"github.com/MissGod1/PProxy/windivert"
	shadow "github.com/imgk/shadow/utils"
)

type App struct {
	pids      map[uint32]bool  	// pid列表
	sessions  *sync.Map   		// session列表
	processes map[string]bool	// 进程列表
	whitelist map[string]bool	// 域名列表
	lwip core.LWIPStack

	hSocket  *windivert.Handle
	hNetwork *windivert.Handle
	hReplay  *windivert.Handle
}

const (
	Filter1 = "outbound and !ipv6 and !loopback and event == CONNECT"
	Filter2 = "outbound and !ipv6 and !loopback and (tcp or udp) and remoteAddr != %v"
	Filter3 = "false"
)

func NewApp(_server *Server, _process *Process) *App {
	iface, subiface, err := windivert.GetInterfaceIndex()
	h1, err := windivert.Open(Filter1, windivert.LayerSocket, 100, windivert.FlagSniff | windivert.FlagRecvOnly)
	if err != nil {
		panic("Open Socket Handle Failed.")
	}
	h2, err := windivert.Open(fmt.Sprintf(Filter2, _server.Server), windivert.LayerNetwork, 101, 0)
	if err != nil {
		panic("Open Network Handle Falied.")
	}

	h3, err := windivert.Open(Filter3, windivert.LayerNetwork, 102, windivert.FlagSendOnly)
	if err != nil {
		panic("Open Network Handle Falied.")
	}
	processes := make(map[string]bool)
	for _,v := range _process.Processes {
		processes[v] = true
	}

	whitelist := make(map[string]bool)
	for _,v := range _process.Whitelist {
		whitelist[v] = true
	}

	core.RegisterOutputFn(func(bytes []byte) (int, error) {
		var address windivert.Address
		address.UnsetOutbound()
		address.SetImpostor()
		address.UnsetIPChecksum()
		address.UnsetTCPChecksum()
		address.UnsetUDPChecksum()
		address.Network().InterfaceIndex = iface
		address.Network().SubInterfaceIndex = subiface
		len, err := h3.Send(bytes, &address)
		return int(len), err
	})

	return &App{
		pids: make(map[uint32]bool),
		sessions: &sync.Map{},
		processes: processes,
		whitelist: whitelist,
		lwip: core.NewLWIPStack(),
		hSocket:   h1,
		hNetwork:  h2,
		hReplay:   h3,
	}
}

func ConvertToSession(address windivert.Address) string {
	localAddr := address.Socket().LocalAddress
	remoteAddr := address.Socket().RemoteAddress
	localPort := address.Socket().LocalPort
	remotePort := address.Socket().RemotePort
	protocol := address.Socket().Protocol

	lip := fmt.Sprintf("%v.%v.%v.%v", localAddr[3], localAddr[2], localAddr[1], localAddr[0])
	rip := fmt.Sprintf("%v.%v.%v.%v", remoteAddr[3], remoteAddr[2], remoteAddr[1], remoteAddr[0])
	return fmt.Sprintf("%v:%v=>%v:%v#%v", lip, localPort, rip, remotePort, protocol)
}

func (a *App) Run() {
	go a.filter1()
	go a.filter2()
}

func (a *App) filter1()  {
	address := windivert.Address{}
	buffer := make([]byte, 1500)
	for {
		_, err := a.hSocket.Recv(buffer, &address)
		if err != nil {
			continue
		}
		if v, ok := a.pids[address.Socket().ProcessID]; ok {
			if v {
				// TODO: 列表中
				session := ConvertToSession(address)
				fmt.Println("Socket Layer:", session)
				a.sessions.LoadOrStore(session, true)
			}
		} else if pName, _ := shadow.QueryName(address.Socket().ProcessID); pName != "" {
			//fmt.Println("Program:", pName)
			if _, ok := a.processes[pName]; ok {
				a.pids[address.Socket().ProcessID]= true
				session := ConvertToSession(address)
				fmt.Println("Socket Layer:", session)
				a.sessions.LoadOrStore(session, true)
				// TODO: 处理session
			} else {
				a.pids[address.Socket().ProcessID] = false
			}
		}
	}
}

func (a *App) filter2()  {
	buffer := make([]byte, 1500)
	address := windivert.Address{}

	for {
		_, err := a.hNetwork.Recv(buffer, &address)
		if err != nil {
			continue
		}

		// TODO: 处理捕获的数据包

		packet := gopacket.NewPacket(buffer, layers.LayerTypeIPv4, gopacket.Default)
		if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
			ip, _ := ipLayer.(*layers.IPv4)
			var session string
			if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
				tcp, _ := tcpLayer.(*layers.TCP)
				session = fmt.Sprintf("%v:%v=>%v:%v#%v", ip.SrcIP, uint16(tcp.SrcPort), ip.DstIP, uint16(tcp.DstPort), uint8(ip.Protocol))

			} else if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
				udp, _:= udpLayer.(*layers.UDP)
				session = fmt.Sprintf("%v:%v=>%v:%v#%v", ip.SrcIP, uint16(udp.SrcPort), ip.DstIP, uint16(udp.DstPort), uint8(ip.Protocol))
			}
			if session != "" {
				//fmt.Println("Network Layer : ", session)
				if _, ok := a.sessions.Load(session); ok {
					fmt.Println("Network Layer : ", session)
					a.lwip.Write(buffer)
					continue
				} else if dnsLayer := packet.Layer(layers.LayerTypeDNS); dnsLayer != nil {
					dns, _ := dnsLayer.(*layers.DNS)
					domain := string(dns.Questions[0].Name)
					//fmt.Println("Domain : ", domain)
					if _, ok := a.whitelist[domain]; ok {
						fmt.Println("Domain : ", domain, "=>", ip.DstIP)
						a.lwip.Write(buffer)
						continue
					}
				}
			}
		}

		a.hNetwork.Send(buffer, &address)
	}
}

func (a *App) Close() {
	a.hSocket.Close()
	a.hNetwork.Close()
	a.hReplay.Close()
	a.lwip.Close()
}
