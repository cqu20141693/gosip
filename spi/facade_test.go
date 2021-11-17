package spi

import (
	"fmt"
	"testing"
)

func TestSRSFacadeImpl_CreateChannel(t *testing.T) {
	ssrc := SrsFacade.CreateChannel("34020000001310000001")
	fmt.Printf("ssrc=%d", ssrc)
}
