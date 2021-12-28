package spi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	httpClient "github.com/asim/go-micro/plugins/client/http/v4"
	"github.com/asim/go-micro/plugins/registry/nacos/v4"
	"github.com/cqu20141693/go-service-common/config"
	"github.com/cqu20141693/go-service-common/event"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/spf13/viper"
	"go-micro.dev/v4/client"
	"go-micro.dev/v4/selector"

	"github.com/ghettovoice/gosip/log"
)

var (
	logger log.Logger = log.NewDefaultLogrusLogger().WithPrefix("facade")
)

type SRSFacade interface {
	CreateChannel(id string) string
}

// 参数配置
var httpCli = &http.Client{
	Timeout: time.Duration(15) * time.Second,
	Transport: &http.Transport{
		MaxIdleConnsPerHost:   1,
		MaxConnsPerHost:       2,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}
var SrsFacade = SRSFacadeImpl{}

type SRSFacadeImpl struct {
}
type Response struct {
	Code int32    `json:"code"`
	Data DataInfo `json:"data"`
}
type DataInfo struct {
	Query QueryInfo `json:"query"`
}
type QueryInfo struct {
	Id       string `json:"id"`
	Ip       string `json:"ip"`
	RtmpPort int32  `json:"rtmp_port"`
	App      string `json:"app"`
	Stream   string `json:"stream"`
	RtpPort  int32  `json:"rtp_port"`
	Ssrc     int32  `json:"ssrc"`
}

const (
	SRSUrl = "http://localhost:1985/api/v1/gb28181"
)

func (S *SRSFacadeImpl) CreateChannel(id string) int32 {
	params := "action=" + url.QueryEscape("create_channel") +
		"&stream=[stream]" + "&port_mode=" + url.QueryEscape("fixed") +
		"&app=" + url.QueryEscape("live") + "&id=" + url.QueryEscape(id)
	path := fmt.Sprintf("%s?%s", config.GetStringOrDefault("cc.srs.url", SRSUrl), params)
	resp, err := httpCli.Get(path)
	if err != nil {
		logger.Infof("createChanel failed,id=%s", id)
		return 0
	}
	r := Response{}

	err = json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		return 0
	}
	return r.Data.Query.Ssrc
}

type DeviceInfo struct {
	GroupKey    string `json:"groupKey"`
	SN          string `json:"sn"`
	DeviceKey   string `json:"deviceKey"`
	DeviceToken string `json:"deviceToken"`
}

func (d DeviceInfo) Sn() string {
	return d.SN
}

func (d DeviceInfo) Gk() string {
	return d.GroupKey
}

func (d DeviceInfo) Dt() string {
	return d.DeviceToken
}

type DeviceFacade interface {
	GetDeviceInfo(cameraId string) (*DeviceInfo, error)
}
type DeviceFacadeImpl struct {
	c    client.Client
	name string
}

var DeviceFacadeClient DeviceFacade

func init() {
	event.RegisterHook(event.ConfigComplete, event.NewHookContext(clientInit, "clientInit"))
}
func clientInit() {
	DeviceFacadeClient = &DeviceFacadeImpl{c: GetClient(), name: "device-backend"}
}

func GetClient() client.Client {
	clientConfig := constant.ClientConfig{}
	err := viper.UnmarshalKey("cc.cloud.nacos.config", &clientConfig)
	if err != nil {
		return nil
	}
	addr := config.GetStringOrDefault("cc.cloud.nacos.server-addr", "localhost:8848")
	addrs := strings.Split(addr, ",")
	registry := nacos.NewRegistry(nacos.WithAddress(addrs), nacos.WithClientConfig(clientConfig))
	selector := selector.NewSelector(selector.Registry(registry))

	return httpClient.NewClient(client.Selector(selector),
		client.ContentType("application/json"))
}

func (d *DeviceFacadeImpl) GetDeviceInfo(cameraId string) (*DeviceInfo, error) {
	endpoint := "/api/device/meta/getByDeviceKey?deviceKey=" + cameraId
	request := d.c.NewRequest(d.name, endpoint, "")
	type result struct {
		Code    string
		Message string
		Data    DeviceInfo
	}
	response := result{}
	err := d.c.Call(context.Background(), request, &response)
	if err != nil {
		return nil, err
	}
	return &response.Data, nil
}
