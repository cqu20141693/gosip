package gb28181

import (
	"bytes"
	"encoding/xml"
	"io/ioutil"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cqu20141693/go-service-common/config"
	"github.com/cqu20141693/go-service-common/event"
	"github.com/cqu20141693/go-service-common/file"
	"github.com/cqu20141693/go-service-common/logger/cclog"
	"github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
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
	event.RegisterHook(event.ConfigComplete, event.NewHookContext(initSip, "initSip"))
}

func initSip() {
	sub := config.Sub("cc.sip")
	sub.Unmarshal(SC)
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
		SipIp:         "127.0.0.1",
		SipPort:       5060,
		MediaIp:       "127.0.0.1",
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
	MediaIp       string   //媒体服务器地址
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
	d.cSeqIncr()
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

var srv gosip.Server

func SetSrv(s gosip.Server) {
	srv = s
}

var OnRegister gosip.RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer func() {
		wg.Done()
		if err := recover(); err != nil {
			logger.Info("occur panic ", err)
		}
	}()
	if req.Method() == sip.REGISTER && tx.Origin().Method() == sip.REGISTER {

		logger.Info("receive REGISTER cmd", req.Recipient(), req.Headers(), req.Fields())
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
			cameraDO, err := spi.DeviceFacadeClient.GetDeviceInfo(ID)
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
					SetPassword(cameraDO.DeviceToken)

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
					addr := getSendAddr(req)
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
					if expires == 0 {
						Session.Remove(ID)
					} else {

						device := GatewayDevice{DeviceID: ID, RegisterTime: time.Now(),
							Expires: expires, From: from.Address.String(), Addr: addr, CSeq: 1, ChannelMap: make(map[string]*Channel, 0)}
						device.ChannelMap[ID] = &Channel{
							ChannelID: ID,
							ChannelEx: &ChannelEx{
								device: &device,
							},
						}
						// channel Map not set
						Session.Store(&device, device.Expires*time.Second)
						go device.Query()
					}
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

func getSendAddr(req sip.Request) string {
	via, _ := req.Via()
	var received, rport, host, port string
	var viaHop *sip.ViaHop
	for _, hop := range via {
		if r, ok := hop.Params.Get("received"); ok {
			received = r.String()
			viaHop = hop
		}
		if rp, ok := hop.Params.Get("rport"); ok {
			rport = rp.String()
			viaHop = hop
		}
	}

	if rport != "" && rport != "0" && rport != "-1" {
		port = rport
	} else if viaHop.Port != nil {
		port = viaHop.Port.String()
	} else {
		if strings.ToUpper(viaHop.Transport) == "UDP" {
			port = "5060"
		} else {
			port = "5061"
		}
	}
	if received != "" {
		host = received
	} else {
		host = viaHop.Host
	}
	return host + ":" + port
}

var OnOptions gosip.RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer func() {
		wg.Done()
		if err := recover(); err != nil {
			logger.Info("occur panic ", err)
		}
	}()
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
	defer func() {
		wg.Done()
		if err := recover(); err != nil {
			logger.Info("occur panic ", err)
		}
	}()
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
	defer func() {
		wg.Done()
		if err := recover(); err != nil {
			logger.Info("occur panic ", err)
		}
	}()
	if req.Method() == sip.BYE && tx.Origin().Method() == sip.BYE {

		// 利用callId
		from, _ := req.From()
		ID := from.Address.User().String()
		var res sip.Response
		_, ok := Session.Get(ID)
		if ok {
			//Session.Remove(ID)
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
	defer func() {
		wg.Done()
		if err := recover(); err != nil {
			logger.Info("occur panic ", err)
		}
	}()
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
	defer func() {
		wg.Done()
		if err := recover(); err != nil {
			logger.Info("occur panic ", err)
		}
	}()
	if req.Method() == sip.MESSAGE && tx.Origin().Method() == sip.MESSAGE {
		logger.Debug("receive Message cmd", req.Recipient(), req.Headers(), req.Fields())
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
					res = sip.NewResponseFromRequest("", req, 401, "not support content type", "")
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
func Run(srvConf gosip.ServerConfig) {
	ServerInit(srvConf)
}

func ServerInit(srvConf gosip.ServerConfig) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	start(srvConf)
	Session.Recover()
	go ScheduleTask()
	port := config.GetStringOrDefault("server.port", "8080")
	ApiListen(":"+port, stop)
	<-stop
	Shutdown()
}

func Shutdown() {
	srv.Shutdown()
}

func newLogger(prefix string) log.Logger {
	config.Default("cc.log.max-age", 3)
	config.Default("cc.log.rotate-time", "24h")
	rotateTime := config.GetString("cc.log.rotate-time")
	maxAge := config.GetInt64("cc.log.max-age")
	var path string
	if logDir := config.GetStringOrDefault("cc.log.dir", ""); logDir != "" {
		if strings.Contains(logDir, "/") {
			path = logDir
		} else {
			path = file.GetCurrentPath() + string(os.PathSeparator) + logDir
		}
	} else {
		path = file.GetCurrentPath()
	}

	service := config.GetStringOrDefault("cc.application.name", "service")
	writer, err := cclog.GetWriter(path, service+".log", rotateTime, maxAge)
	if err != nil {
		cclog.Error("rotate writer create failed")
		defaultLog := log.NewDefaultLogrusLogger().WithPrefix(prefix)
		return defaultLog
	}
	logger := &logrus.Logger{
		Out:          writer,
		Formatter:    new(logrus.TextFormatter),
		Hooks:        make(logrus.LevelHooks),
		Level:        logrus.InfoLevel,
		ExitFunc:     os.Exit,
		ReportCaller: false,
	}
	logger.Formatter = &prefixed.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	}
	return log.NewLogrusLogger(logger, prefix, nil)
}

func start(srvConf gosip.ServerConfig) {
	logger = newLogger("User")
	logger.Info("sc= ", SC)
	srv := gosip.NewServer(srvConf, nil, nil, newLogger("server"))
	_ = srv.OnRequest(sip.INVITE, OnInvite)
	_ = srv.OnRequest(sip.MESSAGE, OnMessage)
	_ = srv.OnRequest(sip.BYE, OnBye)
	_ = srv.OnRequest(sip.REGISTER, OnRegister)
	_ = srv.OnRequest(sip.OPTIONS, OnOptions)
	_ = srv.OnRequest(sip.ACK, OnAck)
	err := srv.Listen(SC.Network, SC.ListenAddress)
	if err != nil {
		panic(err)
	}
	SetSrv(srv)
}

func ScheduleTask() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// 若通道为空，则阻塞
	// 若通道有数据，则读取
	// 若通道关闭，则退出
	for range ticker.C {

		size := 0
		Session.session.Range(func(deviceId, device interface{}) bool {
			size++
			gatewayDevice := device.(*GatewayDevice)
			gatewayDevice.Query()
			return true
		})
		logger.Info("ticker device query size=", size)
	}
}
