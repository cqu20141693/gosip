package gb28181

import (
	"bytes"
	"encoding/xml"
	"io/ioutil"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"github.com/ghettovoice/gosip"
	"github.com/ghettovoice/gosip/log"
	"github.com/ghettovoice/gosip/sip"
	"github.com/ghettovoice/gosip/sip/parser"
	"github.com/ghettovoice/gosip/spi"
	"github.com/ghettovoice/gosip/util"
)

var (
	logger log.Logger
	wg     *sync.WaitGroup
)

func init() {
	logger = log.NewDefaultLogrusLogger().WithPrefix("Server")
	wg = new(sync.WaitGroup)
}

type ResultCommon struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

type Result interface {
	Success(data interface{}) *ResultCommon
	Fail(code, message string) *ResultCommon
}

type ResultUtil struct {
}

var ResultUtils = &ResultUtil{}

func (r *ResultUtil) Success(data interface{}) *ResultCommon {
	rc := ResultCommon{Code: "200"}
	rc.Data = data
	return &rc
}

func (r *ResultUtil) Fail(code, message string) *ResultCommon {
	return &ResultCommon{Code: code, Message: message}
}

func FindChannel(id string, channel string) (*Channel, bool) {
	if d, ok := Session.Get(id); ok {
		c, ok := d.ChannelMap[channel]
		return c, ok
	}
	return nil, false
}

var SC = NewDefaultSipConfig()

func NewDefaultSipConfig() *SipConfig {
	return &SipConfig{
		Serial:        "34020000002000000001",
		Realm:         "3402000000",
		Network:       "udp",
		ListenAddress: "0.0.0.0:5060",
		SipIp:         "192.168.0.123",
		SipPort:       5060,
		MediaIP:       "47.108.93.28",
		MediaPort:     9000,
		AudioEnable:   false,
	}
}

type SipConfig struct {
	Serial        string   `json:"serial"`
	Realm         string   `json:"realm"`
	Network       string   `json:"network"`
	ListenAddress string   `json:"listenAddress"`
	SipIp         string   // sip 服务器ip
	SipPort       sip.Port // sip 服务器端口
	MediaIP       string   //媒体服务器地址
	MediaPort     uint16   //媒体服务器端口
	AudioEnable   bool     //是否开启音频
}

func GetRecipient(from string) sip.SipUri {
	recipient, _ := parser.ParseSipUri(from)
	return recipient
}

func GetSipHeaders(d *GatewayDevice, method sip.RequestMethod, callId sip.CallID) []sip.Header {
	// 设置via,callId,from,to.max-forwards,Cseq
	// contentType 和body 一起设置
	d.CSeq++
	maxForwards := sip.MaxForwards(70)
	if callId == "" {
		callId = sip.CallID(util.RandString(10))
	}
	tagParams := sip.NewParams().Add("tag", sip.String{Str: util.RandString(8)})
	branchParams := sip.NewParams().Add("branch", sip.String{Str: sip.RFC3261BranchMagicCookie + util.RandString(8)})
	fAddr := sip.SipUri{
		FUser: sip.String{Str: "ccsip"},
		FHost: SC.SipIp,
		FPort: &SC.SipPort,
	}
	from := sip.FromHeader{Address: &fAddr, Params: tagParams}
	tAddr, _ := parser.ParseSipUri(d.From)
	to := sip.ToHeader{Address: &tAddr}
	via := sip.ViaHeader{&sip.ViaHop{ProtocolName: "SIP", ProtocolVersion: "2.0", Transport: "UDP", Host: SC.SipIp, Port: &SC.SipPort, Params: branchParams}}
	return []sip.Header{&sip.CSeq{SeqNo: d.CSeq, MethodName: method}, &maxForwards,
		&callId, &from, &to, via}
}

type Record struct {
	//channel   *Channel
	DeviceID  string
	Name      string
	FilePath  string
	Address   string
	StartTime string
	EndTime   string
	Secrecy   int
	Type      string
}

type SipMessage struct {
	XMLName    xml.Name
	CmdType    string
	DeviceID   string
	DeviceList []*Channel `xml:"DeviceList>Item"`
	RecordList []*Record  `xml:"RecordList>Item"`
}

