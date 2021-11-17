package gosip

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"github.com/ghettovoice/gosip/log"
	"github.com/ghettovoice/gosip/sip"
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

// API

var GetSession = func(c *gin.Context) {
	array := c.QueryArray("ids")
	if array != nil || len(array) > 0 {
		var m map[string]interface{}
		for i := range array {
			if d, ok := Session.Get(array[i]); ok {
				m[array[i]] = d
			}
		}
		c.JSON(200, ResultUtils.Success(m))
	} else {
		c.JSON(200, ResultUtils.Success(Session.session))
	}
}
var AddSession = func(c *gin.Context) {
	d := GatewayDevice{}
	body, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusOK, ResultUtils.Success(false))
		return
	}
	err = json.Unmarshal(body, &d)
	if err != nil {
		c.JSON(http.StatusOK, ResultUtils.Success(false))
		return
	}
	Session.Store(d.ID, &d, 3600*time.Second)
	c.JSON(http.StatusOK, ResultUtils.Success(true))
}

var Invite = func(giCxt *gin.Context) {
	id := giCxt.Query("id")
	channel := giCxt.Query("channel")
	startTime := giCxt.Query("startTime")
	endTime := giCxt.Query("endTime")
	if id == "" {
		giCxt.JSON(http.StatusOK, ResultUtils.Success("id is null"))
		return
	}
	if c, ok := FindChannel(id, channel); ok {
		ret := spi.SrsFacade.CreateChannel(id)
		var ssrc []byte
		if startTime != "" {
			ssrc = []byte("1" + strconv.FormatInt(int64(ret), 10))
		} else {
			ssrc = []byte("0" + strconv.FormatInt(int64(ret), 10))
		}
		start, err1 := strconv.ParseInt(startTime, 10, 0)
		end, err2 := strconv.ParseInt(endTime, 10, 0)
		if err1 != nil || err2 != nil {
			giCxt.JSON(200, ResultUtils.Success("Time params error"))
			return
		}
		c.Invite(start, end, ssrc)
		giCxt.JSON(200, ResultUtils.Success(ssrc))
	} else {
		giCxt.JSON(200, ResultUtils.Success("device not online"))
	}

	//
}

func FindChannel(id string, channel string) (*Channel, bool) {
	if d, ok := Session.Get(id); ok {
		c, ok := d.channelMap[channel]
		return c, ok
	}
	return nil, false
}

var Bye = func(c *gin.Context) {

}

func ApiListen(address string) {
	//gin.SetMode(gin.ReleaseMode)
	engine := gin.New()

	engine.GET("/getSession", GetSession)
	engine.POST("/addSession", AddSession)
	engine.POST("/invite", Invite)
	engine.POST("/bye", Bye)

	err := engine.Run(address)
	if err != nil {
		logger.Fatal("api server start failed")
		return
	}
}

//session
var ctx = context.Background()
var rdb = redis.NewClient(&redis.Options{
	Addr:     "172.19.214.114:6379",
	Password: "chongC@123", // no password set
	DB:       0,            // use default DB
})

// redis constant
const (
	SipSessionPrefix = "sips"
	Delimiter        = ":"
)

type SessionManager interface {
	Store(id string, device *GatewayDevice, expiration time.Duration) bool
	Get(id string) (*GatewayDevice, bool)
	Exist(id string) bool
	KeepAlive(id string, device *GatewayDevice)
}

type MemorySession struct {
	session sync.Map
}

var Session = MemorySession{session: sync.Map{}}

func (m *MemorySession) Store(id string, device *GatewayDevice, expiration time.Duration) bool {
	device.Expires = expiration.Milliseconds()
	m.session.Store(id, device)
	return true
}

func (m *MemorySession) Get(id string) (*GatewayDevice, bool) {
	if load, ok := m.session.Load(id); ok {
		d := load.(*GatewayDevice)
		if time.Since(time.Now()).Milliseconds()-time.Since(d.RegisterTime).Milliseconds() <= d.Expires {
			return d, true
		} else {
			m.session.Delete(id)
		}
	}
	return &GatewayDevice{}, false
}

