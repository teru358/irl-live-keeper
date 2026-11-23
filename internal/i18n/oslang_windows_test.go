//go:build windows

package i18n

import "testing"

func TestOSLangWindowsReaderCanBeReplaced(t *testing.T) {
	old := readWindowsUILanguage
	defer func() { readWindowsUILanguage = old }()
	readWindowsUILanguage = func() string { return "ja-JP" }
	if got := OSLang(); got != JA {
		t.Fatalf("OSLang()=%q want %q", got, JA)
	}
	readWindowsUILanguage = func() string { return "en-US" }
	if got := OSLang(); got != EN {
		t.Fatalf("OSLang()=%q want %q", got, EN)
	}
}
