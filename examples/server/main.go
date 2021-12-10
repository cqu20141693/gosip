package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cqu20141693/go-service-common/boot"
	"github.com/cqu20141693/go-service-common/config"

	"github.com/ghettovoice/gosip"
	"github.com/ghettovoice/gosip/gb28181"
	"github.com/ghettovoice/gosip/log"
	"github.com/ghettovoice/gosip/sip"
)

var (
	sipLog log.Logger
)

func init() {
	sipLog = log.NewDefaultLogrusLogger().WithPrefix("Server")
	//sipLog.SetLevel(log.DebugLevel)
	port := config.GetStringOrDefault("server.port", ":8080")
	fmt.Println("port:", port)
}

func main() {
	boot.Task()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	srvConf := gosip.ServerConfig{UserAgent: "ccsip"}
	srv := gosip.NewServer(srvConf, nil, nil, sipLog)
	_ = srv.OnRequest(sip.INVITE, gb28181.OnInvite)
	_ = srv.OnRequest(sip.MESSAGE, gb28181.OnMessage)
	_ = srv.OnRequest(sip.BYE, gb28181.OnBye)
	_ = srv.OnRequest(sip.REGISTER, gb28181.OnRegister)
	_ = srv.OnRequest(sip.OPTIONS, gb28181.OnOptions)
	_ = srv.OnRequest(sip.ACK, gb28181.OnAck)

	err := srv.Listen(gb28181.SC.Network, gb28181.SC.ListenAddress)
	if err != nil {
		panic(err)
		return
	}
	gb28181.SetSrv(srv)
	gb28181.ApiListen(":15093")
	<-stop
	srv.Shutdown()
}
