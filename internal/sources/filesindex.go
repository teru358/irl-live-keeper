package sources

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// MaxFilesCount is the number of usable files that are counted.
	MaxFilesCount = 1000
	// MaxScanEntries is the number of directory entries one scan looks at.
	MaxScanEntries = 10000
	// MaxFolderDepth is the number of folder levels below source-files/.
	MaxFolderDepth = 3
	// MaxPathLength is the length of a whole relative path.
	MaxPathLength = 512
)

var ErrTooManyFiles = errors.New("source files exceed count limit")
var ErrExtensionMismatch = errors.New("extension does not match image")

// Reason is the code explaining why a file is not usable.
type Reason string

const (
	ReasonBadName      Reason = "bad_name"
	ReasonReservedName Reason = "reserved_name"
	ReasonBadExtension Reason = "bad_extension"
	ReasonPathTooLong  Reason = "path_too_long"
	ReasonTooDeep      Reason = "too_deep"
	ReasonTooLarge     Reason = "too_large"
	ReasonNotImage     Reason = "not_image"
	ReasonExtMismatch  Reason = "ext_mismatch"
	ReasonSymlink      Reason = "symlink"
	ReasonNotRegular   Reason = "not_regular"
	ReasonUnreadable   Reason = "unreadable"
	ReasonOverLimit    Reason = "over_limit"
)

// Reasons lists every reason code.
var Reasons = []Reason{ReasonBadName, ReasonReservedName, ReasonBadExtension, ReasonPathTooLong, ReasonTooDeep, ReasonTooLarge, ReasonNotImage, ReasonExtMismatch, ReasonSymlink, ReasonNotRegular, ReasonUnreadable, ReasonOverLimit}

// fileEntry is one row of the index. Path uses "/" only.
type fileEntry struct {
	Path    string
	Size    int64
	ModTime time.Time
	Type    string
	Usable  bool
	Reason  Reason
}

// fileIndex is the remembered list of the folder. Its entries are guarded by idxMu.
type fileIndex struct {
	entries []*fileEntry
	byPath  map[string]*fileEntry
	limited bool
}

var idxMu sync.Mutex

var imageExtensions = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".avif": true, ".bmp": true, ".svg": true}

func isReservedName(seg string) bool {
	stem := seg
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	stem = strings.ToUpper(stem)
	switch stem {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9'
}

func checkSegment(seg string) Reason {
	if !fileNamePattern.MatchString(seg) || strings.HasSuffix(seg, ".") {
		return ReasonBadName
	}
	if isReservedName(seg) {
		return ReasonReservedName
	}
	return ""
}

// CheckPath validates a relative file path ("/" separated, at most three
// folders). It returns "" when the path is acceptable.
func CheckPath(rel string) Reason {
	if rel == "" || strings.ContainsRune(rel, '\\') || strings.HasPrefix(rel, "/") {
		return ReasonBadName
	}
	segs := strings.Split(rel, "/")
	if len(segs) > MaxFolderDepth+1 {
		return ReasonTooDeep
	}
	if len(rel) > MaxPathLength {
		return ReasonPathTooLong
	}
	for _, s := range segs {
		if r := checkSegment(s); r != "" {
			return r
		}
	}
	if !imageExtensions[strings.ToLower(extOf(segs[len(segs)-1]))] {
		return ReasonBadExtension
	}
	return ""
}

func extOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i:]
	}
	return ""
}

func reasonOfDetect(err error) Reason {
	if errors.Is(err, ErrExtensionMismatch) {
		return ReasonExtMismatch
	}
	return ReasonNotImage
}

// checkContent fills the usable fields of e from the bytes of the file.
func (e *fileEntry) checkContent(data []byte) {
	typ, err := DetectImage(e.Path, data)
	if err != nil {
		e.Type, e.Usable, e.Reason = "", false, reasonOfDetect(err)
		return
	}
	e.Type, e.Usable, e.Reason = typ, true, ""
}

var errSymlink = errors.New("symbolic link")
var errNotRegular = errors.New("not a regular file")

