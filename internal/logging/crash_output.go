package logging

import (
	"os"
	"path/filepath"
	"time"
)

// CrashLogPath は起動時刻を含む panic 出力の保存先を返す。
func CrashLogPath(dir string, started time.Time) string {
	return filepath.Join(dir, "crash-"+started.Format("20060102-150405")+".log")
}

// RemoveEmptyCrashLog は通常終了時に空の crash log だけを削除する。
func RemoveEmptyCrashLog(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode().IsRegular() && info.Size() == 0 {
		return os.Remove(path)
	}
	return nil
}
