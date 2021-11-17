package sip

import (
	"testing"

	"github.com/ghettovoice/gosip/log"
	"github.com/ghettovoice/gosip/util"
)

var logger log.Logger = log.NewDefaultLogrusLogger().WithPrefix("test")

func TestAuthorization(t *testing.T) {
	value := `realm="3402000000"`
	value = value + `nonce="` + util.RandString(10) + `"`
	a := AuthFromValue(value)
	logger.Info(a.String())
}
