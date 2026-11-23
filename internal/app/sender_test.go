package app

import (
	"github.com/teru358/irl-live-keeper/internal/config"
	"net"
	"testing"
)

func TestSenderAddresses(t *testing.T) {
	for _, tc := range []struct{ ip, kind string }{{"100.64.0.1", "tailscale"}, {"100.127.255.254", "tailscale"}, {"100.128.0.1", ""}, {"100.63.255.255", ""}, {"192.168.1.2", "lan"}, {"::1", ""}, {"8.8.8.8", ""}} {
		t.Run(tc.ip, func(t *testing.T) {
			list := func() ([]netIface, error) {
				return []netIface{{Up: true, Addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP(tc.ip)}}}}, nil
			}
			got := senderAddresses("example.com", list)
			if len(got) != 1 && tc.kind == "" || tc.kind != "" && (len(got) != 2 || got[1].Kind != tc.kind) {
				t.Fatalf("候補が違う: %+v", got)
			}
		})
	}
}
func TestBuildSender(t *testing.T) {
	cfg := testConfig(t)
	cfg.Cameras = []config.Camera{{ID: "main", Name: "メイン"}}
	cfg.Stream.PublishHost = "example.com"
	cfg.Stream.PublishPass = "&?:%/ space"
	list := func() ([]netIface, error) {
		return []netIface{{Up: true, Addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP("100.64.0.2")}, &net.IPAddr{IP: net.ParseIP("192.168.1.3")}}}}, nil
	}
	for _, app := range []string{"live", ""} {
		for _, enabled := range []string{"both", "srt", "rtmp"} {
			t.Run(app+enabled, func(t *testing.T) {
				cfg.Stream.App = app
				cfg.MediaMTX.SRTEnabled = enabled != "rtmp"
				cfg.MediaMTX.RTMPEnabled = enabled != "srt"
				got := buildSender(cfg, list)
				if got.Default != "example.com" || len(got.Addresses) != 3 || got.Addresses[0].Kind != "publish_host" || got.Addresses[1].Kind != "lan" || got.Addresses[2].Kind != "tailscale" {
					t.Fatalf("住所が違う: %+v", got)
				}
				for _, a := range got.Addresses {
					want := buildCameraURLs(cfg, a.Address)[0]
					c := a.Cameras[0]
					if c.ID != "main" || c.Name != "メイン" || c.OBS != want.OBSRead {
						t.Fatal(c)
					}
					if cfg.MediaMTX.SRTEnabled {
						if c.SRT == nil || c.SRT.URL != want.PublishSRT || c.SRT.Address+"?streamid="+c.SRT.StreamID != want.PublishSRT || c.SRT.Password != cfg.Stream.PublishPass {
							t.Fatal(c)
						}
					} else if c.SRT != nil {
						t.Fatal(c)
					}
					if cfg.MediaMTX.RTMPEnabled {
						if c.RTMP == nil || c.RTMP.URL != want.PublishRTMP || c.RTMP.Address+"/"+c.RTMP.Key != want.PublishRTMP || c.RTMP.Password != cfg.Stream.PublishPass {
							t.Fatal(c)
						}
					} else if c.RTMP != nil {
						t.Fatal(c)
					}
				}
			})
		}
	}
	cfg.Stream.PublishHost = ""
	empty := func() ([]netIface, error) { return nil, nil }
	got := buildSender(cfg, empty)
	if got.Default != fallbackHost || len(got.Addresses) != 1 {
		t.Fatal(got)
	}
}
