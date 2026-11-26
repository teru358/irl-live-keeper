package sources

import (
	"bytes"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	MaxFileSize  = 5 << 20
	MaxFilesSize = 100 << 20
)

var ErrFileTooLarge = errors.New("source file exceeds size limit")
var ErrTotalTooLarge = errors.New("source files exceed total size limit")

var fileNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9_.-]{0,127}$`)
var filesMu sync.Mutex

type FileInfo struct {
	Name   string   `json:"name"`
	Size   int64    `json:"size"`
	Type   string   `json:"type"`
	UsedBy []string `json:"used_by"`
}

type FilesStore struct{ Dir string }

func SafeFileName(upload string) (string, error) {
	if upload == "" || strings.ContainsAny(upload, `/\\`) || upload == "." || upload == ".." || strings.Contains(upload, "..") {
		return "", errors.New("invalid file name")
	}
	base := filepath.Base(upload)
	if base != upload {
		return "", errors.New("invalid file name")
	}
	ext := strings.ToLower(filepath.Ext(base))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" && ext != ".webp" && ext != ".avif" && ext != ".bmp" && ext != ".svg" {
		return "", errors.New("invalid file extension")
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	var b strings.Builder
	for _, c := range stem {
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-' {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	name := b.String() + ext
	if !fileNamePattern.MatchString(name) {
		return "", errors.New("invalid file name")
	}
	return name, nil
}

func DetectImage(name string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(name))
	var typ string
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		typ = "image/png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		typ = "image/jpeg"
	case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		typ = "image/gif"
	case bytes.HasPrefix(data, []byte("RIFF")) && len(data) >= 12 && string(data[8:12]) == "WEBP":
		typ = "image/webp"
	case hasAVIFBrand(data):
		typ = "image/avif"
	case len(data) >= 2 && string(data[:2]) == "BM":
		typ = "image/bmp"
	case isSVG(data):
		typ = "image/svg+xml"
	default:
		return "", errors.New("unknown image type")
	}
	validExt := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp", ".avif": "image/avif", ".bmp": "image/bmp", ".svg": "image/svg+xml"}
	if validExt[ext] != typ {
		return "", errors.New("extension does not match image")
	}
	if typ == "image/svg+xml" {
		// SVG is delivered in a sandboxed context. The prefix check also
		// prevents accepting generic XML or HTML under an SVG extension.
	} else if typ == "image/bmp" {
		if !validBMP(data) {
			return "", errors.New("corrupt image")
		}
	} else if typ == "image/avif" {
		if !hasAVIFBrand(data) {
			return "", errors.New("corrupt image")
		}
	} else if typ != "image/webp" {
		decoded, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || decoded.Width < 1 || decoded.Height < 1 || int64(decoded.Width)*int64(decoded.Height) > 16_000_000 || format != strings.TrimPrefix(typ, "image/") && !(ext == ".jpg" && format == "jpeg") {
			return "", errors.New("corrupt image")
		}
		if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
			return "", errors.New("corrupt image")
		}
	} else {
		if len(data) < 20 {
			return "", errors.New("corrupt image")
		}
		declared := int64(binaryLittle(data[4:8])) + 8
		if declared > int64(len(data)) || declared < 20 {
			return "", errors.New("corrupt image")
		}
		tag, size := string(data[12:16]), int64(binaryLittle(data[16:20]))
		minChunk := int64(1)
		switch tag {
		case "VP8 ":
			minChunk = 10
		case "VP8L":
			minChunk = 5
			if len(data) > 20 && data[20] != 0x2f {
				return "", errors.New("corrupt image")
			}
		case "VP8X":
			minChunk = 10
		default:
			return "", errors.New("corrupt image")
		}
		if size < minChunk || size+20 > declared {
			return "", errors.New("corrupt image")
		}
	}
	return typ, nil
}

func hasAVIFBrand(data []byte) bool {
	if len(data) < 16 || string(data[4:8]) != "ftyp" {
		return false
	}
	// The four-byte box length is big endian.
	size := int64(data[0])<<24 | int64(data[1])<<16 | int64(data[2])<<8 | int64(data[3])
	if size < 16 || size > int64(len(data)) || (size-16)%4 != 0 {
		return false
	}
	if brand := string(data[8:12]); brand == "avif" || brand == "avis" {
		return true
	}
	for i := 16; i+4 <= int(size); i += 4 {
		if brand := string(data[i : i+4]); brand == "avif" || brand == "avis" {
			return true
		}
	}
	return false
}

func validBMP(data []byte) bool {
	if len(data) < 26 || string(data[:2]) != "BM" || int64(binaryLittle(data[2:6])) > int64(len(data)) {
		return false
	}
	offset := int64(binaryLittle(data[10:14]))
	dib := binaryLittle(data[14:18])
	var width, height int64
	switch dib {
	case 12: // BITMAPCOREHEADER
		width, height = int64(data[18])|int64(data[19])<<8, int64(data[20])|int64(data[21])<<8
	case 40, 52, 56, 108, 124:
		if len(data) < 54 {
			return false
		}
		w := int32(binaryLittle(data[18:22]))
		h := int32(binaryLittle(data[22:26]))
		if w <= 0 || h == 0 || h == -1<<31 {
			return false
		}
		width, height = int64(w), int64(h)
		if height < 0 {
			height = -height
		}
	default:
		return false
	}
	return width > 0 && height > 0 && width*height <= 16_000_000 && offset >= int64(14+dib) && offset <= int64(len(data))
}

func isSVG(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	s := strings.TrimLeft(string(data), " \t\r\n\f")
	for {
		if strings.HasPrefix(s, "<?xml") {
			i := strings.Index(s, "?>")
			if i < 0 {
				return false
			}
			s = strings.TrimLeft(s[i+2:], " \t\r\n\f")
			continue
		}
		if strings.HasPrefix(s, "<!--") {
			i := strings.Index(s[4:], "-->")
			if i < 0 {
				return false
			}
			s = strings.TrimLeft(s[i+7:], " \t\r\n\f")
			continue
		}
		break
	}
	return strings.HasPrefix(s, "<svg") && len(s) > 4 && (s[4] == '>' || s[4] == '/' || s[4] == ' ' || s[4] == '\t' || s[4] == '\r' || s[4] == '\n')
}
func binaryLittle(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func (s *FilesStore) List(used func(string) []string) ([]FileInfo, error) {
	filesMu.Lock()
	defer filesMu.Unlock()
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return []FileInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []FileInfo{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.Dir, e.Name()))
		if err != nil {
			return nil, err
		}
		typ, err := DetectImage(e.Name(), data)
		if err != nil {
			continue
		}
		var refs []string
		if used != nil {
			refs = used(e.Name())
		}
		out = append(out, FileInfo{Name: e.Name(), Size: int64(len(data)), Type: typ, UsedBy: refs})
	}
	return out, nil
}
func (s *FilesStore) Save(name string, data []byte) error {
	filesMu.Lock()
	defer filesMu.Unlock()
	if len(data) > MaxFileSize {
		return ErrFileTooLarge
	}
	if _, err := DetectImage(name, data); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var total int64
	for _, e := range entries {
		if !e.IsDir() {
			info, er := e.Info()
			if er != nil {
				return er
			}
			total += info.Size()
		}
	}
	if total+int64(len(data)) > MaxFilesSize {
		return ErrTotalTooLarge
	}
	if err = os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.Dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return err
	}
	return f.Close()
}
func (s *FilesStore) Open(name string) ([]byte, string, error) {
	filesMu.Lock()
	defer filesMu.Unlock()
	if !fileNamePattern.MatchString(name) || strings.ContainsAny(name, `/\\`) || name == "." || name == ".." {
		return nil, "", os.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, name))
	if err != nil {
		return nil, "", err
	}
	typ, err := DetectImage(name, data)
	return data, typ, err
}
func (s *FilesStore) Delete(name string) error {
	filesMu.Lock()
	defer filesMu.Unlock()
	if !fileNamePattern.MatchString(name) || strings.ContainsAny(name, `/\\`) || name == "." || name == ".." {
		return os.ErrNotExist
	}
	return os.Remove(filepath.Join(s.Dir, name))
}

// ReadUpload accepts one bounded multipart file field named "file".
func ReadUpload(w http.ResponseWriter, r *http.Request) (string, []byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileSize+(1<<20))
	if err := r.ParseMultipartForm(MaxFileSize); err != nil {
		return "", nil, err
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	f, h, err := r.FormFile("file")
	if err != nil {
		return "", nil, err
	}
	defer f.Close()
	name, err := SafeFileName(h.Filename)
	if err != nil {
		return "", nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
	if err != nil || len(data) > MaxFileSize {
		return "", nil, errors.New("too large")
	}
	return name, data, nil
}
