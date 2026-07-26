package hdhomerun

import (
	"testing"
)

func TestDiscoverPacketRoundTrip(t *testing.T) {
	req := marshalDiscoverReq()
	typ, payload, err := unmarshalPacket(req)
	if err != nil {
		t.Fatalf("unmarshalPacket(req): %v", err)
	}
	if typ != typeDiscoverReq {
		t.Fatalf("type = 0x%04x", typ)
	}
	if len(payload) == 0 {
		t.Fatal("expected payload")
	}

	rpy := marshalPacket(typeDiscoverRpy, marshalTLVs([]tlv{
		{tag: tagDeviceType, value: uint32BE(deviceTypeTuner)},
		{tag: tagDeviceID, value: uint32BE(0xABCDEF01)},
		{tag: tagTunerCount, value: []byte{4}},
		{tag: tagBaseURL, value: append([]byte("http://192.168.1.50"), 0)},
		{tag: tagLineupURL, value: append([]byte("http://192.168.1.50/lineup.json"), 0)},
	}))
	parsed, err := parseDiscoverReply(rpy)
	if err != nil {
		t.Fatalf("parseDiscoverReply: %v", err)
	}
	if parsed.DeviceID != 0xABCDEF01 || parsed.TunerCount != 4 {
		t.Fatalf("unexpected reply: %+v", parsed)
	}
	if parsed.BaseURL != "http://192.168.1.50" || parsed.LineupURL != "http://192.168.1.50/lineup.json" {
		t.Fatalf("urls = %q / %q", parsed.BaseURL, parsed.LineupURL)
	}
}
