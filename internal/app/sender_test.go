package app

import (
	"github.com/teru358/irl-live-keeper/internal/config"
	"net"
	"reflect"
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

func TestSenderAddressesFilterVirtualInterfaces(t *testing.T) {
	for _, tc := range []struct {
		name       string
		interfaces []netIface
		wantLAN    []string
	}{
		{name: "実 LAN のみ", interfaces: []netIface{
			{Name: "Ethernet", Up: true, Addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP("192.168.1.16")}}},
			{Name: "vEthernet (WSL)", Up: true, Addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP("172.17.176.1")}}},
			{Name: "vEthernet (Hyper-V)", Up: true, Addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP("172.31.192.1")}}},
		}, wantLAN: []string{"192.168.1.16"}},
		{name: "除外後 LAN が空なら元の LAN を出す", interfaces: []netIface{
			{Name: "docker0", Up: true, Addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP("172.17.0.1")}}},
		}, wantLAN: []string{"172.17.0.1"}},
		{name: "複数の仮想アダプター名", interfaces: []netIface{
			{Name: "WSL", Up: true, Addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP("172.20.0.1")}}},
			{Name: "Hyper-V", Up: true, Addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP("172.21.0.1")}}},
			{Name: "br-123456", Up: true, Addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP("172.22.0.1")}}},
			{Name: "virbr0", Up: true, Addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP("172.23.0.1")}}},
		}, wantLAN: []string{"172.20.0.1", "172.21.0.1", "172.22.0.1", "172.23.0.1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := senderAddresses("", func() ([]netIface, error) { return tc.interfaces, nil })
			var lans []string
			for _, addr := range got {
				if addr.Kind == "lan" {
					lans = append(lans, addr.Address)
				}
			}
			if !reflect.DeepEqual(lans, tc.wantLAN) {
				t.Fatalf("LAN candidates = %v, want %v", lans, tc.wantLAN)
			}
		})
	}
}
func TestBuildSender(t *testing.T) {
	cfg := testConfig(t)
	cfg.Cameras = []config.Camera{{ID: "cam1", PublishPass: "publishpass", Name: "メイン"}}
	cfg.Stream.PublishHost = "example.com"
	cfg.Cameras[0].PublishPass = "&?:%/ space"
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
					if c.ID != "cam1" || c.Name != "メイン" || c.OBS != want.OBSRead {
						t.Fatal(c)
					}
					if cfg.MediaMTX.SRTEnabled {
						if c.SRT == nil || c.SRT.URL != want.PublishSRT || c.SRT.Address+"?streamid="+c.SRT.StreamID != want.PublishSRT || c.SRT.Password != cfg.Cameras[0].PublishPass {
							t.Fatal(c)
						}
					} else if c.SRT != nil {
						t.Fatal(c)
					}
					if cfg.MediaMTX.RTMPEnabled {
						if c.RTMP == nil || c.RTMP.URL != want.PublishRTMP || c.RTMP.Address+"/"+c.RTMP.Key != want.PublishRTMP || c.RTMP.Password != cfg.Cameras[0].PublishPass {
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
