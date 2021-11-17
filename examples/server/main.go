package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/ghettovoice/gosip"
	"github.com/ghettovoice/gosip/log"
	"github.com/ghettovoice/gosip/sip"
)

var (
	logger log.Logger
)

func init() {
	logger = log.NewDefaultLogrusLogger().WithPrefix("Server")
}

var Srv *gosip.Server

func main() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	srvConf := gosip.ServerConfig{UserAgent: "ccsip"}
	srv := gosip.NewServer(srvConf, nil, nil, logger)
	srv.OnRequest(sip.INVITE, gosip.OnInvite)
	srv.OnRequest(sip.MESSAGE, gosip.OnMessage)
	srv.OnRequest(sip.BYE, gosip.OnBye)
	srv.OnRequest(sip.REGISTER, gosip.OnRegister)
	srv.OnRequest(sip.OPTIONS, gosip.OnOptions)
	srv.OnRequest(sip.ACK, gosip.OnAck)

	srv.Listen(gosip.SC.Network, gosip.SC.ListenAddress)
	gosip.ApiListen(":15093")
	gosip.SetSrv(srv)
	<-stop
	srv.Shutdown()
}
