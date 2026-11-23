//go:build !windows

package i18n

import (
	"os"
	"strings"
)

var readLanguageEnv = os.Getenv

// OSLang は LC_ALL、LC_MESSAGES、LANG の順に OS の言語を調べる。
func OSLang() Lang { return osLang(readLanguageEnv) }

func osLang(getenv func(string) string) Lang {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		value := strings.TrimSpace(getenv(key))
		if value == "" {
			continue
		}
		if value == "C" || value == "POSIX" {
			return EN
		}
		if strings.HasPrefix(strings.ToLower(value), "ja") {
			return JA
		}
		return EN
	}
	return EN
}
