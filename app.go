package main

import (
	"fmt"
	"github.com/eycorsican/go-tun2socks/common/log"
	"github.com/google/gopacket/layers"
	"github.com/pmezard/adblock/adblock"
	"io"
	"sync"
	"time"

	"github.com/MissGod1/PProxy/common"
	"github.com/google/gopacket"
	"github.com/imgk/shadow/device/windivert"
	shadow "github.com/imgk/shadow/utils"
)

type App struct {
	pids      map[uint32]bool  	// pid列表
	sessions  *sync.Map   		// session列表
	processes map[string]bool	// 进程列表
	whitelist map[string]bool	// 域名列表
	domainMatcher *adblock.RuleMatcher

	hSocket  *windivert.Handle
	hNetwork *windivert.Handle
	*io.PipeReader
	*io.PipeWriter
	*windivert.Address

	event chan struct{}
}

const (
	Filter1 = "outbound and !loopback and !ipv6 and (tcp or udp) and event == CONNECT and remoteAddr != %v"
	Filter2 = "ifIdx == %d and outbound and !loopback and !ipv6 and (tcp or udp) and remoteAddr != %v"
)

func SetParam(hd *windivert.Handle) error {
	if er := hd.SetParam(windivert.QueueLength, windivert.QueueLengthMax); er != nil {
		err := fmt.Errorf("set handle parameter queue length error %v", er)
		return err
	}
	if er := hd.SetParam(windivert.QueueTime, windivert.QueueTimeMax); er != nil {
		err := fmt.Errorf("set handle parameter queue time error %v", er)
		return err
	}
	if er := hd.SetParam(windivert.QueueSize, windivert.QueueSizeMax); er != nil {
		err := fmt.Errorf("set handle parameter queue size error %v", er)
		return err
	}
	return nil
}

