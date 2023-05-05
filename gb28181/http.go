package gb28181

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ghettovoice/gosip/spi"
)

func ApiListen(address string, stop chan os.Signal) {
	//gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.GET("/getSession", GetSession)
	engine.POST("/addSession", AddSession)
	engine.POST("/invite", Invite)
	engine.POST("/inviteWithoutBye", InviteWithoutBye)
	engine.POST("/bye", Bye)
	engine.POST("/bye2", Bye2)
	engine.POST("/query", Query)
	service := &CameraService{}
	service.InitRouterMapper(engine)
	go func() {
		err := engine.Run(address)
		if err != nil {
			logger.Fatal("api server start failed")
			stop <- syscall.SIGQUIT
			return
		}
	}()
}

// API

var GetSession = func(c *gin.Context) {
	array := c.QueryArray("ids")
	if array != nil && len(array) > 0 {
		var m = make(map[string]interface{}, 0)
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
	Session.Store(&d, 3600*time.Second)
	c.JSON(http.StatusOK, ResultUtils.Success(true))
}

var Invite = func(giCxt *gin.Context) {
	id := giCxt.Query("id")
	channel := giCxt.Query("channel")
	startTime := giCxt.Query("startTime")
	endTime := giCxt.Query("endTime")
	if id == "" || channel == "" {
		giCxt.JSON(http.StatusOK, ResultUtils.Fail("10001", "parameter error,(id,channel required)"))
		return
	}
	if c, ok := FindChannel(id, channel); ok {
		if atomic.LoadInt32(&c.invited) == 1 {
			giCxt.JSON(200, ResultUtils.Success("invited"))
			return
		}
		c.Bye2()
		ret := spi.SrsFacade.CreateChannel(channel)
		var ssrc []byte
		if startTime != "" {
			strconv.Itoa(int(ret))
			ssrc = []byte("1" + strconv.Itoa(int(ret)))
		} else {
			ssrc = []byte("0" + strconv.Itoa(int(ret)))
		}
		start, _ := strconv.Atoi(startTime)
		end, _ := strconv.Atoi(endTime)
		if atomic.CompareAndSwapInt32(&c.invited, 0, 1) {
			streamPath, callID, fTag, tTag, ok := c.Invite(start, end, ssrc)
			if ok {

				Session.AddChannelInfo(channel, &ChannelInfo{callID, fTag, tTag})
				giCxt.JSON(200, ResultUtils.Success(streamPath))
			} else {
				atomic.StoreInt32(&c.invited, 0)
				giCxt.JSON(200, ResultUtils.Fail("11001", "invite failed"))
			}
		}

	} else {
		giCxt.JSON(200, ResultUtils.Fail("11002", "device not online"))
	}

}

var InviteWithoutBye = func(giCxt *gin.Context) {
	id := giCxt.Query("id")
	channel := giCxt.Query("channel")
	startTime := giCxt.Query("startTime")
	endTime := giCxt.Query("endTime")
	if id == "" || channel == "" {
		giCxt.JSON(http.StatusOK, ResultUtils.Fail("10001", "parameter error,(id,channel required)"))
		return
	}
	if c, ok := FindChannel(id, channel); ok {
		ret := spi.SrsFacade.CreateChannel(channel)
		var ssrc []byte
		if startTime != "" {
			strconv.Itoa(int(ret))
			ssrc = []byte("1" + strconv.Itoa(int(ret)))
		} else {
			ssrc = []byte("0" + strconv.Itoa(int(ret)))
		}
		start, _ := strconv.Atoi(startTime)
		end, _ := strconv.Atoi(endTime)
		streamPath, _, _, _, ok := c.Invite(start, end, ssrc)
		if ok {
			giCxt.JSON(200, ResultUtils.Success(streamPath))
		} else {
			giCxt.JSON(200, ResultUtils.Fail("11001", "invite failed"))
		}
	} else {
		giCxt.JSON(200, ResultUtils.Fail("11002", "device not online"))
	}

}

func Bye(ginCxt *gin.Context) {
	id := ginCxt.Query("id")
	channel := ginCxt.Query("channel")
	if c, ok := FindChannel(id, channel); ok {
		bye := c.Bye()
		if bye {
			ginCxt.JSON(200, ResultUtils.Success("success"))
		} else {
			ginCxt.JSON(200, ResultUtils.Fail("11003", "send bye failed"))
		}
	} else {
		ginCxt.JSON(200, ResultUtils.Fail("11002", "device not online"))
	}
}
func Bye2(ginCxt *gin.Context) {
	id := ginCxt.Query("id")
	channel := ginCxt.Query("channel")
	if c, ok := FindChannel(id, channel); ok {
		bye := c.Bye2()
		if bye {
			ginCxt.JSON(200, ResultUtils.Success("success"))
		} else {
			ginCxt.JSON(200, ResultUtils.Fail("11003", "send bye failed"))
		}
	} else {
		ginCxt.JSON(200, ResultUtils.Fail("11002", "device not online"))
	}
}
func Query(ginCxt *gin.Context) {
	id := ginCxt.Query("id")
	if id != "" {
		if device, b := Session.Get(id); b {
			device.Query()
			ginCxt.JSON(200, ResultUtils.Success("success"))
			return
		} else {
			ginCxt.JSON(200, ResultUtils.Success("device not online"))
		}
	}
	ginCxt.JSON(200, ResultUtils.Success("id is null"))
}
