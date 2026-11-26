// Package sources はソースで使える値を Snapshot から組み立てる。
package sources

import (
	"time"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
)

// Definitions は Compile に渡す一覧。カメラは Snapshot の設定順で、link は先頭の別名。
// 呼び出しごとに独立した map を返す。資格情報と保留中の session は含めない。
func Definitions(s core.Snapshot) map[string]template.Definition {
	d := map[string]template.Definition{}
	add := func(name string, typ template.Type, format string) {
		d[name] = template.Definition{Type: typ, Format: format}
	}
	cameraDefinitions(d, "link.")
	for _, n := range []string{"rtt", "loss", "mbps"} {
		delete(d, "link.srt."+n)
	}
	for _, c := range s.Cameras {
		prefix := "camera." + c.ID + "."
		cameraDefinitions(d, prefix)
		add(prefix+"name", template.String, "")
		add(prefix+"hidden", template.Number, "0")
	}
	for _, n := range []string{"standby.owner", "standby.return", "obs.scene", "obs.scene_class", "obs.scene_health", "mediamtx.state"} {
		add(n, template.String, "")
	}
	for _, n := range []string{"obs.connected", "obs.streaming"} {
		add(n, template.Bool, "")
	}
	add("standby.for", template.Seconds, "hms")
	add("now", template.Clock, "hm")
	add("srt.rtt", template.Number, "0")
	add("srt.loss", template.Number, "2")
	add("srt.mbps", template.Number, "1")
	add("stream.elapsed", template.Seconds, "hms")
	for _, n := range []string{"stream.kbps", "stream.dropped", "obs.render_skipped"} {
		add(n, template.Number, "0")
	}
	add("stream.congestion", template.Number, "2")
	add("obs.fps", template.Number, "1")
	return d
}

func cameraDefinitions(d map[string]template.Definition, p string) {
	for _, n := range []string{"state", "protocol"} {
		d[p+n] = template.Definition{Type: template.String}
	}
	d[p+"kbps"] = template.Definition{Type: template.Number, Format: "0"}
	d[p+"online"] = template.Definition{Type: template.Bool}
	d[p+"state_for"] = template.Definition{Type: template.Seconds, Format: "hms"}
	for n, f := range map[string]string{"rtt": "0", "loss": "2", "mbps": "1"} {
		d[p+"srt."+n] = template.Definition{Type: template.Number, Format: f}
	}
}

// Values は Render に渡す値。now は本体の時計を渡す。stats は補助接続から読んだ統計。
func Values(s core.Snapshot, now time.Time, stats ...map[string]template.Value) map[string]template.Value {
	v := map[string]template.Value{}
	for n, d := range Definitions(s) {
		v[n] = template.Value{Type: d.Type}
	}
	str := func(n, x string) { v[n] = template.Value{Defined: true, Type: template.String, String: x} }
	num := func(n string, x float64) { v[n] = template.Value{Defined: true, Type: template.Number, Number: x} }
	boolean := func(n string, x bool) { v[n] = template.Value{Defined: true, Type: template.Bool, Bool: x} }
	seconds := func(n string, since time.Time) {
		if !since.IsZero() {
			v[n] = template.Value{Defined: true, Type: template.Seconds, Number: max(0, now.Sub(since).Seconds())}
		}
	}
	srt := func(p string, c core.CameraStatus) {
		if c.Protocol == "srt" && c.SRT != nil {
			num(p+"rtt", c.SRT.RTTMs)
			num(p+"loss", c.SRT.LossRate)
			num(p+"mbps", c.SRT.RecvMbps)
		}
	}
	for i, c := range s.Cameras {
		p := "camera." + c.ID + "."
		str(p+"name", c.Name)
		num(p+"hidden", float64(c.Hidden))
		prefixes := []string{p}
		if i == 0 {
			prefixes = append(prefixes, "link.")
			srt("srt.", c)
		}
		for _, p := range prefixes {
			str(p+"state", c.State)
			str(p+"protocol", c.Protocol)
			num(p+"kbps", c.Kbps)
			boolean(p+"online", c.Online)
			seconds(p+"state_for", c.StateSince)
			if p != "link." {
				srt(p+"srt.", c)
			}
		}
	}
	str("standby.owner", s.OBS.Standby.Owner)
	if s.OBS.Standby.ReturnScene != nil {
		str("standby.return", *s.OBS.Standby.ReturnScene)
	}
	if s.OBS.Standby.Owner != "none" {
		seconds("standby.for", s.OBS.Standby.Since)
	}
	boolean("obs.connected", s.OBS.Connected)
	boolean("obs.streaming", s.OBS.Streaming)
	if s.OBS.Connected {
		str("obs.scene", s.OBS.Scene)
	}
	str("obs.scene_class", s.OBS.SceneClass)
	if s.OBS.SceneHealth != nil {
		str("obs.scene_health", *s.OBS.SceneHealth)
	}
	str("mediamtx.state", s.MediaMTX)
	v["now"] = template.Value{Defined: true, Type: template.Clock, Time: now}
	for _, values := range stats {
		for n, x := range values {
			if s.OBS.Connected && (s.OBS.Streaming || n == "obs.fps" || n == "obs.render_skipped") {
				v[n] = x
			}
		}
	}
	return v
}
