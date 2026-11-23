// Package single は置き場所ごとの Windows の単一起動を扱う。
package single

import (
	"crypto/sha256"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Normalize は解決済みの Windows パスの表記をそろえる。OS の API に依存しない。
func Normalize(resolved string) string {
	s := strings.ReplaceAll(resolved, `\`, "/")
	return strings.ToLower(path.Clean(s))
}
func Hash(resolved string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(Normalize(resolved))))
}
func DirectoryHash(dir string) (string, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return Hash(resolved), nil
}
func MutexName(hash string) string { return `Local\irl-live-keeper-` + hash }
func ClassName(hash string) string { return "irl-live-keeper-" + hash }
