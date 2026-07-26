package hdhomerun

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"strings"
)

// HDHomeRun discovery packet layout (libhdhomerun):
//   uint16 type (BE) | uint16 payload length (BE) | payload TLVs | uint32 CRC (LE, IEEE)

const (
	typeDiscoverReq = 0x0002
	typeDiscoverRpy = 0x0003

	tagDeviceType = 0x01
	tagDeviceID   = 0x02
	tagTunerCount = 0x10
	tagLineupURL  = 0x27
	tagBaseURL    = 0x2A

	deviceTypeWildcard = 0xFFFFFFFF
	deviceTypeTuner    = 0x00000001
	deviceIDWildcard   = 0xFFFFFFFF
)

type discoverReply struct {
	DeviceType uint32
	DeviceID   uint32
	TunerCount int
	BaseURL    string
	LineupURL  string
}

func marshalDiscoverReq() []byte {
	payload := marshalTLVs([]tlv{
		{tag: tagDeviceType, value: uint32BE(deviceTypeWildcard)},
		{tag: tagDeviceID, value: uint32BE(deviceIDWildcard)},
	})
	return marshalPacket(typeDiscoverReq, payload)
}

func parseDiscoverReply(data []byte) (*discoverReply, error) {
	typ, payload, err := unmarshalPacket(data)
	if err != nil {
		return nil, err
	}
	if typ != typeDiscoverRpy {
		return nil, fmt.Errorf("unexpected packet type 0x%04x", typ)
	}
	tlvs, err := unmarshalTLVs(payload)
	if err != nil {
		return nil, err
	}
	reply := &discoverReply{}
	for _, item := range tlvs {
		switch item.tag {
		case tagDeviceType:
			if len(item.value) >= 4 {
				reply.DeviceType = binary.BigEndian.Uint32(item.value[:4])
			}
		case tagDeviceID:
			if len(item.value) >= 4 {
				reply.DeviceID = binary.BigEndian.Uint32(item.value[:4])
			}
		case tagTunerCount:
			if len(item.value) >= 1 {
				reply.TunerCount = int(item.value[0])
			}
		case tagBaseURL:
			reply.BaseURL = cString(item.value)
		case tagLineupURL:
			reply.LineupURL = cString(item.value)
		}
	}
	if reply.DeviceType != 0 && reply.DeviceType != deviceTypeTuner && reply.DeviceType != deviceTypeWildcard {
		return nil, fmt.Errorf("unsupported device type 0x%08x", reply.DeviceType)
	}
	return reply, nil
}

func marshalPacket(packetType uint16, payload []byte) []byte {
	buf := make([]byte, 4+len(payload)+4)
	binary.BigEndian.PutUint16(buf[0:2], packetType)
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(payload)))
	copy(buf[4:], payload)
	crc := crc32.ChecksumIEEE(buf[:4+len(payload)])
	binary.LittleEndian.PutUint32(buf[4+len(payload):], crc)
	return buf
}

func unmarshalPacket(data []byte) (uint16, []byte, error) {
	if len(data) < 8 {
		return 0, nil, errors.New("packet too short")
	}
	packetType := binary.BigEndian.Uint16(data[0:2])
	length := int(binary.BigEndian.Uint16(data[2:4]))
	if len(data) < 4+length+4 {
		return 0, nil, errors.New("packet truncated")
	}
	payload := data[4 : 4+length]
	got := binary.LittleEndian.Uint32(data[4+length:])
	want := crc32.ChecksumIEEE(data[:4+length])
	if got != want {
		return 0, nil, fmt.Errorf("crc mismatch")
	}
	return packetType, payload, nil
}

type tlv struct {
	tag   uint8
	value []byte
}

func marshalTLVs(items []tlv) []byte {
	var buf []byte
	for _, item := range items {
		buf = append(buf, item.tag)
		length := len(item.value)
		if length < 128 {
			buf = append(buf, byte(length))
		} else {
			buf = append(buf, byte(0x80|((length>>7)&0x7F)), byte(length&0x7F))
		}
		buf = append(buf, item.value...)
	}
	return buf
}

func unmarshalTLVs(payload []byte) ([]tlv, error) {
	var out []tlv
	pos := 0
	for pos < len(payload) {
		if pos+2 > len(payload) {
			return nil, errors.New("truncated tlv")
		}
		tag := payload[pos]
		pos++
		length := int(payload[pos] & 0x7F)
		pos++
		if payload[pos-1]&0x80 != 0 {
			if pos >= len(payload) {
				return nil, errors.New("truncated tlv length")
			}
			length = (length << 7) | int(payload[pos])
			pos++
		}
		if pos+length > len(payload) {
			return nil, errors.New("truncated tlv value")
		}
		value := append([]byte(nil), payload[pos:pos+length]...)
		pos += length
		out = append(out, tlv{tag: tag, value: value})
	}
	return out, nil
}

func uint32BE(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

func cString(b []byte) string {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}
