package gb28181

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cqu20141693/go-service-common/config"
	"github.com/cqu20141693/go-service-common/logger/cclog"
	ccredis "github.com/cqu20141693/go-service-common/redis"
	"github.com/cqu20141693/go-service-common/utils"
	"github.com/go-redis/redis/v8"
)

type SessionManager interface {
	Store(id string, device *GatewayDevice, expiration time.Duration) bool
	Get(id string) (*GatewayDevice, bool)
	Remove(id string)
	Exist(id string) bool
}

type MemorySession struct {
	session sync.Map
}

func (m *MemorySession) Remove(id string) {
	m.session.Delete(id)
	RedisRouter.RemoveRoute(id)
}

var Session = MemorySession{session: sync.Map{}}

func (m *MemorySession) Store(deviceId string, device *GatewayDevice, expiration time.Duration) bool {
	device.Expires = expiration
	m.session.Store(deviceId, device)
	ip, _ := utils.GetOutBoundIP()
	port := config.GetString("server.port")
	if !RedisRouter.Register(ip, port, deviceId, expiration) {
		cclog.Warn(fmt.Sprintf("redis router register failed,cameraID=%s", deviceId))
	}
	return true
}

func (m *MemorySession) Get(id string) (*GatewayDevice, bool) {
	if load, ok := m.session.Load(id); ok {
		d := load.(*GatewayDevice)
		if time.Since(time.Now()).Milliseconds()-time.Since(d.RegisterTime).Milliseconds() <= d.Expires.Milliseconds() {
			return d, true
		} else {
			cclog.Info(fmt.Sprintf("session not exist,cameraID=%s", id))
			m.session.Delete(id)
		}
	}
	return nil, false
}

func (m *MemorySession) Exist(id string) bool {
	if load, ok := m.session.Load(id); ok {
		d := load.(*GatewayDevice)
		if time.Since(time.Now()).Milliseconds()-time.Since(d.RegisterTime).Milliseconds() <= d.Expires.Milliseconds() {
			return true
		} else {
			cclog.Info(fmt.Sprintf("session not exist,cameraID=%s", id))
			m.session.Delete(id)
		}
	}
	return false
}

type RouterManager interface {
	// Register 过期时间内有效，重复注册最新有效
	Register(ip, port, cameraID string, expire time.Duration) bool

	GetRoute(cameraID string) string

	RemoveRoute(cameraID string) bool
}

//RSession
var RedisRouter = redisRouter{}

type redisRouter struct {
}

//session
// cc redis constant
const (
	SipSessionPrefix = "sips"
	Delimiter        = ":"
)

func (r *redisRouter) Register(ip, port, cameraID string, expire time.Duration) bool {
	key := strings.Join([]string{SipSessionPrefix, cameraID}, Delimiter)
	value := strings.Join([]string{ip, port}, Delimiter)
	_, err := ccredis.RedisDB.Set(context.Background(), key, value, expire).Result()
	if err != nil {
		return false
	}
	return true
}

func (r *redisRouter) GetRoute(cameraID string) string {
	key := strings.Join([]string{SipSessionPrefix, cameraID}, Delimiter)
	result, err := ccredis.RedisDB.Get(context.Background(), key).Result()
	switch {
	case err == redis.Nil:
		return ""
	case err != nil:
		cclog.Info("redis request failed")
		return ""
	default:
		return result
	}
}

func (r *redisRouter) RemoveRoute(cameraID string) bool {
	key := strings.Join([]string{SipSessionPrefix, cameraID}, Delimiter)
	_, err := ccredis.RedisDB.Del(context.Background(), key).Result()
	if err != nil {
		return false
	}
	return true
}