func (m *MemorySession) Exist(id string) bool {
	if load, ok := m.session.Load(id); ok {
		d := load.(*GatewayDevice)
		if time.Since(time.Now()).Milliseconds()-time.Since(d.RegisterTime).Milliseconds() <= d.Expires {
			return true
		} else {
			m.session.Delete(id)
		}
	}
	return false
}

func (m *MemorySession) KeepAlive(id string, device *GatewayDevice) {
	panic("implement me")
}

type RedisSession struct {
	session sync.Map
}

func (s *RedisSession) Store(id string, device *GatewayDevice, expiration time.Duration) bool {
	key := strings.Join([]string{SipSessionPrefix, id}, Delimiter)
	marshal, err := json.Marshal(device)
	if err != nil {
		return false
	}
	err = rdb.Set(ctx, key, marshal, expiration).Err()
	if err != nil {
		return false
	}
	return true
}
func (s *RedisSession) Exist(id string) bool {
	key := strings.Join([]string{SipSessionPrefix, id}, Delimiter)
	_, err := rdb.Exists(ctx, key).Result()
	if err != nil {
		return false
	}
	return true
}

func (s *RedisSession) Get(id string) (*GatewayDevice, bool) {
	key := strings.Join([]string{SipSessionPrefix, id}, Delimiter)
	result, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return &GatewayDevice{}, false
	}
	d := GatewayDevice{}
	err = json.Unmarshal([]byte(result), &d)
	if err != nil {
		return &GatewayDevice{}, false

	}
	return &d, true
}

func (s *RedisSession) KeepAlive(id string, device *GatewayDevice) {
	panic("implement me")
}

var SC = NewDefaultSipConfig()

func NewDefaultSipConfig() *SipConfig {
	return &SipConfig{
		Serial:        "34020000002000000001",
		Realm:         "3402000000",
		Network:       "udp",
		ListenAddress: "0.0.0.0:5060",
		MediaIP:       "47.108.93.28",
		MediaPort:     9000,
		AudioEnable:   false,
	}
}

type SipConfig struct {
	Serial        string `json:"serial"`
	Realm         string `json:"realm"`
	Network       string `json:"network"`
	ListenAddress string `json:"listenAddress"`
	MediaIP       string //媒体服务器地址
	MediaPort     uint16 //媒体服务器端口
	AudioEnable   bool   //是否开启音频
}

type GatewayDevice struct {
	ID       string
	GroupKey string
	SN       string
	From     sip.ContactHeader
	To       sip.ContactHeader
	CallId   string
	// 用于注册过期处理
	RegisterTime time.Time
	Expires      int64
	// 管理通道
	channelMap map[string]*Channel
}
type Channel struct {
	DeviceID string
	ParentID string
	Name     string
	Children []*Channel
}

func (c *Channel) Invite(start, end int64, ssrc []byte) (streamPath string) {

	// c.Bye()
	streamPath = c.DeviceID
	s := "Play"
	if start != 0 {
		s = "Playback"
		streamPath = fmt.Sprintf("%s/%d-%d", c.DeviceID, start, end)
	}

	inviteSdpInfo := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", SC.Serial, SC.MediaIP),
		"s=" + s,
		"u=" + c.DeviceID + ":0",
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

	headers := []sip.Header{}
	request := sip.NewRequest(sip.MessageID(util.RandString(10)), sip.INVITE, nil, "SIP/2.0", headers, "body", nil)
	srv.Send(request)
	return
}

