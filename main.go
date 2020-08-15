package main

import (
	"encoding/json"
	"flag"
	"github.com/MissGod1/PProxy/common/dns"
	"github.com/MissGod1/PProxy/common/dns/fakedns"
	"github.com/MissGod1/go-tun2socks/common/log"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/MissGod1/PProxy/utils"
)

// 服务配置
type Server struct {
	Type       string `json:"type"`
	Server     string `json:"server"`
	ServerPort uint16 `json:"server_port"`
	Password   string `json:"password"`
	Method     string `json:"method"`

	Plugin     string `json:"plugin"`
	PluginOpts string `json:"plugin_opts"`
}

// 进程配置
type Process struct {
	Processes []string `json:"processes"`
	Whitelist []string `json:"whitelist"`
	DNS       string   `json:"dns"`
}

var server *Server
var process *Process
var plugin *utils.Plugin
var fakeDns dns.FakeDns

var createrhandler = make(map[string]func())

func RegisterHandler(key string, creater func()) {
	createrhandler[key] = creater
}

func GetServer(file string) *Server {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		panic("Open Server Configure File Error!")
	}
	s := Server{}
	err = json.Unmarshal(data, &s)
	if err != nil {
		panic("Server Configure File Have some issue!")
	}
	return &s
}

func GetProcess(file string) *Process {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		panic("Open Server Configure File Error!")
	}

	p := Process{}
	err = json.Unmarshal(data, &p)
	if err != nil {
		panic("Server Configure File Have some issue!")
	}
	return &p
}

func main() {
	sconfig := flag.String("sconfig", "", "server configure file")
	pconfig := flag.String("pconfig", "", "process configure file")

	flag.Parse()
	if *sconfig == "" || *pconfig == "" {
		flag.Usage()
		os.Exit(1)
	}
	log.SetLevel(log.INFO)
	fakeDns = fakedns.NewSimpleFakeDns()

	server = GetServer(*sconfig)
	process = GetProcess(*pconfig)

	if creater, found := createrhandler[server.Type]; found {
		creater()
	} else {
		panic("Unsupported proxy type.")
	}
	app := NewApp(server, process)
	app.Run()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	if plugin != nil {
		plugin.KillPlugin()
	}
	//app.Close()
}
