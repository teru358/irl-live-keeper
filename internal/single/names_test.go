package single

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashNormalization(t *testing.T) {
	want := Hash(`C:\Data\Keeper`)
	for _, p := range []string{`c:\data\keeper\`, `C:\DATA\Keeper\.`, `C:\Data\temp\..\Keeper`, `c:/data/keeper`} {
		if Hash(p) != want {
			t.Errorf("%s: hash differs", p)
		}
	}
	if Hash(`C:\Data\Other`) == want {
		t.Fatal("different directory")
	}
	if MutexName(want) != `Local\irl-live-keeper-`+want {
		t.Fatal("mutex name")
	}
	if ClassName(want) != "irl-live-keeper-"+want {
		t.Fatal("class name")
	}
}
func TestDirectoryHashResolvesLinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	a, err := DirectoryHash(target)
	if err != nil {
		t.Fatal(err)
	}
	b, err := DirectoryHash(link)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("symlink hash differs")
	}
}
