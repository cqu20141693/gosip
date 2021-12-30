package main

import (
	"fmt"

	"github.com/cqu20141693/go-service-common/boot"
	"github.com/cqu20141693/go-service-common/config"

	"github.com/ghettovoice/gosip"
	"github.com/ghettovoice/gosip/gb28181"
	"github.com/ghettovoice/gosip/log"
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
	srvConf := gosip.ServerConfig{UserAgent: "ccsip"}

	gb28181.Run(srvConf)
}
