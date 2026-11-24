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
	appendAddresses := func(kind string, filtered bool) {
		for _, i := range ifs {
			if !i.Up || i.Loopback || filtered && virtualInterface(i.Name) {
				continue
			}
			for _, a := range i.Addrs {
				ip := addrIP(a)
				if ip == nil {
					continue
				}
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
	appendAddresses("lan", true)
	// Keep LAN choices available on platforms whose adapter names are unusual
	// when the interface-name filter would otherwise leave no LAN address.
	hasLAN := false
	for _, a := range out {
		if a.Kind == "lan" {
			hasLAN = true
			break
		}
	}
	if !hasLAN {
		appendAddresses("lan", false)
	}
	appendAddresses("tailscale", false)
	return out
}
func (b *pageBackend) Sender() web.SenderSettings {
	b.app.mu.Lock()
	cfg := b.senderConfig
	host := b.senderDefault
	if b.app.liveConfig != nil {
		cfg = b.app.liveConfig
		host = ""
	}
	b.app.mu.Unlock()
	return buildSender(cfg, systemInterfaces, host)
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
