package sources

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAssetPolicyConstants(t *testing.T) {
	var rules struct {
		Asset struct {
			MaxSegmentLength, MaxPathLength int
			Extensions                      []string
		}
	}
	if err := json.Unmarshal(HTMLRules, &rules); err != nil {
		t.Fatal(err)
	}
	if rules.Asset.MaxSegmentLength != 128 || rules.Asset.MaxPathLength != MaxPathLength {
		t.Fatal("length constants differ from policy")
	}
	want := map[string]bool{}
	for _, ext := range rules.Asset.Extensions {
		want["."+ext] = true
	}
	if !reflect.DeepEqual(want, imageExtensions) {
		t.Fatalf("extensions: %v != %v", want, imageExtensions)
	}
}
func TestAssetGeneratorReproducible(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	root := t.TempDir()
	copyFile := func(rel string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("../..", rel))
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	copyFile("scripts/generate-asset-name-ranges.py")
	for _, name := range []string{"UnicodeData.txt", "DerivedCoreProperties.txt", "DerivedNormalizationProps.txt"} {
		copyFile("scripts/unicode/15.0.0/" + name)
	}
	copyFile("internal/sources/html_rules.json")
	cmd := exec.Command(python, filepath.Join(root, "scripts/generate-asset-name-ranges.py"))
	cmd.Env = append(os.Environ(), "LC_ALL=C", "PYTHONUTF8=0", "PYTHONCOERCECLOCALE=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(root, "internal/sources/html_rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	var before, after map[string]json.RawMessage
	if err := json.Unmarshal(HTMLRules, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &after); err != nil {
		t.Fatal(err)
	}
	var a, b any
	if err := json.Unmarshal(before["asset"], &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after["asset"], &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("regenerated asset differs")
	}
}
