package settings

import (
	"fmt"
	"net"
	"strings"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/i18n"
)

// ListeningPort identifies a listener owned by the currently running generation.
// Protocol is "tcp" or "udp"; Address is the exact listen address, such as
// "0.0.0.0:1935" or "127.0.0.1:8787".
type ListeningPort struct {
	Protocol string
	Address  string
}

// PortFailure is suitable for conversion to the settings API's fields map.
type PortFailure struct {
	Path   string   `json:"path"`
	Reason i18n.Msg `json:"reason"`
}

type portCheck struct{ path, protocol, network, address string }

// CheckPorts binds each requested endpoint and immediately closes it. Only an
// exact endpoint owned by the current generation is skipped.
func CheckPorts(next, current *config.Config, owned []ListeningPort) []PortFailure {
	if next == nil {
		return []PortFailure{{Path: "", Reason: i18n.New("settings.port_missing")}}
	}
	checks := makeChecks(next)
	ownedSet := make(map[string]bool, len(owned))
	for _, p := range owned {
		ownedSet[portKey(p.Protocol, p.Address)] = true
	}
	var failures []PortFailure
	for _, c := range checks {
		if current != nil && c.path == "admin.port" && next.Admin.Port == current.Admin.Port {
			continue
		}
		if ownedSet[portKey(c.protocol, c.address)] {
			continue
		}
		var closer interface{ Close() error }
		var err error
		if c.protocol == "udp" {
			closer, err = net.ListenPacket(c.network, c.address)
		} else {
			closer, err = net.Listen(c.network, c.address)
		}
		if err != nil {
			failures = append(failures, PortFailure{Path: c.path, Reason: i18n.New("settings.port_busy", "protocol", c.protocol, "address", c.address, "error", err.Error())})
			continue
		}
		_ = closer.Close()
	}
	return failures
}

func portKey(protocol, address string) string {
	return protocol + "|" + strings.ToLower(address)
}

func makeChecks(c *config.Config) []portCheck {
	m := c.MediaMTX
	var out []portCheck
	if m.RTMPEnabled {
		out = append(out, portCheck{"mediamtx.rtmp_port", "tcp", "tcp4", fmt.Sprintf("0.0.0.0:%d", m.RTMPPort)}, portCheck{"mediamtx.rtmp_port", "tcp", "tcp", fmt.Sprintf(":%d", m.RTMPPort)})
	}
	if m.SRTEnabled {
		out = append(out, portCheck{"mediamtx.srt_port", "udp", "udp4", fmt.Sprintf("0.0.0.0:%d", m.SRTPort)}, portCheck{"mediamtx.srt_port", "udp", "udp", fmt.Sprintf(":%d", m.SRTPort)})
	}
	out = append(out,
		portCheck{"mediamtx.rtsp_port", "tcp", "tcp4", fmt.Sprintf("0.0.0.0:%d", m.RTSPPort)},
		portCheck{"mediamtx.rtsp_port", "tcp", "tcp", fmt.Sprintf(":%d", m.RTSPPort)},
		portCheck{"mediamtx.api_port", "tcp", "tcp4", fmt.Sprintf("127.0.0.1:%d", m.APIPort)},
	)
	if c.Admin.LAN {
		out = append(out, portCheck{"admin.port", "tcp", "tcp4", fmt.Sprintf("0.0.0.0:%d", c.Admin.Port)}, portCheck{"admin.port", "tcp", "tcp", fmt.Sprintf(":%d", c.Admin.Port)})
	} else {
		out = append(out, portCheck{"admin.port", "tcp", "tcp4", fmt.Sprintf("127.0.0.1:%d", c.Admin.Port)})
	}
	return out
}
