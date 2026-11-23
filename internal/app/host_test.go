package app

import (
	"errors"
	"net"
	"testing"
)

func ipnet(s string) net.Addr {
	ip, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	n.IP = ip
	return n
}

func TestChoosePublishHost(t *testing.T) {
	list := func(ifs ...netIface) func() ([]netIface, error) {
		return func() ([]netIface, error) { return ifs, nil }
	}
	tests := []struct {
		name      string
		manual    string
		ifaces    func() ([]netIface, error)
		wantHost  string
		wantAuto  bool
		wantFound bool
	}{
		{
			name: "手動の値はそのまま使い、一覧を見ない", manual: "example.com",
			ifaces:   func() ([]netIface, error) { t.Fatal("手動のときに一覧を取得した"); return nil, nil },
			wantHost: "example.com", wantAuto: false, wantFound: true,
		},
		{
			name:     "プライベートの IPv4 を選ぶ",
			ifaces:   list(netIface{Name: "eth0", Up: true, Addrs: []net.Addr{ipnet("192.168.1.10/24")}}),
			wantHost: "192.168.1.10", wantAuto: true, wantFound: true,
		},
		{
			name: "インターフェースの順で最初の1つ",
			ifaces: list(
				netIface{Name: "eth0", Up: true, Addrs: []net.Addr{ipnet("10.0.0.5/8")}},
				netIface{Name: "wlan0", Up: true, Addrs: []net.Addr{ipnet("192.168.1.10/24")}},
			),
			wantHost: "10.0.0.5", wantAuto: true, wantFound: true,
		},
		{
			name: "loopback は選ばない",
			ifaces: list(
				netIface{Name: "lo", Up: true, Loopback: true, Addrs: []net.Addr{ipnet("127.0.0.1/8")}},
				netIface{Name: "eth0", Up: true, Addrs: []net.Addr{ipnet("172.16.0.2/12")}},
			),
			wantHost: "172.16.0.2", wantAuto: true, wantFound: true,
		},
		{
			name: "down のインターフェースは選ばない",
			ifaces: list(
				netIface{Name: "eth0", Up: false, Addrs: []net.Addr{ipnet("192.168.1.10/24")}},
				netIface{Name: "eth1", Up: true, Addrs: []net.Addr{ipnet("192.168.2.10/24")}},
			),
			wantHost: "192.168.2.10", wantAuto: true, wantFound: true,
		},
		{
			name: "IPv6 とリンクローカルとグローバルは選ばない",
			ifaces: list(netIface{Name: "eth0", Up: true, Addrs: []net.Addr{
				ipnet("fd00::1/64"), ipnet("fe80::1/64"), ipnet("169.254.1.1/16"), ipnet("203.0.113.5/24"),
			}}),
			wantHost: "127.0.0.1", wantAuto: true, wantFound: false,
		},
		{
			name: "同じインターフェースの中ではプライベートの IPv4 を選ぶ",
			ifaces: list(netIface{Name: "eth0", Up: true, Addrs: []net.Addr{
				ipnet("203.0.113.5/24"), ipnet("192.168.1.10/24"),
			}}),
			wantHost: "192.168.1.10", wantAuto: true, wantFound: true,
		},
		{
			name:     "*net.IPAddr の形も扱う",
			ifaces:   list(netIface{Name: "eth0", Up: true, Addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP("10.1.2.3")}}}),
			wantHost: "10.1.2.3", wantAuto: true, wantFound: true,
		},
		{
			name: "見つからなければ 127.0.0.1", ifaces: list(),
			wantHost: "127.0.0.1", wantAuto: true, wantFound: false,
		},
		{
			name:     "一覧を取れなくても 127.0.0.1",
			ifaces:   func() ([]netIface, error) { return nil, errors.New("boom") },
			wantHost: "127.0.0.1", wantAuto: true, wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, auto, found := choosePublishHost(tt.manual, tt.ifaces)
			if host != tt.wantHost || auto != tt.wantAuto || found != tt.wantFound {
				t.Fatalf("got (%q, auto=%v, found=%v), want (%q, auto=%v, found=%v)",
					host, auto, found, tt.wantHost, tt.wantAuto, tt.wantFound)
			}
		})
	}
}
