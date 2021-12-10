package gb28181

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ghettovoice/gosip"
	"github.com/ghettovoice/gosip/sip"
	"github.com/ghettovoice/gosip/util"
)

type GatewayDevice struct {
	DeviceID string
	GroupKey string
	SN       string
	From     string
	// 用于注册过期处理
	RegisterTime time.Time
	Expires      time.Duration
	// 管理通道
	ChannelMap map[string]*Channel
	CSeq       uint32
	Addr       string
}

func (d *GatewayDevice) Query() bool {
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
	res, err := srv.RequestWithContext(context.Background(), request)
	if err != nil {
		logger.Info("query failed", err)
		return false
	}
	return res.StatusCode() == 200
}

func (d *GatewayDevice) UpdateChannels(list []*Channel) {
	logger.Info("updateChannels ", list)
	for i := range list {
		d.ChannelMap[list[i].ChannelID] = list[i]
		list[i].ChannelEx = &ChannelEx{
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

func (c *Channel) Invite(start, end int, ssrc []byte) (streamPath string, ok bool) {

	// c.Bye()
	streamPath = c.ChannelID
	s := "Play"
	if start != 0 {
		s = "Playback"
		streamPath = fmt.Sprintf("%s/%d-%d", c.ChannelID, start, end)
	}

	inviteSdpInfo := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", SC.Serial, SC.MediaIP),
		"s=" + s,
		"u=" + c.ChannelID + ":0",
		"c=IN IP4 " + SC.MediaIP,
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
		logger.Info("send query cmd failed,d=", device.SN, err)
		return "", false
	}
	callID, _ := res.CallID()
	c.CallId = callID.Value()
	from, _ := res.From()
	c.From = from
	to, _ := res.To()
	c.To = to
	return streamPath, true
}

func (c *Channel) Bye() bool {
	if c.CallId != "" {
		d := c.device
		recipient := GetRecipient(d.From)
		d.CSeq++
		maxForwards := sip.MaxForwards(70)
		branchParams := sip.NewParams().Add("branch", sip.String{Str: sip.RFC3261BranchMagicCookie + util.RandString(8)})
		via := sip.ViaHeader{&sip.ViaHop{ProtocolName: "SIP", ProtocolVersion: "2.0", Transport: "UDP", Host: SC.SipIp, Port: &SC.SipPort, Params: branchParams}}
		callID := sip.CallID(c.CallId)
		headers := []sip.Header{&sip.CSeq{SeqNo: d.CSeq, MethodName: sip.BYE}, &maxForwards,
			&callID, c.From, c.To, via}
		request := sip.NewRequest(sip.MessageID(util.RandString(10)), sip.BYE, &recipient, "SIP/2.0",
			headers, "", nil)
		res, err := srv.RequestWithContext(context.Background(), request)
		if err != nil {
			logger.Info("bye failed", err)
			return false
		}
		return res.StatusCode() == 200
	}
	return false
}