// lstatPath checks that no component of rel is a link and returns the info of
// the last one.
func lstatPath(root *os.Root, rel string) (fs.FileInfo, error) {
	segs := strings.Split(rel, "/")
	var info fs.FileInfo
	for i := range segs {
		var err error
		info, err = root.Lstat(strings.Join(segs[:i+1], "/"))
		if err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, errSymlink
		}
	}
	return info, nil
}

// readRegular opens rel through root, checks with the opened handle that it is
// a regular file (the same one as want, when given) and returns its bytes (at
// most MaxFileSize+1). Bytes are nil when the file is larger than MaxFileSize.
func readRegular(root *os.Root, rel string, want fs.FileInfo) ([]byte, fs.FileInfo, error) {
	f, err := root.Open(rel)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || want != nil && !os.SameFile(want, info) {
		return nil, nil, errNotRegular
	}
	if info.Size() > MaxFileSize {
		return nil, info, nil
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
	if data == nil {
		data = []byte{}
	}
	return data, info, err
}

type scanner struct {
	root    *os.Root
	entries []*fileEntry
	visited int
	limited bool
}

func (sc *scanner) add(e *fileEntry) { sc.entries = append(sc.entries, e) }

func (sc *scanner) walk(dir string, folders int) {
	name := dir
	if name == "" {
		name = "."
	}
	f, err := sc.root.Open(name)
	if err != nil {
		if dir != "" {
			sc.add(&fileEntry{Path: dir, Reason: ReasonUnreadable})
		}
		return
	}
	remaining := MaxScanEntries - sc.visited
	des, _ := f.ReadDir(remaining + 1)
	_ = f.Close()
	if len(des) > remaining {
		des = des[:remaining]
		sc.limited = true
	}
	sort.Slice(des, func(i, j int) bool { return des[i].Name() < des[j].Name() })
	for _, de := range des {
		sc.visited++
		rel := de.Name()
		if dir != "" {
			rel = dir + "/" + rel
		}
		sc.visit(rel, de, folders)
	}
}

func (sc *scanner) visit(rel string, de fs.DirEntry, folders int) {
	typ := de.Type()
	switch {
	case typ&fs.ModeSymlink != 0:
		sc.add(&fileEntry{Path: rel, Reason: ReasonSymlink})
	case typ.IsDir():
		if folders+1 > MaxFolderDepth {
			sc.add(&fileEntry{Path: rel, Reason: ReasonTooDeep})
			return
		}
		sc.walk(rel, folders+1)
	case typ.IsRegular():
		sc.visitFile(rel)
	default:
		sc.add(&fileEntry{Path: rel, Reason: ReasonNotRegular})
	}
}

func (sc *scanner) visitFile(rel string) {
	e := &fileEntry{Path: rel}
	sc.add(e)
	if r := CheckPath(rel); r != "" {
		e.Reason = r
		return
	}
	info, err := sc.root.Lstat(rel)
	if err != nil {
		e.Reason = ReasonUnreadable
		return
	}
	e.Size, e.ModTime = info.Size(), info.ModTime()
	if info.Mode()&fs.ModeSymlink != 0 {
		e.Reason = ReasonSymlink
		return
	}
	if !info.Mode().IsRegular() {
		e.Reason = ReasonNotRegular
		return
	}
	if info.Size() > MaxFileSize {
		e.Reason = ReasonTooLarge
		return
	}
	data, finfo, err := readRegular(sc.root, rel, info)
	if err != nil {
		if errors.Is(err, errNotRegular) {
			e.Reason = ReasonNotRegular
		} else {
			e.Reason = ReasonUnreadable
		}
		return
	}
	if data == nil || len(data) > MaxFileSize {
		e.Reason = ReasonTooLarge
		return
	}
	e.Size, e.ModTime = int64(len(data)), finfo.ModTime()
	e.checkContent(data)
}

// scan walks the folder once. The caller holds filesMu.
func (s *FilesStore) scan() (*fileIndex, error) {
	idxMu.Lock()
	s.scans++
	idxMu.Unlock()
	idx := &fileIndex{byPath: map[string]*fileEntry{}}
	root, err := os.OpenRoot(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return idx, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	sc := &scanner{root: root}
	sc.walk("", 0)
	sort.Slice(sc.entries, func(i, j int) bool { return sc.entries[i].Path < sc.entries[j].Path })
	var count int
	var total int64
	exceeded := false
	for _, e := range sc.entries {
		if !e.Usable {
			continue
		}
		if !exceeded && (count+1 > MaxFilesCount || total+e.Size > MaxFilesSize) {
			exceeded = true
		}
		if exceeded {
			e.Type, e.Usable, e.Reason = "", false, ReasonOverLimit
			continue
		}
		count++
		total += e.Size
	}
	idx.entries, idx.limited = sc.entries, sc.limited
	for _, e := range sc.entries {
		idx.byPath[e.Path] = e
	}
	return idx, nil
}

// rebuildLocked scans the folder and replaces the index. The caller holds filesMu.
func (s *FilesStore) rebuildLocked() (*fileIndex, error) {
	idx, err := s.scan()
	if err != nil {
		return nil, err
	}
	idxMu.Lock()
	s.idx = idx
	idxMu.Unlock()
	return idx, nil
}

// Rebuild scans the folder and replaces the index.
func (s *FilesStore) Rebuild() error {
	filesMu.Lock()
	defer filesMu.Unlock()
	_, err := s.rebuildLocked()
	return err
}

// EnsureIndex builds the index when it has not been built yet.
func (s *FilesStore) EnsureIndex() error {
	if s.indexed() {
		return nil
	}
	filesMu.Lock()
	defer filesMu.Unlock()
	if s.indexed() {
		return nil
	}
	_, err := s.rebuildLocked()
	return err
}

func (s *FilesStore) indexed() bool {
	idxMu.Lock()
	defer idxMu.Unlock()
	return s.idx != nil
}

// ScanCount is the number of times the folder has been scanned.
func (s *FilesStore) ScanCount() int {
	idxMu.Lock()
	defer idxMu.Unlock()
	return s.scans
}

func (s *FilesStore) lookup(name string) (fileEntry, bool, error) {
	if err := s.EnsureIndex(); err != nil {
		return fileEntry{}, false, err
	}
	idxMu.Lock()
	defer idxMu.Unlock()
	e, ok := s.idx.byPath[name]
	if !ok {
		return fileEntry{}, false, nil
	}
	return *e, true, nil
}

// update stores a re-validated entry in the index.
func (s *FilesStore) update(name string, e fileEntry) {
	idxMu.Lock()
	defer idxMu.Unlock()
	if s.idx != nil {
		if cur, ok := s.idx.byPath[name]; ok {
			*cur = e
		}
	}
}

// Open returns the bytes and type of a usable file. name must equal an index
// path exactly. Only that file is opened.
func (s *FilesStore) Open(name string) ([]byte, string, error) {
	e, ok, err := s.lookup(name)
	if err != nil {
		return nil, "", err
	}
	if !ok || !e.Usable {
		return nil, "", os.ErrNotExist
	}
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return nil, "", os.ErrNotExist
	}
	defer root.Close()
	lst, err := lstatPath(root, name)
	if err != nil {
		return nil, "", os.ErrNotExist
	}
	data, info, err := readRegular(root, name, lst)
	if err != nil {
		return nil, "", os.ErrNotExist
	}
	if info.Size() == e.Size && info.ModTime().Equal(e.ModTime) && int64(len(data)) == e.Size {
		return data, e.Type, nil
	}
	// The file changed after the index was made: re-validate only this file.
	ne := fileEntry{Path: name, Size: info.Size(), ModTime: info.ModTime()}
	if data == nil || len(data) > MaxFileSize {
		ne.Reason = ReasonTooLarge
	} else {
		ne.Size = int64(len(data))
		ne.checkContent(data)
	}
	s.update(name, ne)
	if !ne.Usable {
		return nil, "", os.ErrNotExist
	}
	return data, ne.Type, nil
}
