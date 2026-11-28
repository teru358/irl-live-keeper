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

// FileInfo is one listed file. Name is the relative path with "/" only.
type FileInfo struct {
	Name   string   `json:"name"`
	Size   int64    `json:"size"`
	Type   string   `json:"type"`
	Usable bool     `json:"usable"`
	Reason string   `json:"reason,omitempty"`
	UsedBy []string `json:"used_by"`
}

// FilesStore is the source-files folder and its in-memory index. The zero
// value of the unexported fields is ready to use; keep one store per folder.
type FilesStore struct {
	Dir   string
	idx   *fileIndex
	scans int
}

func SafeFileName(upload string) (string, error) {
	if upload == "" || strings.ContainsAny(upload, `/\\`) || upload == "." || upload == ".." || strings.Contains(upload, "..") {
		return "", errors.New("invalid file name")
	}
	base := filepath.Base(upload)
	if base != upload {
		return "", errors.New("invalid file name")
	}
	ext := asciiLower(filepath.Ext(base))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" && ext != ".webp" && ext != ".avif" && ext != ".bmp" && ext != ".svg" {
		return "", errors.New("invalid file extension")
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	var b strings.Builder
	for _, c := range stem {
		if allowedAssetRune(c) {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	name := b.String() + ext
	if CheckPath(name) != "" {
		return "", errors.New("invalid file name")
	}
	return name, nil
}

func DetectImage(name string, data []byte) (string, error) {
	ext := asciiLower(filepath.Ext(name))
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
		return "", ErrExtensionMismatch
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

// FileList is the result of List.
type FileList struct {
	Files []FileInfo
	// ScanLimited is true when the folder had more than MaxScanEntries entries
	// and the rest was not looked at.
	ScanLimited bool
}

// List rebuilds the index and returns every file with its usable flag.
func (s *FilesStore) List(used func(string) []string) (FileList, error) {
	filesMu.Lock()
	defer filesMu.Unlock()
	idx, err := s.rebuildLocked()
	if err != nil {
		return FileList{}, err
	}
	out := FileList{Files: []FileInfo{}, ScanLimited: idx.limited}
	for _, e := range s.rows(idx) {
		info := FileInfo{Name: e.Path, Size: e.Size, Type: e.Type, Usable: e.Usable, Reason: string(e.Reason), UsedBy: []string{}}
		if e.Usable && used != nil {
			if refs := used(e.Path); refs != nil {
				info.UsedBy = refs
			}
		}
		out.Files = append(out.Files, info)
	}
	return out, nil
}

// Save writes a new file directly under the folder. It never overwrites.
func (s *FilesStore) Save(name string, data []byte) error {
	filesMu.Lock()
	defer filesMu.Unlock()
	if len(data) > MaxFileSize {
		return ErrFileTooLarge
	}
	if strings.Contains(name, "/") || CheckPath(name) != "" {
		return errors.New("invalid file name")
	}
	if _, err := DetectImage(name, data); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	idx, err := s.rebuildLocked()
	if err != nil {
		return err
	}
	var count int
	var total int64
	for _, e := range s.rows(idx) {
		if e.Reason == ReasonOverLimit {
			return ErrTotalTooLarge
		}
		if e.Usable {
			count++
			total += e.Size
		}
	}
	if count+1 > MaxFilesCount {
		return ErrTooManyFiles
	}
	if total+int64(len(data)) > MaxFilesSize {
		return ErrTotalTooLarge
	}
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return err
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	des, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return err
	}
	for _, de := range des {
		if foldName(de.Name()) == foldName(name) {
			return os.ErrExist
		}
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		_ = root.Remove(name)
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	_, _ = s.rebuildLocked()
	return nil
}

// Delete removes a file under the folder; name is the relative path with "/"
// (up to three folders). Folders themselves are never removed.
func (s *FilesStore) Delete(name string) error {
	filesMu.Lock()
	defer filesMu.Unlock()
	if !s.indexed() {
		if _, err := s.rebuildLocked(); err != nil {
			return err
		}
	}
	idxMu.Lock()
	_, known := s.idx.byPath[name]
	idxMu.Unlock()
	if !known || CheckPath(name) != "" {
		return os.ErrNotExist
	}
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return err
	}
	defer root.Close()
	s.refreshNames(root, []string{name})
	if !s.exactKnown(name) {
		return os.ErrNotExist
	}
	info, err := lstatPath(root, name)
	if err != nil {
		if errors.Is(err, errSymlink) {
			return os.ErrNotExist
		}
		return err
	}
	if info.IsDir() {
		return os.ErrNotExist
	}
	if err = root.Remove(name); err != nil {
		return err
	}
	_, _ = s.rebuildLocked()
	return nil
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
