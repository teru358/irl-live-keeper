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
// path はカメラの id の数だけ書き、publish と read の権限も path ごとに書く（それ以外の path は受け付けない）。
// 値は config の検証で英数字・_・- に限られているので、クォートは不要。
func GenerateYAML(cfg *config.Config, apiUser, apiPass string) ([]byte, error) {
	if apiUser == "" || apiPass == "" {
		return nil, fmt.Errorf("API の資格情報が空です")
	}
	m := cfg.MediaMTX
	s := cfg.Stream
	var pubPerms, readPerms, paths strings.Builder
	for _, cam := range cfg.Cameras {
		fmt.Fprintf(&pubPerms, "      - action: publish\n        path: %s\n", cam.ID)
		fmt.Fprintf(&readPerms, "      - action: read\n        path: %s\n", cam.ID)
		fmt.Fprintf(&paths, "  %s: {}\n", cam.ID)
	}
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
rtmp: %t
rtmpEncryption: "no"
rtmpAddress: :%d
srt: %t
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
%s  - user: %s
    pass: %s
    ips: []
    permissions:
%s  - user: %s
    pass: %s
    ips: ["127.0.0.1", "::1"]
    permissions:
      - action: api
paths:
%s`,
		m.APIPort, m.RTSPPort, m.RTMPEnabled, m.RTMPPort, m.SRTEnabled, m.SRTPort,
		s.PublishUser, s.PublishPass, pubPerms.String(),
		s.ReadUser, s.ReadPass, readPerms.String(),
		apiUser, apiPass,
		paths.String())
	return []byte(out), nil
}

// PublishURLSet は1台のカメラの送信用 URL。止めたプロトコルの URL は作らず、空文字にする。
type PublishURLSet struct {
	ID   string
	SRT  string
	RTMP string
}

// PublishURLs は送信用 URL をカメラの設定の順に作る。host は送信側から見た本機のアドレス。
// ポートは stream.publish_rtmp_port / publish_srt_port が 0 でなければその値、0 なら受信のポート。
func PublishURLs(cfg *config.Config, host string) []PublishURLSet {
	s := cfg.Stream
	rtmpPort, srtPort := cfg.MediaMTX.RTMPPort, cfg.MediaMTX.SRTPort
	if s.PublishRTMPPort != 0 {
		rtmpPort = s.PublishRTMPPort
	}
	if s.PublishSRTPort != 0 {
		srtPort = s.PublishSRTPort
	}
	out := make([]PublishURLSet, 0, len(cfg.Cameras))
	for _, cam := range cfg.Cameras {
		u := PublishURLSet{ID: cam.ID}
		if cfg.MediaMTX.SRTEnabled {
			u.SRT = fmt.Sprintf("srt://%s?streamid=publish:%s:%s:%s", hostPort(host, srtPort), cam.ID, s.PublishUser, s.PublishPass)
		}
		if cfg.MediaMTX.RTMPEnabled {
			u.RTMP = fmt.Sprintf("rtmp://%s/%s?user=%s&pass=%s", hostPort(host, rtmpPort), cam.ID, s.PublishUser, s.PublishPass)
		}
		out = append(out, u)
	}
	return out
}

// ReadURLSet は1台のカメラの OBS メディアソース用 URL の候補。RTSP はいつも作る。
// RTMP と SRT の候補は、そのプロトコルを止めているときは空文字にする。
type ReadURLSet struct {
	ID   string
	RTSP string
	RTMP string
	SRT  string
}

// ReadURLs は OBS が読み出す URL の候補をカメラの設定の順に作る。どれも read の資格情報を埋め込む。
func ReadURLs(cfg *config.Config, host string) []ReadURLSet {
	s := cfg.Stream
	out := make([]ReadURLSet, 0, len(cfg.Cameras))
	for _, cam := range cfg.Cameras {
		u := ReadURLSet{
			ID:   cam.ID,
			RTSP: fmt.Sprintf("rtsp://%s:%s@%s/%s", s.ReadUser, s.ReadPass, hostPort(host, cfg.MediaMTX.RTSPPort), cam.ID),
		}
		if cfg.MediaMTX.RTMPEnabled {
			u.RTMP = fmt.Sprintf("rtmp://%s/%s?user=%s&pass=%s", hostPort(host, cfg.MediaMTX.RTMPPort), cam.ID, s.ReadUser, s.ReadPass)
		}
		if cfg.MediaMTX.SRTEnabled {
			u.SRT = fmt.Sprintf("srt://%s?streamid=read:%s:%s:%s", hostPort(host, cfg.MediaMTX.SRTPort), cam.ID, s.ReadUser, s.ReadPass)
		}
		out = append(out, u)
	}
	return out
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
	type check struct {
		name, network, addr string
		port                int
	}
	// 止めたプロトコルのポートは確かめない（使われていても、MediaMTX は開かないため）
	var checks []check
	if m.RTMPEnabled {
		checks = append(checks,
			check{"rtmp_port", "tcp4", "0.0.0.0:%d", m.RTMPPort},
			check{"rtmp_port", "tcp", ":%d", m.RTMPPort})
	}
	if m.SRTEnabled {
		checks = append(checks,
			check{"srt_port", "udp4", "0.0.0.0:%d", m.SRTPort},
			check{"srt_port", "udp", ":%d", m.SRTPort})
	}
	checks = append(checks,
		check{"rtsp_port", "tcp4", "0.0.0.0:%d", m.RTSPPort},
		check{"rtsp_port", "tcp", ":%d", m.RTSPPort},
		check{"api_port", "tcp", "127.0.0.1:%d", m.APIPort})
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
