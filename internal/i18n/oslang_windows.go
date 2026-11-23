//go:build windows

package i18n

import (
	"strings"

	"golang.org/x/sys/windows"
)

var readWindowsUILanguage = func() string {
	languages, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(languages) == 0 {
		return ""
	}
	return languages[0]
}

// OSLang は Windows のユーザー既定表示言語を返す。
func OSLang() Lang { return windowsLang(readWindowsUILanguage()) }

func windowsLang(name string) Lang {
	if strings.HasPrefix(strings.ToLower(name), "ja") {
		return JA
	}
	return EN
}
