package main

import (
	"fmt"
	"github.com/MissGod1/PProxy/common"
	"github.com/MissGod1/PProxy/proxy/shadowsocks"
	"github.com/eycorsican/go-tun2socks/common/log"
	"github.com/eycorsican/go-tun2socks/core"
	"time"
)

func init()  {
	RegisterHandler("shadowsocks", func() {
		//_, err := net.ResolveIPAddr("tcp", server.Server)
		//if err != nil {
		//	log.Fatalf("invalid proxy server address: %v", err)
		//}
		if server.Plugin != "" {
			plugin = common.NewPlugin()
			localAddr, err := plugin.StartPlugin(server.Plugin, server.PluginOpts, fmt.Sprintf("%v:%v", server.Server, server.ServerPort), false)
			if err != nil {
				log.Fatalf("start plugin failed.")
			}
			core.RegisterTCPConnHandler(shadowsocks.NewTCPHandler(localAddr, server.Method, server.Password, fakeDns))
		}else {
			core.RegisterTCPConnHandler(shadowsocks.NewTCPHandler(core.ParseTCPAddr(server.Server, server.ServerPort).String(), server.Method, server.Password, fakeDns))
		}

		core.RegisterUDPConnHandler(shadowsocks.NewUDPHandler(core.ParseUDPAddr(server.Server, server.ServerPort).String(), server.Method, server.Password, 1*time.Second, fakeDns))
	})
}