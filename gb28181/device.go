package gb28181

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cqu20141693/go-service-common/config"
	ccredis "github.com/cqu20141693/go-service-common/redis"
	"github.com/cqu20141693/go-service-common/utils"

	"github.com/ghettovoice/gosip"
	"github.com/ghettovoice/gosip/sip"
	"github.com/ghettovoice/gosip/sip/parser"
	"github.com/ghettovoice/gosip/util"
)

type GatewayDevice struct {
	DeviceID string
	From     string
	Addr     string
	// 用于注册过期处理
	RegisterTime time.Time
	Expires      time.Duration
	// 管理通道
	ChannelMap map[string]*Channel
	CSeq       uint32
}

func (d *GatewayDevice) cSeqIncr() {
	key := strings.Join([]string{SipSessionPrefix, d.DeviceID}, Delimiter)
	result, err := ccredis.RedisDB.HIncrBy(context.Background(), key, "CSeq", 1).Result()
	if err != nil {
		return
	}
	d.CSeq = uint32(result)
}

func (d *GatewayDevice) toHashValues() []string {
	rt := strconv.FormatInt(d.RegisterTime.UnixMilli(), 10)
	exp := strconv.FormatInt(int64(d.Expires), 10)
	ip, _ := utils.GetOutBoundIP()
	port := config.GetString("server.port")
	return []string{"from", d.From, "send", d.Addr, "rt", rt, "exp", exp, "addr", strings.Join([]string{ip, port}, Delimiter)}
}

func getDeviceFields() []string {
	return []string{"from", "rt", "exp", "addr"}
}

type ChannelInfo struct {
	CallId string `json:"callId"`
	FTag   string `json:"fTag"`
	TTag   string `json:"tTag"`
}

func CreatChannelInfo(values []interface{}) *ChannelInfo {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("CreatChannelInfo occur error")
			return
		}
	}()
	if len(values) != 3 {
		return nil
	}
	var callId, fTag, tTag string
	if values[0] != nil {
		callId = values[0].(string)
	}
	if values[1] != nil {
		fTag = values[1].(string)
	}
	if values[2] != nil {
		tTag = values[2].(string)
	}
	return &ChannelInfo{callId, fTag, tTag}
}

func (r *ChannelInfo) toHashValues() []string {
	return []string{"callId", r.CallId, "fTag", r.FTag, "tTag", r.TTag}
}

func (d *GatewayDevice) Query() bool {
	//d.cSeqIncr()
	recipient := GetRecipient(d.From)
	contentType := sip.ContentType("Application/MANSCDP+xml")

	headers := GetSipHeaders(d, sip.MESSAGE, "")
	headers = append(headers, &contentType)
	body := fmt.Sprintf(`<?xml version="1.0"?>
<Query>
<CmdType>Catalog</CmdType>
<SN>%d</SN>
<DeviceID>%s</DeviceID>
</Query>`, d.CSeq, d.DeviceID)
	request := sip.NewRequest(sip.MessageID(util.RandString(10)), sip.MESSAGE, &recipient, "SIP/2.0",
		headers, body, nil)
	request.SetDestination(d.Addr)
	res, err := srv.RequestWithContext(context.Background(), request)
	if err != nil {
		logger.Info("query failed ", err)
		return false
	}
	return res.StatusCode() == 200
}

func (d *GatewayDevice) UpdateChannels(list []*Channel) {
	logger.Info("updateChannels ", list)
	for i := range list {
		channel := list[i]
		info := GetChannelInfo(channel.ChannelID)
		if info != nil {
			Session.channelCache[channel.ChannelID] = info
		}
		d.ChannelMap[channel.ChannelID] = channel
		channel.ChannelEx = &ChannelEx{
			device: d,
		}
	}
}

type Channel struct {
	ChannelID    string `xml:"DeviceID"`
	ParentID     string
	Name         string
	Manufacturer string
	Model        string
	Owner        string
	CivilCode    string
	Address      string
	Parental     int
	SafetyWay    int
	RegisterWay  int
	Secrecy      int
	Status       string
	// need store
	CallId     string
	From       *sip.FromHeader
	To         *sip.ToHeader
	Children   []*Channel
	*ChannelEx //自定义属性
}
type ChannelEx struct {
	device *GatewayDevice
}