func NewApp(_server *Server, _process *Process) (*App, error) {
	iface, subiface, err := common.GetInterfaceIndex(_server.Server)
	h1, err := windivert.Open(fmt.Sprintf(Filter1, _server.Server), windivert.LayerSocket, 100, windivert.FlagSniff | windivert.FlagRecvOnly)
	if err != nil {
		err = fmt.Errorf("Open Socket Handle Failed.")
		return nil, err
	}
	h2, err := windivert.Open(fmt.Sprintf(Filter2, iface, _server.Server), windivert.LayerNetwork, 101, 0)
	if err != nil {
		err = fmt.Errorf("Open Network Handle Falied.")
		return nil, err
	}
	err = SetParam(h1)
	if err != nil {
		return nil, err
	}
	err = SetParam(h2)
	if err != nil {
		return nil, err
	}

	processes := make(map[string]bool)
	for _,v := range _process.Processes {
		processes[v] = true
	}

	whitelist := make(map[string]bool)
	for _,v := range _process.Whitelist {
		whitelist[v] = true
	}

	matcher := adblock.NewMatcher()
	for _, r := range _process.Whitelist {
		rule, err := adblock.ParseRule(r)
		if err != nil {
			continue
		}
		matcher.AddRule(rule, 0)
	}

	r, w := io.Pipe()
	app := &App{
		pids: make(map[uint32]bool),
		sessions: &sync.Map{},
		processes: processes,
		whitelist: whitelist,
		hSocket:   h1,
		hNetwork:  h2,
		Address: new(windivert.Address),
		PipeWriter: w,
		PipeReader: r,
		event: make(chan struct{}, 1),
		domainMatcher: matcher,
	}
	app.Address.Network().InterfaceIndex = iface
	app.Address.Network().SubInterfaceIndex = subiface
	go app.filtersession()
	go app.writeloop()

	return app, nil
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

func (a *App) filtersession()  {
	address := make([]windivert.Address, windivert.BatchMax)
	buffer := make([]byte, 1500*windivert.BatchMax)
	for {
		_, nx, err := a.hSocket.RecvEx(buffer, address, nil)
		if err != nil {
			return
		}
		if nx < 1	{
			continue
		}
		for i := uint(0); i < nx; i++ {
			if v, ok := a.pids[address[i].Socket().ProcessID]; ok {
				if v {
					// TODO: 列表中
					session := ConvertToSession(address[i])
					log.Debugf("Socket Layer: %v", session)
					a.sessions.LoadOrStore(session, true)
				}
			} else if pName, _ := shadow.QueryName(address[i].Socket().ProcessID); pName != "" {
				log.Debugf("Program: %v", pName)
				if _, ok := a.processes[pName]; ok {
					a.pids[address[i].Socket().ProcessID]= true
					session := ConvertToSession(address[i])
					log.Debugf("Socket Layer: %v", session)
					a.sessions.LoadOrStore(session, true)
					// TODO: 处理session
				} else {
					a.pids[address[i].Socket().ProcessID] = false
				}
			}
		}
	}
}

func (a *App) WriteTo(w io.Writer) (n int64, err error) {
	buffer := make([]byte, 1500*windivert.BatchMax)
	address := make([]windivert.Address, windivert.BatchMax)

	const f = uint8(0x01<<7) | uint8(0x01<<6) | uint8(0x01<<5) | uint8(0x01<<3)

	for {
		nr, nx, err := a.hNetwork.RecvEx(buffer, address, nil)
		if err != nil {
			return 0, err
		}
		if nr < 1 || nx < 1 {
			continue
		}
		// TODO: 处理捕获的数据包
		n += int64(nr)
		bb := buffer[:nr]
		for i := uint(0); i < nx; i++ {
			l := int(bb[2])<<8 | int(bb[3])

			if a.CheckSession(bb[:l]) {
				_, err = w.Write(bb[:l])
				if err != nil {
					return 0, err
				}

				address[i].Flags |= f

				bb[8] = 0 // TTL = 0
			}

			bb = bb[l:]
		}

		a.hNetwork.Lock()
		_, err = a.hNetwork.SendEx(buffer[:nr], address[:nx], nil)
		a.hNetwork.Unlock()
		if err != nil && err != windivert.ErrHostUnreachable {
			return 0, err
		}
	}
}

func (a *App) Write(data []byte) (n int, err error) {
	a.event <- struct{}{}
	n, err = a.PipeWriter.Write(data)
	return
}

func (a *App)writeloop()  {
	t := time.NewTicker(time.Millisecond)
	defer t.Stop()

	const f = uint8(0x01<<7) | uint8(0x01<<6) | uint8(0x01<<5)

	address := make([]windivert.Address, windivert.BatchMax)
	buffer := make([]byte, 1500*windivert.BatchMax)

	for i := range address {
		address[i] = *a.Address
		address[i].Flags |= f
	}

	n, m := 0, 0
	for {
		select {
		case <-t.C:
			if m > 0 {
				a.hNetwork.Lock()
				_, err := a.hNetwork.SendEx(buffer[:n], address[:m], nil)
				a.hNetwork.Unlock()
				if err != nil {
					return
				}

				n, m = 0, 0
			}
		case <-a.event:
			nr, err := a.PipeReader.Read(buffer[n:])
			if err != nil {
				return
			}

			n += nr
			m++

			if m == windivert.BatchMax {
				a.hNetwork.Lock()
				_, err := a.hNetwork.SendEx(buffer[:n], address[:m], nil)
				a.hNetwork.Unlock()
				if err != nil {
					return
				}

				n, m = 0, 0
			}
		}
	}
}

func (a *App) checkDns(domain string) bool {
	url := fmt.Sprintf("https://%v/", domain)
	rq := &adblock.Request{
		URL: url,
	}
	found, _, err := a.domainMatcher.Match(rq)
	if err != nil {
		return false
	}
	return found
}

func (a *App) CheckSession(buffer []byte) bool {
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
				log.Debugf("Network Layer : %v", session)
				return true
			} else if dnsLayer := packet.Layer(layers.LayerTypeDNS); dnsLayer != nil {
				dns, _ := dnsLayer.(*layers.DNS)
				domain := string(dns.Questions[0].Name)
				//fmt.Println("Domain : ", domain)
				//if _, ok := a.whitelist[domain]; ok {
				if a.checkDns(domain) {
					log.Debugf("Domain : %v => %v", domain, ip.DstIP)
					return true
				}
			}
		}
	}
	return false
}

func (a *App) Close() error {

	if err := a.hSocket.Shutdown(windivert.ShutdownBoth); err != nil {
		return fmt.Errorf("shutdown handle error: %v", err)
	}

	if err := a.hSocket.Close(); err != nil {
		return fmt.Errorf("close handle error: %v", err)
	}

	if err := a.hNetwork.Shutdown(windivert.ShutdownBoth); err != nil {
		return fmt.Errorf("shutdown handle error: %v", err)
	}

	if err := a.hNetwork.Close(); err != nil {
		return fmt.Errorf("close handle error: %v", err)
	}

	close(a.event)
	a.PipeWriter.Close()
	a.PipeReader.Close()

	return nil
}
