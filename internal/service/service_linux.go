//go:build linux

// Package service は Linux の systemd user unit を登録・解除する。
package service

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/teru358/irl-live-keeper/internal/i18n"
	"gopkg.in/yaml.v3"
)

type Options struct {
	Mode, RunAs, Exe, Dir, Home, UnitDir, SystemDir string
	EUID                                            int
	SudoUser                                        string
	OSLang                                          i18n.Lang
}

var ErrRootUser = errors.New("root cannot be the service user") // 画面には service.root_user を出す

func ResolveUser(runAs, sudoUser string, euid int, current string) (string, error) {
	if runAs != "" {
		return runAs, nil
	}
	if sudoUser != "" {
		return sudoUser, nil
	}
	if euid == 0 {
		return "", ErrRootUser
	}
	return current, nil
}

func Unit(o Options) (string, error) {
	mode := o.Mode
	if mode == "" {
		mode = "user"
	}
	if mode != "user" && mode != "system" {
		return "", fmt.Errorf("service mode must be user or system") // 呼び出しの誤り（run は既定で user にする）
	}
	exe, err := filepath.Abs(o.Exe)
	if err != nil {
		return "", err
	}
	dir := o.Dir
	if dir == "" {
		dir = filepath.Dir(exe)
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	quote := func(s string) string {
		if strings.ContainsAny(s, " \t\"\\") {
			s = strings.ReplaceAll(s, "\\", "\\\\")
			s = strings.ReplaceAll(s, "\"", "\\\"")
			return "\"" + s + "\""
		}
		return s
	}
	u := "[Unit]\nDescription=irl-live-keeper\nAfter=network.target\n\n[Service]\nType=simple\nExecStart=" + quote(exe) + " --dir " + quote(dir) + "\nRestart=on-failure\nKillSignal=SIGTERM\n"
	if mode == "system" {
		name, e := ResolveUser(o.RunAs, o.SudoUser, o.EUID, currentUsername())
		if e != nil {
			return "", e
		}
		if name == "" || name == "root" {
			return "", ErrRootUser
		}
		u += "User=" + name + "\n"
	}
	u += "\n[Install]\nWantedBy="
	if mode == "system" {
		u += "multi-user.target\n"
	} else {
		u += "default.target\n"
	}
	return u, nil
}

func currentUsername() string {
	u, _ := user.Current()
	if u == nil {
		return ""
	}
	return u.Username
}
func configLanguage(dir string, fallback i18n.Lang) i18n.Lang {
	lang, _ := configValues(dir, fallback)
	return lang
}

// configValues は --dir の config.yaml から言語と管理 API のポートを読む。読めなければ fallback と既定のポート。
func configValues(dir string, fallback i18n.Lang) (i18n.Lang, int) {
	const defaultPort = 8787
	b, e := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if e != nil {
		return fallback, defaultPort
	}
	var c struct {
		Language string `yaml:"language"`
		Admin    struct {
			Port int `yaml:"port"`
		} `yaml:"admin"`
	}
	if yaml.Unmarshal(b, &c) != nil {
		return fallback, defaultPort
	}
	port := c.Admin.Port
	if port <= 0 || port > 65535 {
		port = defaultPort
	}
	return i18n.Resolve(c.Language, func() i18n.Lang { return fallback }), port
}

func Run(args []string, stdout, stderr *os.File) int {
	return run(args, stdout, stderr, Options{Exe: executable(), EUID: os.Geteuid(), SudoUser: os.Getenv("SUDO_USER"), OSLang: i18n.OSLang(), Home: homeDir()})
}
func run(args []string, stdout, stderr *os.File, o Options) int {
	if runtime.GOOS != "linux" {
		fmt.Fprintln(stderr, i18n.Render(o.OSLang, i18n.New("service.linux_only")))
		return 1
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, i18n.Render(o.OSLang, i18n.New("service.usage")))
		return 1
	}
	mode, runAs, dir := "", "", ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--user":
			mode = "user"
		case "--system":
			mode = "system"
		case "--run-as":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, i18n.Render(o.OSLang, i18n.New("service.need_value", "flag", "--run-as")))
				return 1
			}
			i++
			runAs = args[i]
		case "--dir":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, i18n.Render(o.OSLang, i18n.New("service.need_value", "flag", "--dir")))
				return 1
			}
			i++
			dir = args[i]
		default:
			fmt.Fprintln(stderr, i18n.Render(o.OSLang, i18n.New("service.unknown_arg", "arg", args[i])))
			return 1
		}
	}
	if mode == "" {
		mode = "user"
	}
	o.Mode, o.RunAs, o.Dir = mode, runAs, dir
	if dir == "" {
		dir = filepath.Dir(o.Exe)
	}
	dir, _ = filepath.Abs(dir)
	o.Dir = dir
	lang, port := configValues(dir, o.OSLang)
	msg := func(code string, p ...string) { fmt.Fprintln(stdout, i18n.Render(lang, i18n.New(code, p...))) }
	if mode == "system" {
		o.UnitDir = o.SystemDir
		if o.UnitDir == "" {
			o.UnitDir = filepath.Join(string(filepath.Separator), "etc", "systemd", "system")
		}
	} else {
		home := o.Home
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		// systemd は XDG_CONFIG_HOME があればその下の systemd/user を読む
		if x := os.Getenv("XDG_CONFIG_HOME"); x != "" && o.Home == "" {
			o.UnitDir = filepath.Join(x, "systemd", "user")
		} else {
			o.UnitDir = filepath.Join(home, ".config", "systemd", "user")
		}
	}
	path := filepath.Join(o.UnitDir, "irl-live-keeper.service")
	if args[0] == "uninstall" {
		if e := os.Remove(path); os.IsNotExist(e) {
			msg("service.missing")
			return 0
		} else if e != nil {
			fmt.Fprintln(stderr, e)
			return 1
		}
		// 自動起動の印（enable が作るリンク）も消す。定義を消したあとでは systemctl disable が
		// 「定義がない」で失敗し、リンクが残るため
		wants := "default.target.wants"
		if mode != "user" {
			wants = "multi-user.target.wants"
		}
		if e := os.Remove(filepath.Join(o.UnitDir, wants, "irl-live-keeper.service")); e != nil && !os.IsNotExist(e) {
			fmt.Fprintln(stderr, e)
			return 1
		}
		if mode == "user" {
			msg("service.uninstall_user")
		} else {
			msg("service.uninstall_system")
		}
		return 0
	}
	if args[0] != "install" {
		fmt.Fprintln(stderr, i18n.Render(lang, i18n.New("service.usage")))
		return 1
	}
	if mode == "system" && o.EUID != 0 {
		fmt.Fprintln(stderr, i18n.Render(lang, i18n.New("service.root")))
		return 1
	}
	unit, e := Unit(o)
	if e != nil {
		if errors.Is(e, ErrRootUser) {
			fmt.Fprintln(stderr, i18n.Render(lang, i18n.New("service.root_user")))
			return 1
		}
		fmt.Fprintln(stderr, e)
		return 1
	}
	if e = os.MkdirAll(o.UnitDir, 0755); e != nil {
		fmt.Fprintf(stderr, "%s: %v\n", i18n.Render(lang, i18n.New("service.write_error")), e)
		return 1
	}
	if e = os.WriteFile(path, []byte(unit), 0644); e != nil {
		fmt.Fprintf(stderr, "%s: %v\n", i18n.Render(lang, i18n.New("service.write_error")), e)
		return 1
	}
	if mode == "user" {
		msg("service.install_user")
		msg("service.linger")
	} else {
		msg("service.install_system")
	}
	msg("service.url", "port", strconv.Itoa(port))
	msg("service.lan")
	msg("service.vpn")
	return 0
}
func executable() string { p, _ := os.Executable(); return p }
func homeDir() string    { p, _ := os.UserHomeDir(); return p }
