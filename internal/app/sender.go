package app

import (
	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/web"
	"strings"
)

// senderAddresses は一覧の取得を差し替えて住所の優先順を検証できる。
func senderAddresses(manual string, list func() ([]netIface, error)) []web.SenderAddress {
	out := []web.SenderAddress{}
	if manual != "" {
		out = append(out, web.SenderAddress{Kind: "publish_host", Address: manual})
	}
	ifs, err := list()
	if err != nil {
		ifs = nil
	}
	seen := map[string]bool{}
	for _, kind := range []string{"lan", "tailscale"} {
		for _, i := range ifs {
			if !i.Up || i.Loopback {
				continue
			}
			for _, a := range i.Addrs {
				ip := addrIP(a)
				v := ip.To4()
				if v == nil {
					continue
				}
				tail := v[0] == 100 && v[1] >= 64 && v[1] <= 127
				if (kind == "lan" && isLANv4(ip) || kind == "tailscale" && tail) && !seen[ip.String()] {
					seen[ip.String()] = true
					out = append(out, web.SenderAddress{Kind: kind, Address: ip.String()})
				}
			}
		}
	}
	return out
}
func (b *pageBackend) Sender() web.SenderSettings {
	return buildSender(b.senderConfig, systemInterfaces, b.senderDefault)
}
func buildSender(cfg *config.Config, list func() ([]netIface, error), defaultHost ...string) web.SenderSettings {
	if cfg == nil {
		return web.SenderSettings{Addresses: []web.SenderAddress{}}
	}
	// 同じ一覧で既定と候補を決め、要求中のインターフェース変化を避ける。
	ifs, err := list()
	stable := func() ([]netIface, error) { return ifs, err }
	host, _, _ := choosePublishHost(cfg.Stream.PublishHost, stable)
	if len(defaultHost) > 0 && defaultHost[0] != "" {
		host = defaultHost[0]
	}
	out := web.SenderSettings{Addresses: senderAddresses(cfg.Stream.PublishHost, stable), Default: host}
	found := false
	for _, a := range out.Addresses {
		found = found || a.Address == host
	}
	if !found {
		out.Addresses = append(out.Addresses, web.SenderAddress{Kind: "lan", Address: host})
	}
	for i := range out.Addresses {
		a := &out.Addresses[i]
		a.Cameras = []web.SenderCamera{}
		for _, u := range buildCameraURLs(cfg, a.Address) {
			c := web.SenderCamera{ID: u.ID, Name: u.Name, OBS: u.OBSRead}
			if u.PublishSRT != "" {
				address, id, _ := strings.Cut(u.PublishSRT, "?streamid=")
				c.SRT = &web.SenderProtocol{URL: u.PublishSRT, Address: address, StreamID: id, User: cfg.Stream.PublishUser, Password: cfg.Stream.PublishPass}
			}
			if u.PublishRTMP != "" {
				path, query, _ := strings.Cut(u.PublishRTMP, "?")
				n := strings.LastIndex(path, "/")
				c.RTMP = &web.SenderProtocol{URL: u.PublishRTMP, Address: path[:n], Key: path[n+1:] + "?" + query, User: cfg.Stream.PublishUser, Password: cfg.Stream.PublishPass}
			}
			a.Cameras = append(a.Cameras, c)
		}
	}
	return out
}
