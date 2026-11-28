package sources

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// 部分の見直し（Refresh・配るときの見直し）でも、件数と合計の上限が外れない。

func TestRefreshKeepsCountLimit(t *testing.T) {
	dir := t.TempDir()
	png := samplePNG(t)
	for i := 0; i < MaxFilesCount; i++ {
		writeFile(t, dir, fmt.Sprintf("f%04d.png", i), png)
	}
	s := &FilesStore{Dir: dir}
	wantUsable(t, listMap(t, s), "f0999.png")
	// 本文が参照する1001件目を外から足す。
	writeFile(t, dir, "f1000.png", png)
	s.Refresh([]string{AssetPrefix + "f1000.png"})
	if _, _, err := s.Open("f1000.png"); err == nil {
		t.Fatal("1001件目が使える")
	}
	if got := len(s.Available()); got != MaxFilesCount {
		t.Fatalf("使えるのが %d 件", got)
	}
	if v := s.Versions([]string{AssetPrefix + "f1000.png"}); len(v) != 0 {
		t.Fatal("1001件目に版が付く", v)
	}
	m := listMap(t, s)
	wantUsable(t, m, "f0999.png")
	wantReason(t, m, "f1000.png", ReasonOverLimit)
	// 前に並ぶファイルを外から足すと、いちばん後ろが外れる。開き直した一覧と同じ。
	writeFile(t, dir, "0first.png", png)
	s.Refresh([]string{AssetPrefix + "0first.png"})
	if _, _, err := s.Open("0first.png"); err != nil {
		t.Fatal("先頭に足したのが使えない", err)
	}
	if _, _, err := s.Open("f0999.png"); err == nil {
		t.Fatal("後ろが外れない")
	}
	if got := len(s.Available()); got != MaxFilesCount {
		t.Fatalf("使えるのが %d 件", got)
	}
	m = listMap(t, s)
	wantUsable(t, m, "0first.png")
	wantReason(t, m, "f0999.png", ReasonOverLimit)
	// 消せば、場所があくので、ずっと後ろのものも使える。
	if err := os.Remove(filepath.Join(dir, "0first.png")); err != nil {
		t.Fatal(err)
	}
	s.Refresh([]string{AssetPrefix + "0first.png", AssetPrefix + "f0999.png"})
	if _, _, err := s.Open("f0999.png"); err != nil {
		t.Fatal("あいた場所に入らない", err)
	}
}

func TestRefreshKeepsTotalSizeLimit(t *testing.T) {
	for _, via := range []string{"refresh", "open"} {
		t.Run(via, func(t *testing.T) {
			dir := t.TempDir()
			big := paddedPNG(t, MaxFileSize)
			n := MaxFilesSize/MaxFileSize - 1
			for i := 0; i < n; i++ {
				writeFile(t, dir, fmt.Sprintf("m%02d.png", i), big)
			}
			writeFile(t, dir, "z1.png", samplePNG(t))
			writeFile(t, dir, "z2.png", samplePNG(t))
			s := &FilesStore{Dir: dir}
			m := listMap(t, s)
			wantUsable(t, m, "z1.png")
			wantUsable(t, m, "z2.png")
			// 既にあるファイルを大きくして、合計を超える。超えた所から後ろが使えない。
			writeFile(t, dir, "z1.png", paddedPNG(t, MaxFileSize))
			if via == "refresh" {
				s.Refresh([]string{AssetPrefix + "z1.png"})
			} else if _, _, err := s.Open("z1.png"); err != nil {
				t.Fatal("ちょうど上限の合計が使えない", err)
			}
			if _, _, err := s.Open("z1.png"); err != nil {
				t.Fatal("大きくしたファイルが使えない", err)
			}
			if _, _, err := s.Open("z2.png"); err == nil {
				t.Fatal("合計を超えた後ろが使える")
			}
			m = listMap(t, s)
			wantUsable(t, m, "z1.png")
			wantReason(t, m, "z2.png", ReasonOverLimit)
			// もう一つ大きくしても、最初に超えたところから後ろは使えないまま。
			writeFile(t, dir, "m00.png", samplePNG(t))
			s.Refresh([]string{AssetPrefix + "m00.png"})
			if _, _, err := s.Open("z2.png"); err != nil {
				t.Fatal("小さくしてあいた分が使えない", err)
			}
		})
	}
}