func (c *Channel) Invite(start, end int, ssrc []byte) (streamPath, fCallID, tCallID, tag string, ok bool) {

	// c.Bye()
	streamPath = c.ChannelID
	s := "Play"
	if start != 0 {
		s = "Playback"
		streamPath = fmt.Sprintf("%s/%d-%d", c.ChannelID, start, end)
	}

	inviteSdpInfo := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", SC.Serial, SC.MediaIp),
		"s=" + s,
		"u=" + c.ChannelID + ":0",
		"c=IN IP4 " + SC.MediaIp,
		fmt.Sprintf("t=%d %d", start, end),
		fmt.Sprintf("m=video %d RTP/AVP 96 97 98", SC.MediaPort),
		"a=recvonly",
		"a=rtpmap:96 PS/90000",
		"a=rtpmap:97 MPEG4/90000",
		"a=rtpmap:98 H264/90000",
		"y=" + string(ssrc),
	}
	fmt.Println(inviteSdpInfo)
	// 接收者
	device := c.device
	recipient := GetRecipient(device.From)
	headers := GetSipHeaders(device, sip.INVITE, "")
	contentType := sip.ContentType("application/sdp")
	headers = append(headers, &contentType)
	request := sip.NewRequest(sip.MessageID(util.RandString(10)), sip.INVITE, &recipient, "SIP/2.0",
		headers, strings.Join(inviteSdpInfo, "\r\n")+"\r\n", nil)
	request.SetDestination(device.Addr)
	res, err := srv.RequestWithContext(context.Background(), request, gosip.WithResponseHandler(func(res sip.Response, request sip.Request) {

		if res.StatusCode() == 100 {
			logger.Info("invite receive 100 calling")
		} else if res.StatusCode() == 200 {
			//ack := sip.NewAckRequest("", request, res, "", nil)
			//res1, err := srv.RequestWithContext(context.Background(), ack)
			//if err != nil || res1.StatusCode() != 200 {
			//	logger.Info("invite ack failed",err)
			//}
			logger.Info("invite ack 200")
		}
		//id, _ := res.CallID()
		//c.CallId = sip.CallID(id.Value())
	}))

	if err != nil || res.StatusCode() != 200 {
		logger.Info("send query cmd failed,d=", device.DeviceID, err)
		return "", "", "", "", false
	}
	callID, _ := res.CallID()
	c.CallId = callID.Value()
	from, _ := res.From()
	fTag, _ := from.Params.Get("tag")
	c.From = from
	to, _ := res.To()
	tTag, _ := to.Params.Get("tag")
	c.To = to
	return streamPath, callID.Value(), fTag.String(), tTag.String(), true
}

func (c *Channel) Bye() bool {
	if c.CallId != "" {
		d := c.device
		recipient := GetRecipient(d.From)
		d.cSeqIncr()
		maxForwards := sip.MaxForwards(70)
		branchParams := sip.NewParams().Add("branch", sip.String{Str: sip.RFC3261BranchMagicCookie + util.RandString(8)})
		via := sip.ViaHeader{&sip.ViaHop{ProtocolName: "SIP", ProtocolVersion: "2.0", Transport: "UDP", Host: SC.SipIp, Port: &SC.SipPort, Params: branchParams}}
		callID := sip.CallID(c.CallId)
		headers := []sip.Header{&sip.CSeq{SeqNo: d.CSeq, MethodName: sip.BYE}, &maxForwards,
			&callID, c.From, c.To, via}
		request := sip.NewRequest(sip.MessageID(util.RandString(10)), sip.BYE, &recipient, "SIP/2.0",
			headers, "", nil)
		request.SetDestination(d.Addr)
		res, err := srv.RequestWithContext(context.Background(), request)
		if err != nil {
			logger.Info("bye failed", err)
			return false
		}
		return res.StatusCode() == 200
	}
	return false
}

func (c *Channel) Bye2() bool {
	info := Session.GetAndDelChannelInfo(c.ChannelID)
	if info != nil {
		d := c.device
		recipient := GetRecipient(d.From)
		d.cSeqIncr()
		maxForwards := sip.MaxForwards(70)
		branchParams := sip.NewParams().Add("branch", sip.String{Str: sip.RFC3261BranchMagicCookie + util.RandString(8)})
		via := sip.ViaHeader{&sip.ViaHop{ProtocolName: "SIP", ProtocolVersion: "2.0", Transport: "UDP", Host: SC.SipIp, Port: &SC.SipPort, Params: branchParams}}
		callID := sip.CallID(info.CallId)
		fParams := sip.NewParams().Add("tag", sip.String{Str: info.FTag})
		fAddr := sip.SipUri{
			FUser: sip.String{Str: "ccsip"},
			FHost: SC.SipIp,
			FPort: &SC.SipPort,
		}
		from := sip.FromHeader{Address: &fAddr, Params: fParams}
		tAddr, _ := parser.ParseSipUri(d.From)
		to := sip.ToHeader{Address: &tAddr, Params: sip.NewParams().Add("tag", sip.String{Str: info.TTag})}
		headers := []sip.Header{&sip.CSeq{SeqNo: d.CSeq, MethodName: sip.BYE}, &maxForwards,
			&callID, &from, &to, via}
		request := sip.NewRequest(sip.MessageID(util.RandString(10)), sip.BYE, &recipient, "SIP/2.0",
			headers, "", nil)
		request.SetDestination(d.Addr)
		deadline := time.Now().Add(time.Second * 10)
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()
		result := make(chan sip.Response, 1)
		go func() {
			res, err := srv.RequestWithContext(ctx, request)
			if err != nil {
				logger.Info("bye failed", err)
				result <- nil
			}
			result <- res
		}()

		select {
		case v := <-result:
			if v == nil {
				return false
			}
			return v.StatusCode() == 200
		case <-ctx.Done():
			return false
		}

	}
	return false
}
