package spi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ghettovoice/gosip/log"
)

var (
	logger log.Logger = log.NewDefaultLogrusLogger().WithPrefix("facade")
)

type DeviceFacade interface {
	GetDeviceInfo(sn string) *DeviceInfo
}

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
	SRSUrl = "http://172.30.203.21:1985/api/v1/gb28181"
)

func (S *SRSFacadeImpl) CreateChannel(id string) int32 {
	params := "action=" + url.QueryEscape("create_channel") +
		"&stream=[stream]" + "&port_mode=" + url.QueryEscape("fixed") +
		"&app=" + url.QueryEscape("live") + "&id=" + url.QueryEscape(id)
	path := fmt.Sprintf("%s?%s", SRSUrl, params)
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

type DeviceFacadeImpl struct {
}

func (d *DeviceFacadeImpl) GetDeviceInfo(cameraId string) *DeviceInfo {
	return &DeviceInfo{GroupKey: "ccsip", DeviceToken: "ccipcnvr"}
}