var deviceFacade = &spi.DeviceFacadeImpl{}

var srv gosip.Server

func SetSrv(s gosip.Server) {
	srv = s
}

var OnRegister gosip.RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer wg.Done()
	if req.Method() == sip.REGISTER && tx.Origin().Method() == sip.REGISTER {

		//logger.Printf("receive REGISTER cmd", req.Recipient(), req.Body(), tx)
		var res sip.Response

		headers := req.GetHeaders("Authorization")
		if headers == nil || len(headers) == 0 {
			value := `realm="3402000000"`
			value = value + `nonce="` + util.RandString(10) + `"`
			authorization := sip.AuthFromValue(value)
			res = sip.NewResponseFromRequest("", req, 401, "Unauthorized", "")
			header := sip.Authenticate(authorization.String())
			res.AppendHeader(&header)
		} else if len(headers) == 1 {
			authHeader := headers[0].Value()
			logger.Info("authorization=", authHeader)
			from, _ := req.From()
			ID := from.Address.User().String()
			logger.Info("contact=", from)

			cameraDO, err := GetByCameraId(ID)
			if err != nil {
				value := `realm="3402000000"`
				value = value + `nonce="` + util.RandString(10) + `"`
				authorization := sip.AuthFromValue(value)
				res = sip.NewResponseFromRequest("", req, 401, "Unauthorized", "")
				header := sip.Authenticate(authorization.String())
				res.AppendHeader(&header)

			} else {
				auth := sip.AuthFromValue(authHeader).
					SetMethod(string(req.Method())).
					SetPassword(cameraDO.Token)

				response := auth.CalcResponse()
				if response != auth.Response() {
					value := `realm="3402000000"`
					value = value + `nonce="` + util.RandString(10) + `"`
					authorization := sip.AuthFromValue(value)
					res = sip.NewResponseFromRequest("", req, 401, "Unauthorized", "")
					header := sip.Authenticate(authorization.String())
					res.AppendHeader(&header)
				} else {
					// register 成功
					res = sip.NewResponseFromRequest("", req, 200, "OK", "")
					from, _ := req.From()
					var expires time.Duration = 3600
					for i := range req.Headers() {
						header := req.Headers()[i]
						if header.Name() == "Expires" {
							expire, err := strconv.ParseInt(header.Value(), 10, 64)
							if err == nil {
								expires = time.Duration(expire)
							}
							break
						}
					}
					device := GatewayDevice{DeviceID: ID, GroupKey: cameraDO.GroupKey, SN: cameraDO.Sn, RegisterTime: time.Now(),
						Expires: expires, From: from.Address.String(), CSeq: 1, ChannelMap: make(map[string]*Channel, 0)}
					device.ChannelMap[ID] = &Channel{
						ChannelID: ID,
						ChannelEx: &ChannelEx{
							device: &device,
						},
					}
					// channel Map not set
					Session.Store(device.DeviceID, &device, device.Expires*time.Second)
					go device.Query()
				}
			}
		}

		if _, err := srv.Respond(res); err != nil {
			logger.Errorf("respond '405 Method Not Allowed' failed: %s", err)
		}
	} else {
		logger.Printf("error REGISTER cmd", req, tx)
	}
}

