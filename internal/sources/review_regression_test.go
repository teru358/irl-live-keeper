package sources

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMissingFolderRefresh(t *testing.T) {
	for _, initial := range []string{"EnsureIndex", "List"} {
		t.Run(initial, func(t *testing.T) {
			s := &FilesStore{Dir: filepath.Join(t.TempDir(), "files")}
			if initial == "List" {
				if _, err := s.List(nil); err != nil {
					t.Fatal(err)
				}
			} else if err := s.EnsureIndex(); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(s.Dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(s.Dir, "x.png"), samplePNG(t), 0600); err != nil {
				t.Fatal(err)
			}
			s.Refresh([]string{AssetPrefix + "x.png"})
			if !s.Available()[AssetPrefix+"x.png"] {
				t.Fatal("restored file unavailable")
			}
		})
	}
}
func TestReviewLeadingDot(t *testing.T) {
	for _, stem := range []string{"夕焼け", "x"} {
		got, err := SafeFileName("." + stem + ".png")
		if err != nil || got != "_"+stem+".png" {
			t.Fatalf("%q %v", got, err)
		}
	}
}
func TestReviewDeleteBadExtension(t *testing.T) {
	s := &FilesStore{Dir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(s.Dir, "memo.txt"), []byte("memo"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureIndex(); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("MEMO.txt"); !os.IsNotExist(err) {
		t.Fatalf("alias: %v", err)
	}
	if err := s.Delete("memo.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "memo.txt")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
func TestReviewOverLimitCollision(t *testing.T) {
	s := &FilesStore{Dir: t.TempDir()}
	data := samplePNG(t)
	for i := 0; i < MaxFilesCount+2; i++ {
		if err := os.WriteFile(filepath.Join(s.Dir, fmt.Sprintf("f%04d.png", i)), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	last := fmt.Sprintf("f%04d.png", MaxFilesCount+1)
	alias := "F" + last[1:]
	if err := os.WriteFile(filepath.Join(s.Dir, alias), data, 0600); err != nil {
		t.Fatal(err)
	}
	s.Refresh([]string{AssetPrefix + last})
	if err := os.WriteFile(filepath.Join(s.Dir, last), append(data, 0), 0600); err != nil {
		t.Fatal(err)
	}
	s.Refresh([]string{AssetPrefix + last})
	if err := os.Remove(filepath.Join(s.Dir, alias)); err != nil {
		t.Fatal(err)
	}
	s.Refresh([]string{AssetPrefix + last})
	e, _, _ := s.peek(last)
	if e.Usable || e.ContentReason != ReasonOverLimit {
		t.Fatalf("%+v", e)
	}
	if len(s.Available()) > MaxFilesCount {
		t.Fatal("usable limit exceeded")
	}
}
func TestReviewASCIIURL(t *testing.T) {
	if CheckHTMLURL("DATA:İMAGE/png,x", "rendered", false) {
		t.Fatal("Unicode case fold accepted")
	}
}