func (c *Channel) Bye() {

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

var srv Server

func SetSrv(s Server) {
	srv = s
}

var OnRegister RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer wg.Done()
	if req.Method() == sip.REGISTER && tx.Origin().Method() == sip.REGISTER {

		logger.Printf("receive REGISTER cmd", req.Recipient(), req.Body(), tx)
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
			info := deviceFacade.GetDeviceInfo(ID)
			auth := sip.AuthFromValue(authHeader).
				SetMethod(string(req.Method())).
				SetPassword(info.Dt())

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
				id, _ := req.CallID()
				device := GatewayDevice{ID: ID, GroupKey: info.Gk(), SN: info.Sn(), CallId: id.Value()}

				Session.Store(device.ID, &device, 3600*time.Second)
			}
		}

		if _, err := srv.Respond(res); err != nil {
			logger.Errorf("respond '405 Method Not Allowed' failed: %s", err)
		}
	} else {
		logger.Printf("error REGISTER cmd", req, tx)
	}
}
var OnOptions RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer wg.Done()
	if req.Method() == sip.OPTIONS && tx.Origin().Method() == sip.OPTIONS {
		logger.Printf("receive OPTIONS cmd", req, tx)
		res := sip.NewResponseFromRequest("", req, 401, "Method Not Allowed", "")
		if _, err := srv.Respond(res); err != nil {
			logger.Errorf("respond '405 Method Not Allowed' failed: %s", err)
		}
	} else {
		logger.Printf("error OPTIONS cmd", req, tx)
	}
}
var OnInvite RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer wg.Done()
	if req.Method() == sip.INVITE && tx.Origin().Method() == sip.INVITE {
		logger.Printf("receive INVITE cmd", req, tx)
		res := sip.NewResponseFromRequest("", req, 405, "Method Not Allowed", "")
		if _, err := srv.Respond(res); err != nil {
			logger.Errorf("respond '405 Method Not Allowed' failed: %s", err)
		}
	} else {
		logger.Printf("error INVITE cmd", req, tx)
	}
}
var OnBye RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer wg.Done()
	if req.Method() == sip.BYE && tx.Origin().Method() == sip.BYE {
		// 利用callId
		logger.Printf("receive BYE cmd", req, tx)
		res := sip.NewResponseFromRequest("", req, 405, "Method Not Allowed", "")
		if _, err := srv.Respond(res); err != nil {
			logger.Errorf("respond '405 Method Not Allowed' failed: %s", err)
		}
	} else {
		logger.Printf("error BYE cmd", req, tx)
	}
}
var OnAck RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer wg.Done()
	if req.Method() == sip.ACK && tx.Origin().Method() == sip.ACK {
		logger.Printf("receive ACK cmd", req, tx)
		res := sip.NewResponseFromRequest("", req, 405, "Method Not Allowed", "")
		if _, err := srv.Respond(res); err != nil {
			logger.Errorf("respond '405 Method Not Allowed' failed: %s", err)
		}
	} else {
		logger.Printf("error ACK cmd", req, tx)
	}
}

var OnMessage RequestHandler = func(req sip.Request, tx sip.ServerTransaction) {

	wg.Add(1)
	defer wg.Done()
	if req.Method() == sip.MESSAGE && tx.Origin().Method() == sip.MESSAGE {
		from, _ := req.From()
		ID := from.Address.User().String()
		if v, ok := Session.Get(ID); ok {
			marshal, _ := json.Marshal(v)
			logger.Printf(string(marshal))
			temp := &SipMessage{}
			decoder := xml.NewDecoder(bytes.NewReader([]byte(req.Body())))
			decoder.CharsetReader = charset.NewReaderLabel
			err := decoder.Decode(temp)
			if err != nil {
				err = DecodeGbk(temp, []byte(req.Body()))
				if err != nil {
					logger.Printf("decode message err: %s", err)
				}
			}
			if handleMessage(temp) {
				res := sip.NewResponseFromRequest("", req, 200, "OK", "")
				if _, err := srv.Respond(res); err != nil {
					logger.Errorf("respond 200 failed: %s", err)
				}
			}
		}
	} else {
		logger.Printf("error MESSAGE cmd", req, tx)
	}
}

const (
	Notify   = "Notify"
	Response = "Response"
)

func handleMessage(temp *SipMessage) bool {
	logger.Printf("todo handle message", temp)
	switch temp.XMLName.Local {
	case Notify:

	case Response:

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
