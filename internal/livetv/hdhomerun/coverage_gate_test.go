package hdhomerun

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeCandidateURLsEdges(t *testing.T) {
	if ProbeCandidateURLs("") != nil {
		t.Fatal("empty")
	}
	if got := ProbeCandidateURLs("://"); len(got) != 0 {
		t.Fatalf("invalid URL candidates: %v", got)
	}
	if got := ProbeCandidateURLs("192.168.1.9"); len(got) == 0 {
		t.Fatal("host without scheme")
	}
	got := ProbeCandidateURLs("http://x/discover.json")
	if got[0] != "http://x/discover.json" {
		t.Fatalf("exact discover path: %v", got)
	}
	got = ProbeCandidateURLs("http://x:9191/")
	if len(got) < 2 || got[0] != "http://x:9191/hdhr/discover.json" {
		t.Fatalf("root on 9191: %v", got)
	}
	got = ProbeCandidateURLs("http://x/app")
	if len(got) < 3 {
		t.Fatalf("default path: %v", got)
	}
	if ClassifyKind(nil, "") != kindUnknown {
		t.Fatal("nil info")
	}
	if ClassifyKind(&DeviceInfo{ModelNumber: "OTHER"}, "http://x/discover.json") != kindHDHomeRun {
		t.Fatal("default kind")
	}
	if DiscoverURLForBase(" ") != "" {
		t.Fatal("empty base")
	}
	if DiscoverURLForBase("http://x/") != "http://x/discover.json" {
		t.Fatal("DiscoverURLForBase")
	}
	if got := uniqueStrings([]string{" a ", "a", "", "b"}); len(got) != 2 {
		t.Fatalf("unique=%v", got)
	}
}

func TestProbeDiscoverURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"DeviceID":"ABC","BaseURL":"http://x","TunerCount":1}`))
	}))
	defer srv.Close()
	c := NewClient(srv.Client())
	info, err := c.ProbeDiscoverURL(context.Background(), srv.URL)
	if err != nil || info.DeviceID != "ABC" {
		t.Fatalf("ProbeDiscoverURL: %+v %v", info, err)
	}
}

func TestPacketErrorPaths(t *testing.T) {
	if _, _, err := unmarshalPacket([]byte{1, 2, 3}); err == nil {
		t.Fatal("too short")
	}
	buf := make([]byte, 12)
	binary.BigEndian.PutUint16(buf[0:2], typeDiscoverRpy)
	binary.BigEndian.PutUint16(buf[2:4], 100) // truncated length
	if _, _, err := unmarshalPacket(buf); err == nil {
		t.Fatal("truncated")
	}
	payload := []byte{1, 2, 3, 4}
	pkt := marshalPacket(typeDiscoverReq, payload)
	pkt[len(pkt)-1] ^= 0xff
	if _, _, err := unmarshalPacket(pkt); err == nil {
		t.Fatal("crc")
	}

	badType := marshalPacket(0x0099, nil)
	if _, err := parseDiscoverReply(badType); err == nil {
		t.Fatal("bad type")
	}
	badDevice := marshalPacket(typeDiscoverRpy, marshalTLVs([]tlv{
		{tag: tagDeviceType, value: uint32BE(0x12345678)},
	}))
	if _, err := parseDiscoverReply(badDevice); err == nil {
		t.Fatal("unsupported device")
	}

	// long TLV length encoding (>=128)
	longVal := make([]byte, 130)
	for i := range longVal {
		longVal[i] = 'x'
	}
	_ = marshalTLVs([]tlv{{tag: tagBaseURL, value: longVal}})

	if _, err := unmarshalTLVs([]byte{0x01}); err == nil {
		t.Fatal("truncated tlv header")
	}
	if _, err := unmarshalTLVs([]byte{0x01, 0x80}); err == nil {
		t.Fatal("truncated extended length")
	}
	if _, err := unmarshalTLVs([]byte{0x01, 0x05, 1, 2}); err == nil {
		t.Fatal("truncated value")
	}
	// extended length form that is valid
	ext := []byte{tagBaseURL, 0x80, 0x02, 'a', 'b'}
	tlvs, err := unmarshalTLVs(ext)
	if err != nil || len(tlvs) != 1 || string(tlvs[0].value) != "ab" {
		t.Fatalf("ext tlv: %v %v", tlvs, err)
	}
	if cString([]byte("hi")) != "hi" {
		t.Fatal("cstring without nul")
	}
	_ = crc32.ChecksumIEEE(nil)
}
