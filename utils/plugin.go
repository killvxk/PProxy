package utils

import (
	"fmt"
	"github.com/MissGod1/PProxy/common/log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type Plugin struct {
	cmd *exec.Cmd
}

func NewPlugin() *Plugin {
	return &Plugin{
		cmd: nil,
	}
}

func (p *Plugin)StartPlugin(plugin, pluginOpts, ssAddr string, isServer bool) (newAddr string, err error) {
	log.Infof("starting plugin (%s) with option (%s)....", plugin, pluginOpts)
	freePort, err := getFreePort()
	if err != nil {
		return "", fmt.Errorf("failed to fetch an unused port for plugin (%v)", err)
	}
	localHost := "127.0.0.1"
	ssHost, ssPort, err := net.SplitHostPort(ssAddr)
	if err != nil {
		return "", err
	}
	newAddr = localHost + ":" + freePort
	if isServer {
		if ssHost == "" {
			ssHost = "0.0.0.0"
		}
		log.Infof("plugin (%s) will listen on %s:%s", plugin, ssHost, ssPort)
	} else {
		log.Infof("plugin (%s) will listen on %s:%s", plugin, localHost, freePort)
	}
	err = p.execPlugin(plugin, pluginOpts, ssHost, ssPort, localHost, freePort)
	return
}

func (p *Plugin) KillPlugin() {
	if p.cmd != nil {
		p.cmd.Process.Signal(syscall.SIGTERM)
		waitCh := make(chan struct{})
		go func() {
			p.cmd.Wait()
			close(waitCh)
		}()
		timeout := time.After(3 * time.Second)
		select {
		case <-waitCh:
		case <-timeout:
			p.cmd.Process.Kill()
		}
	}
}

func (p *Plugin)execPlugin(plugin, pluginOpts, remoteHost, remotePort, localHost, localPort string) (err error) {
	pluginFile := plugin
	if fileExists(plugin) {
		if !filepath.IsAbs(plugin) {
			pluginFile = "./" + plugin
		}
	} else {
		pluginFile, err = exec.LookPath(plugin)
		if err != nil {
			return err
		}
	}
	//logH := newLogHelper("[" + plugin + "]: ")
	env := append(os.Environ(),
		"SS_REMOTE_HOST="+remoteHost,
		"SS_REMOTE_PORT="+remotePort,
		"SS_LOCAL_HOST="+localHost,
		"SS_LOCAL_PORT="+localPort,
		"SS_PLUGIN_OPTIONS="+pluginOpts,
	)
	p.cmd = &exec.Cmd{
		Path:   pluginFile,
		Env:    env,
	}
	if err = p.cmd.Start(); err != nil {
		return err
	}
	go func() {
		if err := p.cmd.Wait(); err != nil {
			log.Infof("plugin exited (%v)\n", err)
			os.Exit(2)
		}
		log.Infof("plugin exited\n")
		os.Exit(0)
	}()
	return nil
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func getFreePort() (string, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return "", err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return "", err
	}
	port := fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port)
	l.Close()
	return port, nil
}
