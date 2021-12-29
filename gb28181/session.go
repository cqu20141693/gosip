package gb28181

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cqu20141693/go-service-common/logger/cclog"
	ccredis "github.com/cqu20141693/go-service-common/redis"
	"github.com/go-redis/redis/v8"
)

type SessionManager interface {
	Store(device *GatewayDevice, expiration time.Duration) bool
	Get(id string) (*GatewayDevice, bool)
	Remove(id string)
	Exist(id string) bool
	Recover()
}

type MemorySession struct {
	session      sync.Map
	channelCache map[string]*ChannelInfo
}

func (m *MemorySession) Recover() {
	logger.Info("recover session")
	var allKeys []string
	background := context.Background()
	iter := ccredis.RedisDB.Scan(background, 0, SipSessionPrefix+"*", 500).Iterator()
	for iter.Next(background) {
		allKeys = append(allKeys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		panic(err)
	}
	length := len(allKeys)
	logger.Info("recover ", length)
	if allKeys != nil && length > 0 {
		for start := 0; start < length; start += 50 {
			if start+50 > length {
				handleData(start, length, allKeys)
			} else {
				handleData(start, start+500, allKeys)
			}
		}

	}
}

func handleData(start, end int, keys []string) {
	sub := keys[start:end]
	ctx := context.Background()
	cmders, err := ccredis.RedisDB.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, key := range sub {
			_, _ = pipe.HGetAll(ctx, key).Result()
		}
		return nil
	})
	if err != nil {
		logger.Info("redis pipe get sips data error")
		return
	}
	var removeKeys []string
	for i, cmd := range cmders {
		mapCmd := cmd.(*redis.StringStringMapCmd)
		value := mapCmd.Val()
		key := sub[i]
		if value != nil {
			deviceId := key[5:]
			if !checkAndAddSession(deviceId, value) {
				removeKeys = append(removeKeys, key)
			}
		} else {
			removeKeys = append(removeKeys, key)
			logger.Info("value is nil ,key=", key)
		}
	}
}

func checkAndAddSession(deviceId string, value map[string]string) bool {
	if from, ok := value["from"]; ok {

		if rt, ok := value["rt"]; ok {
			parseInt, err := strconv.ParseInt(rt, 10, 64)
			if err != nil {
				return false
			}
			registerTime := time.UnixMilli(parseInt)
			if expire, ok := value["exp"]; ok {

				exp, err := strconv.ParseInt(expire, 10, 64)
				if err != nil {
					return false
				}
				CSeq := uint32(1)
				if cseq, ok := value["CSeq"]; ok {
					cseqInt, err := strconv.ParseInt(cseq, 10, 64)
					if err != nil {
						return false
					}
					CSeq = uint32(cseqInt)
				}

				channelMap := make(map[string]*Channel, 0)
				device := GatewayDevice{DeviceID: deviceId, From: from, RegisterTime: registerTime, Expires: time.Duration(exp), CSeq: CSeq, ChannelMap: channelMap}
				device.ChannelMap[deviceId] = &Channel{
					ChannelID: deviceId,
					ChannelEx: &ChannelEx{
						device: &device,
					},
				}
				Session.Store(&device, device.Expires)
				device.Query()
				return true
			} else {
				return false
			}
		} else {
			return false
		}
	} else {
		return false
	}
}

func (m *MemorySession) Remove(id string) {
	m.session.Delete(id)
	RedisRouter.RemoveRoute(id)
}

var Session = MemorySession{session: sync.Map{}, channelCache: map[string]*ChannelInfo{}}

func (m *MemorySession) Store(device *GatewayDevice, expiration time.Duration) bool {
	if device == nil {
		return false
	}
	device.Expires = expiration
	m.session.Store(device.DeviceID, device)
	if !RedisRouter.Register(device, expiration) {
		cclog.Warn(fmt.Sprintf("redis router register failed,cameraID=%s", device.DeviceID))
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
			m.Remove(id)
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
			m.Remove(id)
		}
	}
	return false
}

func (m *MemorySession) AddChannelInfo(channelId string, c *ChannelInfo) bool {
	m.channelCache[channelId] = c
	key := strings.Join([]string{SipChannelPrefix, channelId}, Delimiter)
	err := ccredis.RedisDB.HSet(context.Background(), key, c.toHashValues()).Err()
	if err != nil {
		return false
	}
	return true
}
func getChannelFields() []string {
	return []string{"callId", "fTag", "tTag"}
}

func GetChannelInfo(channelId string) *ChannelInfo {
	key := strings.Join([]string{SipChannelPrefix, channelId}, Delimiter)
	result, err := ccredis.RedisDB.HMGet(context.Background(), key, getChannelFields()...).Result()
	if err != nil {
		return nil
	}
	return CreatChannelInfo(result)
}
func (m *MemorySession) GetAndDelChannelInfo(channelId string) *ChannelInfo {
	if info, ok := m.channelCache[channelId]; ok {
		delete(m.channelCache, channelId)
		key := strings.Join([]string{SipChannelPrefix, channelId}, Delimiter)
		ccredis.RedisDB.Del(context.Background(), key)
		return info
	}
	return nil
}

type RouterManager interface {
	// Register 过期时间内有效，重复注册最新有效
	Register(device *GatewayDevice, expire time.Duration) bool

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
	SipChannelPrefix = "sipc"
	Delimiter        = ":"
)

func (r *redisRouter) Register(device *GatewayDevice, expire time.Duration) bool {
	key := strings.Join([]string{SipSessionPrefix, device.DeviceID}, Delimiter)
	_, err := ccredis.RedisDB.HMSet(context.Background(), key, device.toHashValues()).Result()
	ccredis.RedisDB.Expire(context.Background(), key, expire)
	if err != nil {
		return false
	}
	return true
}

func (r *redisRouter) GetRoute(cameraID string) string {
	key := strings.Join([]string{SipSessionPrefix, cameraID}, Delimiter)
	result, err := ccredis.RedisDB.HGet(context.Background(), key, "Addr").Result()
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
	_, err1 := ccredis.RedisDB.Del(context.Background(), key).Result()
	key2 := strings.Join([]string{SipChannelPrefix, cameraID}, Delimiter)
	_, err := ccredis.RedisDB.Del(context.Background(), key2).Result()
	if err1 != nil || err != nil {
		return false
	}
	return true
}
