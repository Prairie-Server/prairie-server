package hdhomerun

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDiscoverLANFindsLoopbackResponder(t *testing.T) {
	responder, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen responder: %v", err)
	}
	defer func() { _ = responder.Close() }()

	port := responder.LocalAddr().(*net.UDPAddr).Port
	prev := discoverUDPPort
	discoverUDPPort = port
	t.Cleanup(func() { discoverUDPPort = prev })

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 2048)
		_ = responder.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, addr, err := responder.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if _, _, err := unmarshalPacket(buf[:n]); err != nil {
			return
		}
		rpy := marshalPacket(typeDiscoverRpy, marshalTLVs([]tlv{
			{tag: tagDeviceType, value: uint32BE(deviceTypeTuner)},
			{tag: tagDeviceID, value: uint32BE(0xAABBCCDD)},
			{tag: tagTunerCount, value: []byte{2}},
			// Omit BaseURL to exercise the RemoteIP fallback path.
		}))
		_, _ = responder.WriteToUDP(rpy, addr)
	}()

	got, err := DiscoverLAN(context.Background(), 500*time.Millisecond)
	<-done
	if err != nil {
		t.Fatalf("DiscoverLAN: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("candidates=%+v", got)
	}
	if got[0].DeviceIDHex != "AABBCCDD" || got[0].TunerCount != 2 {
		t.Fatalf("unexpected candidate: %+v", got[0])
	}
	if got[0].BaseURL == "" || got[0].LineupURL == "" {
		t.Fatalf("expected derived urls: %+v", got[0])
	}
}

func TestInterfaceBroadcastAddrsDoesNotPanic(t *testing.T) {
	_ = interfaceBroadcastAddrs()
}

func TestDiscoverURLForBase(t *testing.T) {
	if got := DiscoverURLForBase(" http://x/ "); got != "http://x/discover.json" {
		t.Fatalf("got %q", got)
	}
	if got := DiscoverURLForBase(""); got != "" {
		t.Fatalf("empty got %q", got)
	}
}
