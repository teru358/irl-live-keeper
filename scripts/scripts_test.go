package scripts

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Windows PowerShell 5.1 は BOM のない .ps1 を Shift_JIS として読むので、
// 日本語を含むスクリプトは UTF-8 の BOM つきでなければ構文エラーになる（T-15 の walk-log.ps1）。
func TestPowerShellScriptsWithNonASCIIHaveBOM(t *testing.T) {
	files, err := filepath.Glob("*.ps1")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no .ps1 files found")
	}
	bom := []byte{0xEF, 0xBB, 0xBF}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		ascii := true
		for _, c := range b {
			if c >= 0x80 {
				ascii = false
				break
			}
		}
		if !ascii && !bytes.HasPrefix(b, bom) {
			t.Errorf("%s: 日本語を含むが UTF-8 の BOM がない（Windows PowerShell 5.1 で構文エラーになる）", f)
		}
	}
}
