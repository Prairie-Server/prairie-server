package hdhomerun

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

const discoverUDPPort = 65001

// LANCandidate is a device that answered HDHomeRun UDP discovery.
type LANCandidate struct {
	DeviceIDHex string
	TunerCount  int
	BaseURL     string
	LineupURL   string
	RemoteIP    string
}

// DiscoverLAN broadcasts a SiliconDust discovery request and collects replies.
// Requires UDP broadcast reachability to the tuner's subnet (often unavailable
// from bridge-mode Docker without host networking).
func DiscoverLAN(ctx context.Context, timeout time.Duration) ([]LANCandidate, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("hdhomerun lan discover: listen: %w", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	req := marshalDiscoverReq()
	dst := &net.UDPAddr{IP: net.IPv4bcast, Port: discoverUDPPort}
	if _, err := conn.WriteToUDP(req, dst); err != nil {
		return nil, fmt.Errorf("hdhomerun lan discover: broadcast: %w", err)
	}

	// Also unicast to interface broadcast addresses for networks that drop
	// 255.255.255.255 (common on some VMs / Docker bridges).
	for _, bcast := range interfaceBroadcastAddrs() {
		_, _ = conn.WriteToUDP(req, &net.UDPAddr{IP: bcast, Port: discoverUDPPort})
	}

	seen := make(map[string]struct{})
	var out []LANCandidate
	buf := make([]byte, 2048)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			if len(out) > 0 {
				break
			}
			return nil, fmt.Errorf("hdhomerun lan discover: read: %w", err)
		}
		reply, err := parseDiscoverReply(buf[:n])
		if err != nil {
			continue
		}
		idHex := fmt.Sprintf("%08X", reply.DeviceID)
		key := idHex + "|" + reply.BaseURL + "|" + addr.IP.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		base := strings.TrimSpace(reply.BaseURL)
		if base == "" {
			base = "http://" + addr.IP.String()
		}
		lineup := strings.TrimSpace(reply.LineupURL)
		if lineup == "" {
			lineup = strings.TrimRight(base, "/") + "/lineup.json"
		}
		out = append(out, LANCandidate{
			DeviceIDHex: idHex,
			TunerCount:  reply.TunerCount,
			BaseURL:     base,
			LineupURL:   lineup,
			RemoteIP:    addr.IP.String(),
		})
	}
	return out, nil
}

func interfaceBroadcastAddrs() []net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []net.IP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil || ipNet.Mask == nil {
				continue
			}
			ip4 := ipNet.IP.To4()
			mask := net.IP(ipNet.Mask).To4()
			if ip4 == nil || mask == nil {
				continue
			}
			bcast := make(net.IP, 4)
			for i := 0; i < 4; i++ {
				bcast[i] = ip4[i] | ^mask[i]
			}
			out = append(out, bcast)
		}
	}
	return out
}
