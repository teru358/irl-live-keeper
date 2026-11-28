package sources

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestAssetNamesShared(t *testing.T) {
	b, err := os.ReadFile("asset_name_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string
		Reason Reason
	}
	if err = json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			if got := CheckPath(c.Name); got != c.Reason {
				t.Fatalf("%q: %s want %s", c.Name, got, c.Reason)
			}
		})
	}
	if CheckPath(string([]byte{0xff})+".png") != ReasonBadName {
		t.Fatal("invalid UTF-8")
	}
}

func TestCollisionSurvivesRefresh(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"A.png", "a.png"} {
		if err := os.WriteFile(dir+"/"+n, samplePNG(t), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s := &FilesStore{Dir: dir}
	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"A.png", "a.png"} {
		e, _, _ := s.lookup(n)
		if e.Reason != "name_collision" {
			t.Fatalf("%s: %+v", n, e)
		}
	}
	s.Refresh([]string{AssetPrefix + "A.png"})
	if _, _, err := s.Open("A.png"); err == nil {
		t.Fatal("collision served")
	}
}

func TestParentCollisionAndRecovery(t *testing.T) {
	dir := t.TempDir()
	data := samplePNG(t)
	for _, n := range []string{"A", "a"} {
		os.Mkdir(dir+"/"+n, 0700)
		os.WriteFile(dir+"/"+n+"/x.png", data, 0600)
	}
	s := &FilesStore{Dir: dir}
	s.Rebuild()
	if len(s.Available()) != 0 {
		t.Fatal("parent collision available")
	}
	os.RemoveAll(dir + "/a")
	s.Refresh([]string{AssetPrefix + "A/x.png"})
	if !s.Available()[AssetPrefix+"A/x.png"] {
		t.Fatal("parent did not recover")
	}
	if _, _, err := s.Open("a/x.png"); err == nil {
		t.Fatal("alias opened")
	}
	if err := s.Delete("a/x.png"); err == nil {
		t.Fatal("missing indexed alias deleted")
	}
}

func TestRootFailureRevokesIndex(t *testing.T) {
	dir := t.TempDir()
	s := &FilesStore{Dir: dir + "/files"}
	os.Mkdir(s.Dir, 0700)
	os.WriteFile(s.Dir+"/x.png", samplePNG(t), 0600)
	s.Rebuild()
	os.Rename(s.Dir, dir+"/moved")
	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if len(s.Available()) != 0 {
		t.Fatal("stale usable")
	}
}

func TestJapaneseSaveOpenDelete(t *testing.T) {
	s := &FilesStore{Dir: t.TempDir()}
	data := samplePNG(t)
	if err := s.Save("夕焼け.png", data); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Open("夕焼け.png"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("夕焼け.png"); err != nil {
		t.Fatal(err)
	}
}

func TestScanLimitRevokesAllRootChildren(t *testing.T) {
	dir := t.TempDir()
	s := &FilesStore{Dir: dir}
	os.WriteFile(dir+"/x.png", samplePNG(t), 0600)
	s.Rebuild()
	for i := 0; i < MaxScanEntries; i++ {
		if err := os.WriteFile(fmt.Sprintf("%s/f%05d", dir, i), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if len(s.Available()) != 0 || len(s.Versions([]string{AssetPrefix + "x.png"})) != 0 {
		t.Fatal("incomplete sibling scan served files")
	}
}

func TestNameErrorPriority(t *testing.T) {
	if got := CheckPath("CON/か\u3099.png"); got != ReasonBadName {
		t.Fatalf("got %s", got)
	}
}
func TestRebuildFailureRevokesOldIndex(t *testing.T) {
	dir := t.TempDir()
	s := &FilesStore{Dir: dir + "/files"}
	os.Mkdir(s.Dir, 0700)
	os.WriteFile(s.Dir+"/x.png", samplePNG(t), 0600)
	s.Rebuild()
	os.Rename(s.Dir, dir+"/old")
	os.WriteFile(s.Dir, nil, 0600)
	if err := s.Rebuild(); err == nil {
		t.Fatal("expected open error")
	}
	if len(s.Available()) != 0 {
		t.Fatal("old usable survived error")
	}
}
