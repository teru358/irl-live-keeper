//go:build linux

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/i18n"
)

func TestUnitGoldenAndTemplate(t *testing.T) {
	tmp := t.TempDir()
	for _, tc := range []struct{ mode, user, target string }{{"user", "", "default.target"}, {"system", "keeper", "multi-user.target"}} {
		exe, dir := filepath.Join(tmp, "bin folder", "keeper"), filepath.Join(tmp, "data folder")
		u, err := Unit(Options{Mode: tc.mode, RunAs: tc.user, Exe: exe, Dir: dir, SudoUser: "sudo-user", EUID: 1000})
		if err != nil {
			t.Fatal(err)
		}
		expected := "[Unit]\nDescription=irl-live-keeper\nAfter=network.target\n\n[Service]\nType=simple\nExecStart=\"" + exe + "\" --dir \"" + dir + "\"\nRestart=on-failure\nKillSignal=SIGTERM\n"
		if tc.mode == "system" {
			expected += "User=keeper\n"
		}
		expected += "\n[Install]\nWantedBy=" + tc.target + "\n"
		if u != expected {
			t.Fatalf("unit mismatch\nwant:\n%s\ngot:\n%s", expected, u)
		}
		if tc.mode == "user" && strings.Contains(u, "User=") {
			t.Fatal("user unit has User")
		}
	}
	b, err := os.ReadFile("../../irl-live-keeper.service")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ mode, user, target, userLine string }{{"user", "", "default.target", ""}, {"system", "keeper", "multi-user.target", "User=keeper\n"}} {
		template := strings.ReplaceAll(string(b), "{{EXECUTABLE}}", "\"/x/bin folder/keeper\"")
		template = strings.ReplaceAll(template, "{{DIRECTORY}}", "\"/x/data folder\"")
		template = strings.ReplaceAll(template, "{{USER}}", tc.userLine)
		template = strings.ReplaceAll(template, "{{TARGET}}", tc.target)
		u, _ := Unit(Options{Mode: tc.mode, RunAs: tc.user, Exe: "/x/bin folder/keeper", Dir: "/x/data folder"})
		if template != u {
			t.Fatalf("template differs from generated %s unit\n%s\n%s", tc.mode, template, u)
		}
	}
}

func TestResolveUser(t *testing.T) {
	for _, tc := range []struct {
		runAs, sudo   string
		uid           int
		current, want string
		fail          bool
	}{
		{"explicit", "sudo", 0, "current", "explicit", false}, {"", "sudo-user", 0, "current", "sudo-user", false}, {"", "", 1000, "current", "current", false}, {"", "", 0, "root", "", true},
	} {
		got, err := ResolveUser(tc.runAs, tc.sudo, tc.uid, tc.current)
		if (err != nil) != tc.fail || got != tc.want {
			t.Errorf("ResolveUser(%q,%q,%d)=(%q,%v)", tc.runAs, tc.sudo, tc.uid, got, err)
		}
	}
}

func TestInstallErrorsAndUninstall(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	system := filepath.Join(tmp, "system")
	stdoutPath, stderrPath := filepath.Join(tmp, "out"), filepath.Join(tmp, "err")
	open := func(p string) *os.File {
		f, e := os.Create(p)
		if e != nil {
			t.Fatal(e)
		}
		return f
	}
	call := func(args []string, uid int, sysdir string) (int, string, string) {
		out, errf := open(stdoutPath), open(stderrPath)
		code := run(args, out, errf, Options{Exe: filepath.Join(tmp, "bin", "keeper"), Home: home, EUID: uid, SudoUser: "", OSLang: i18n.JA, SystemDir: sysdir})
		out.Close()
		errf.Close()
		a, _ := os.ReadFile(stdoutPath)
		b, _ := os.ReadFile(stderrPath)
		return code, string(a), string(b)
	}
	if c, _, e := call([]string{"install", "--system"}, 0, system); c != 1 || !strings.Contains(e, "root") {
		t.Fatalf("root: %d %s", c, e)
	}
	if e := os.WriteFile(system, []byte("not a directory"), 0600); e != nil {
		t.Fatal(e)
	}
	if c, _, e := call([]string{"install", "--system", "--run-as", "keeper"}, 1000, system); c != 1 || !strings.Contains(e, "sudo") {
		t.Fatalf("write error: %d %s", c, e)
	}
	if c, _, e := call([]string{"install"}, 1000, ""); c != 0 || e != "" {
		t.Fatalf("install: %d %s", c, e)
	}
	unitPath := filepath.Join(home, ".config", "systemd", "user", "irl-live-keeper.service")
	if _, e := os.Stat(unitPath); e != nil {
		t.Fatal(e)
	}
	if c, out, _ := call([]string{"uninstall"}, 1000, ""); c != 0 || !strings.Contains(out, "systemctl --user stop irl-live-keeper.service && systemctl --user daemon-reload") {
		t.Fatalf("uninstall: %d %s", c, out)
	}
	if _, e := os.Stat(unitPath); !os.IsNotExist(e) {
		t.Fatalf("unit remains: %v", e)
	}
	if c, out, _ := call([]string{"uninstall"}, 1000, ""); c != 0 || !strings.Contains(out, "見つかりません") {
		t.Fatalf("missing: %d %s", c, out)
	}
}

func TestLanguageAndServiceGuidance(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		language string
		want     i18n.Lang
	}{{"ja", i18n.JA}, {"en", i18n.EN}, {"auto", i18n.EN}} {
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("language: "+tc.language+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if got := configLanguage(dir, i18n.EN); got != tc.want {
			t.Errorf("language %s: %s", tc.language, got)
		}
	}
	for _, lang := range []i18n.Lang{i18n.JA, i18n.EN} {
		for _, code := range []string{"service.install_user", "service.install_system", "service.uninstall_user", "service.linger", "service.url", "service.lan", "service.vpn"} {
			text := i18n.Render(lang, i18n.New(code, "port", "8787"))
			if text == code || strings.Contains(text, "{") {
				t.Errorf("%s missing in %s: %s", code, lang, text)
			}
		}
		lingerTerm := "loginctl"
		if lang == i18n.EN {
			lingerTerm = "logged out"
		}
		for code, part := range map[string]string{"service.install_user": "systemctl", "service.linger": lingerTerm, "service.url": "127.0.0.1", "service.lan": "admin.lan", "service.vpn": "Tailscale"} {
			if got := i18n.Render(lang, i18n.New(code)); !strings.Contains(strings.ToLower(got), strings.ToLower(part)) {
				t.Errorf("%s %s: %s", lang, code, got)
			}
		}
	}
}

// 案内の管理ページの URL は、--dir の config.yaml の admin.port を使う（8787 の決め打ちにしない）
func TestInstallGuidanceUsesConfiguredPort(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("language: en\nadmin:\n  port: 9123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, port := configValues(dir, i18n.JA); port != 9123 {
		t.Fatalf("port = %d, want 9123", port)
	}
	if lang, port := configValues(t.TempDir(), i18n.JA); lang != i18n.JA || port != 8787 {
		t.Fatalf("no config: %v %d", lang, port)
	}
	if got := i18n.Render(i18n.EN, i18n.New("service.url", "port", "9123")); !strings.Contains(got, "http://127.0.0.1:9123/") {
		t.Fatalf("service.url = %q", got)
	}
}