var OnOptions gosip.RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer wg.Done()
	if req.Method() == sip.OPTIONS && tx.Origin().Method() == sip.OPTIONS {
		from, _ := req.From()
		ID := from.Address.User().String()
		var res sip.Response
		_, ok := Session.Get(ID)
		logger.Printf("receive options cmd", ok, req, tx)
		if ok {
			res = sip.NewResponseFromRequest("", req, 200, "", "")
		} else {
			res = sip.NewResponseFromRequest("", req, 401, "Unauthorized", "")
		}
		if _, err := srv.Respond(res); err != nil {
			logger.Errorf("respond options failed: %s", err)
		}
	} else {
		logger.Printf("error OPTIONS cmd", req, tx)
	}
}
var OnInvite gosip.RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer wg.Done()
	if req.Method() == sip.INVITE && tx.Origin().Method() == sip.INVITE {
		res := sip.NewResponseFromRequest("", req, 405, "Method Not Allowed", "")
		if _, err := srv.Respond(res); err != nil {
			logger.Errorf("respond '405 Method Not Allowed' failed: %s", err)
		}
	} else {
		logger.Printf("error INVITE cmd", req, tx)
	}
}
var OnBye gosip.RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer wg.Done()
	if req.Method() == sip.BYE && tx.Origin().Method() == sip.BYE {

		// 利用callId
		from, _ := req.From()
		ID := from.Address.User().String()
		var res sip.Response
		_, ok := Session.Get(ID)
		if ok {
			Session.Remove(ID)
			res = sip.NewResponseFromRequest("", req, 200, "", "ok")
		} else {
			res = sip.NewResponseFromRequest("", req, 401, "Unauthorized", "")
		}
		if _, err := srv.Respond(res); err != nil {
			logger.Errorf("respond bye failed: %s", err)
		}
	} else {
		logger.Printf("error BYE cmd", req, tx)
	}
}
var OnAck gosip.RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer wg.Done()
	if req.Method() == sip.ACK && tx.Origin().Method() == sip.ACK {
		from, _ := req.From()
		ID := from.Address.User().String()
		var res sip.Response
		_, ok := Session.Get(ID)
		if ok {
			res = sip.NewResponseFromRequest("", req, 200, "", "ok")
		} else {
			res = sip.NewResponseFromRequest("", req, 401, "Unauthorized", "")
		}
		if _, err := srv.Respond(res); err != nil {
			logger.Errorf("respond ack failed: %s", err)
		}
	} else {
		logger.Printf("error ACK cmd", req, tx)
	}
}

var OnMessage gosip.RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer wg.Done()
	if req.Method() == sip.MESSAGE && tx.Origin().Method() == sip.MESSAGE {

		from, _ := req.From()
		ID := from.Address.User().String()
		device, ok := Session.Get(ID)
		var res sip.Response
		if ok {
			if contentType, b := req.ContentType(); b {
				if contentType.Value() == "Application/MANSCDP+xml" {
					msg := &SipMessage{}
					decoder := xml.NewDecoder(bytes.NewReader([]byte(req.Body())))
					decoder.CharsetReader = charset.NewReaderLabel
					err := decoder.Decode(msg)
					if err != nil {
						err = DecodeGbk(msg, []byte(req.Body()))
						if err != nil {
							logger.Printf("decode message err: %s", err)
						}
					}
					if handleMessage(msg, device) {
						res = sip.NewResponseFromRequest("", req, 200, "OK", "")
						if _, err := srv.Respond(res); err != nil {
							logger.Errorf("respond message 200 failed: %s", err)
						}
					} else {
						res = sip.NewResponseFromRequest("", req, 401, "Unauthorized", "")
					}
				} else {
					res = sip.NewResponseFromRequest("", req, 401, "Unauthorized", "")
				}
			} else {
				res = sip.NewResponseFromRequest("", req, 401, "Unauthorized", "")
			}
		} else {
			res = sip.NewResponseFromRequest("", req, 401, "Unauthorized", "")
		}
		if _, err := srv.Respond(res); err != nil {
			logger.Errorf("respond message 405 failed: %s", err)
		}
	} else {
		logger.Printf("error MESSAGE cmd", req, tx)
	}
}

const (
	Notify   = "Notify"
	Response = "Response"
)

func handleMessage(msg *SipMessage, d *GatewayDevice) bool {

	switch msg.XMLName.Local {
	case Notify:
		if d.ChannelMap == nil {
			go d.Query()
		}
	case Response:
		switch msg.CmdType {
		case "Catalog":
			d.UpdateChannels(msg.DeviceList)
		case "RecordInfo":
			logger.Printf("todo handle RecordInfo message", msg)
		}
	}
	return true
}

func DecodeGbk(v interface{}, body []byte) error {
	bodyBytes, err := GbkToUtf8(body)
	if err != nil {
		return err
	}
	decoder := xml.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.CharsetReader = charset.NewReaderLabel
	err = decoder.Decode(v)
	return err
}
func GbkToUtf8(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), simplifiedchinese.GBK.NewDecoder())
	d, e := ioutil.ReadAll(reader)
	if e != nil {
		return s, e
	}
	return d, nil
}
