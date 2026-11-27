package logging

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func newTestLogger(t *testing.T, secrets ...string) (*Logger, *bytes.Buffer, string, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: at(2026, 11, 21, 12, 0, 0)}
	var out bytes.Buffer
	dir := t.TempDir()
	return openLogger(t, dir, 7, secrets, clk, &out), &out, dir, clk
}

func TestAddSecretsEverywhere(t *testing.T) {
	lg, out, dir, clk := newTestLogger(t, "old-secret")
	lg.AddSecrets("", "new-secret")
	lg.Slog().Info("x", "a", "old-secret", "b", "new-secret")
	file := readFile(t, dir+"/"+fileName(clk.Now()))
	for name, s := range map[string]string{"stdout": out.String(), "file": file} {
		if strings.Contains(s, "old-secret") || strings.Contains(s, "new-secret") {
			t.Errorf("%s に生の値: %q", name, s)
		}
		if !strings.Contains(s, "a=***") || !strings.Contains(s, "b=***") {
			t.Errorf("%s が *** でない: %q", name, s)
		}
	}
}

func TestAddSecretsConcurrent(t *testing.T) {
	lg, out, _, _ := newTestLogger(t)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			lg.Slog().Info("line", "v", "s0")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			lg.AddSecrets(fmt.Sprintf("s%d", i))
		}
	}()
	wg.Wait()
	lg.Slog().Info("after", "v", "s0", "w", "s299")
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	text := lines[len(lines)-1]
	if strings.Contains(text, "s0") || strings.Contains(text, "s299") {
		t.Errorf("生の値: %q", text)
	}
}
