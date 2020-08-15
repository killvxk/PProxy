package main

import (
	"fmt"
	"github.com/MissGod1/PProxy/proxy/socks"
	"github.com/MissGod1/go-tun2socks/common/log"
	"github.com/MissGod1/go-tun2socks/core"
	"net"
)

func init()  {
	RegisterHandler("socks5", func() {
		// Verify proxy server address.
		_, err := net.ResolveTCPAddr("tcp",fmt.Sprintf("%v:%v", server.Server, server.ServerPort))
		if err != nil {
			log.Fatalf("invalid proxy server address: %v", err)
		}
		//proxyHost := proxyAddr.IP.String()
		//proxyPort := uint16(proxyAddr.Port)

		core.RegisterTCPConnHandler(socks.NewTCPHandler(server.Server, server.ServerPort, fakeDns))
		core.RegisterUDPConnHandler(socks.NewUDPHandler(server.Server, server.ServerPort, 10, fakeDns))
	})
}
