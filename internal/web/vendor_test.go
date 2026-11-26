package web

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"strings"
	"testing"
)

// TestVendorFilesMatchRecord は、同梱した外のファイル（assets/vendor）が VENDOR.txt の sha256 と合うこと、
// 記録にないファイルを置いていないことを確かめる（spec v0.20 2章）。
func TestVendorFilesMatchRecord(t *testing.T) {
	record, err := assets.ReadFile("assets/vendor/VENDOR.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(string(record)))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) != 6 || !strings.HasPrefix(cols[3], "sha256:") {
			t.Fatalf("VENDOR.txt の行の形が違う: %q", line)
		}
		want[cols[0]] = strings.TrimPrefix(cols[3], "sha256:")
	}
	if len(want) == 0 {
		t.Fatal("VENDOR.txt に行がない")
	}
	for path, sum := range want {
		data, err := assets.ReadFile("assets/vendor/" + path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		got := sha256.Sum256(data)
		if hex.EncodeToString(got[:]) != sum {
			t.Errorf("%s: sha256 が記録と違う（手を加えた？）", path)
		}
	}
	// 記録の対象のフォルダ（lit-html と codemirror）に、記録にないファイルを置いていない
	for _, dir := range []string{"assets/vendor/lit-html-3.3.3", "assets/vendor/codemirror"} {
		err := fs.WalkDir(assets, dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if _, ok := want[strings.TrimPrefix(p, "assets/vendor/")]; !ok {
				t.Errorf("%s が VENDOR.txt にない", p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
