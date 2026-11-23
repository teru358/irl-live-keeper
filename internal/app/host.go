package app

import (
	"net"
	"strings"
)

// netIface はネットワークインターフェースのうち、送信用 host の選択に使う部分。
type netIface struct {
	Name     string
	Up       bool
	Loopback bool
	Addrs    []net.Addr
}

// systemInterfaces は OS のインターフェースとアドレスの一覧を返す。外部への通信はしない。
func systemInterfaces() ([]netIface, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]netIface, 0, len(ifs))
	for _, i := range ifs {
		addrs, err := i.Addrs()
		if err != nil {
			continue // 取れないインターフェースは飛ばす
		}
		out = append(out, netIface{
			Name:     i.Name,
			Up:       i.Flags&net.FlagUp != 0,
			Loopback: i.Flags&net.FlagLoopback != 0,
			Addrs:    addrs,
		})
	}
	return out, nil
}

// fallbackHost は自動で見つからなかったときの送信用 host。
const fallbackHost = "127.0.0.1"

// choosePublishHost は送信用 URL の host を決める。
// manual が空でなければそれを使う（auto=false）。空なら list の一覧から、有効で loopback でない
// インターフェースのプライベート IPv4 を、インターフェースとアドレスの順で最初の1つ選ぶ（auto=true）。
// 見つからなければ 127.0.0.1 と found=false を返す。IPv6、リンクローカル、グローバルは選ばない。
func choosePublishHost(manual string, list func() ([]netIface, error)) (host string, auto, found bool) {
	if manual != "" {
		return manual, false, true
	}
	ifs, err := list()
	if err != nil {
		return fallbackHost, true, false
	}
	for _, i := range ifs {
		if !i.Up || i.Loopback || virtualInterface(i.Name) {
			continue
		}
		for _, a := range i.Addrs {
			if ip := addrIP(a); ip != nil && isLANv4(ip) {
				return ip.String(), true, true
			}
		}
	}
	// If filtering removed every LAN address, preserve the previous selection
	// behavior so users still have a usable candidate on unusual networks.
	for _, i := range ifs {
		if !i.Up || i.Loopback {
			continue
		}
		for _, a := range i.Addrs {
			if ip := addrIP(a); ip != nil && isLANv4(ip) {
				return ip.String(), true, true
			}
		}
	}
	return fallbackHost, true, false
}

// virtualInterface identifies common host-only, container, and VM adapters.
func virtualInterface(name string) bool {
	n := strings.ToLower(name)
	for _, marker := range []string{"vethernet", "wsl", "hyper-v", "hyperv", "docker", "br-", "virbr", "vmware", "virtualbox", "vbox", "container"} {
		if strings.Contains(n, marker) {
			return true
		}
	}
	return false
}

// addrIP はアドレスから IP を取り出す。
func addrIP(a net.Addr) net.IP {
	switch v := a.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	}
	return nil
}

// isLANv4 は loopback でないプライベートの IPv4 か。
func isLANv4(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4.IsPrivate() && !v4.IsLoopback()
}
