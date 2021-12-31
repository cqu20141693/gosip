package main

import (
	"github.com/cqu20141693/go-service-common/boot"

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
}

func main() {
	boot.Task()
	srvConf := gosip.ServerConfig{UserAgent: "ccsip"}
	gb28181.Run(srvConf)
}
