package supervisor

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/teru358/irl-live-keeper/internal/config"
)

// GenerateYAML は mediamtx.yml の内容を作る。
// 既定値に頼らず、v1.21.4 にあるプロトコルと機能のスイッチをすべて明示する。
// 値は config の検証で英数字・_・- に限られているので、クォートは不要。
func GenerateYAML(cfg *config.Config, apiUser, apiPass string) ([]byte, error) {
	if apiUser == "" || apiPass == "" {
		return nil, fmt.Errorf("API の資格情報が空です")
	}
	m := cfg.MediaMTX
	s := cfg.Stream
	out := fmt.Sprintf(`# 自動生成。編集しないこと
logLevel: info
logDestinations: [stdout]
api: true
apiAddress: 127.0.0.1:%d
apiEncryption: false
metrics: false
pprof: false
playback: false
rtsp: true
rtspTransports: [tcp]
rtspEncryption: "no"
rtspAddress: :%d
rtmp: true
rtmpEncryption: "no"
rtmpAddress: :%d
srt: true
srtAddress: :%d
hls: false
webrtc: false
moq: false
authMethod: internal
authInternalUsers:
  - user: %s
    pass: %s
    ips: []
    permissions:
      - action: publish
        path: %s
  - user: %s
    pass: %s
    ips: []
    permissions:
      - action: read
        path: %s
  - user: %s
    pass: %s
    ips: ["127.0.0.1", "::1"]
    permissions:
      - action: api
paths:
  %s: {}
`, m.APIPort, m.RTSPPort, m.RTMPPort, m.SRTPort,
		s.PublishUser, s.PublishPass, s.Name,
		s.ReadUser, s.ReadPass, s.Name,
		apiUser, apiPass,
		s.Name)
	return []byte(out), nil
}

// PublishURLSet は送信用 URL。
type PublishURLSet struct {
	SRT  string
	RTMP string
}

// PublishURLs は送信用 URL を作る。host は送信側から見た本機のアドレス。
// ポートは stream.publish_rtmp_port / publish_srt_port が 0 でなければその値、0 なら受信のポート。
func PublishURLs(cfg *config.Config, host string) PublishURLSet {
	s := cfg.Stream
	rtmpPort, srtPort := cfg.MediaMTX.RTMPPort, cfg.MediaMTX.SRTPort
	if s.PublishRTMPPort != 0 {
		rtmpPort = s.PublishRTMPPort
	}
	if s.PublishSRTPort != 0 {
		srtPort = s.PublishSRTPort
	}
	return PublishURLSet{
		SRT:  fmt.Sprintf("srt://%s?streamid=publish:%s:%s:%s", hostPort(host, srtPort), s.Name, s.PublishUser, s.PublishPass),
		RTMP: fmt.Sprintf("rtmp://%s/%s?user=%s&pass=%s", hostPort(host, rtmpPort), s.Name, s.PublishUser, s.PublishPass),
	}
}

// ReadURLSet は OBS メディアソース用 URL の候補。
type ReadURLSet struct {
	RTSP string
	RTMP string
	SRT  string
}

// ReadURLs は OBS が読み出す URL の候補をすべて作る。どれも read の資格情報を埋め込む。
func ReadURLs(cfg *config.Config, host string) ReadURLSet {
	s := cfg.Stream
	return ReadURLSet{
		RTSP: fmt.Sprintf("rtsp://%s:%s@%s/%s", s.ReadUser, s.ReadPass, hostPort(host, cfg.MediaMTX.RTSPPort), s.Name),
		RTMP: fmt.Sprintf("rtmp://%s/%s?user=%s&pass=%s", hostPort(host, cfg.MediaMTX.RTMPPort), s.Name, s.ReadUser, s.ReadPass),
		SRT:  fmt.Sprintf("srt://%s?streamid=read:%s:%s:%s", hostPort(host, cfg.MediaMTX.SRTPort), s.Name, s.ReadUser, s.ReadPass),
	}
}

// hostPort は IPv6 のリテラルも含めて host:port を作る。
func hostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// CheckPorts は MediaMTX と同じ形でポートを listen できるか確かめ、すぐ閉じる。
// 全インターフェースのポートは、IPv4 の 0.0.0.0:port を先に、:port をあとに確かめる。
// Windows では :port が IPv4 の 0.0.0.0:port の使用中も開けてしまうため。
// 補助の診断であって、ポートの予約ではない。
func CheckPorts(cfg *config.Config) error {
	m := cfg.MediaMTX
	checks := []struct {
		name, network, addr string
		port                int
	}{
		{"rtmp_port", "tcp4", "0.0.0.0:%d", m.RTMPPort},
		{"rtmp_port", "tcp", ":%d", m.RTMPPort},
		{"srt_port", "udp4", "0.0.0.0:%d", m.SRTPort},
		{"srt_port", "udp", ":%d", m.SRTPort},
		{"rtsp_port", "tcp4", "0.0.0.0:%d", m.RTSPPort},
		{"rtsp_port", "tcp", ":%d", m.RTSPPort},
		{"api_port", "tcp", "127.0.0.1:%d", m.APIPort},
	}
	for _, c := range checks {
		addr := fmt.Sprintf(c.addr, c.port)
		var closer interface{ Close() error }
		var err error
		if strings.HasPrefix(c.network, "udp") {
			closer, err = net.ListenPacket(c.network, addr)
		} else {
			closer, err = net.Listen(c.network, addr)
		}
		if err != nil {
			return fmt.Errorf("mediamtx.%s のポート %d（%s）を使えません。前回の mediamtx が残っていないか確認してください: %w",
				c.name, c.port, c.network, err)
		}
		closer.Close()
	}
	return nil
}
