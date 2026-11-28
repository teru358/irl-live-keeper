package sources

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
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
	ReasonNameCollision Reason = "name_collision"
	ReasonBadName       Reason = "bad_name"
	ReasonReservedName  Reason = "reserved_name"
	ReasonBadExtension  Reason = "bad_extension"
	ReasonPathTooLong   Reason = "path_too_long"
	ReasonTooDeep       Reason = "too_deep"
	ReasonTooLarge      Reason = "too_large"
	ReasonNotImage      Reason = "not_image"
	ReasonExtMismatch   Reason = "ext_mismatch"
	ReasonSymlink       Reason = "symlink"
	ReasonNotRegular    Reason = "not_regular"
	ReasonUnreadable    Reason = "unreadable"
	ReasonOverLimit     Reason = "over_limit"
)

// Reasons lists every reason code.
var Reasons = []Reason{ReasonNameCollision, ReasonBadName, ReasonReservedName, ReasonBadExtension, ReasonPathTooLong, ReasonTooDeep, ReasonTooLarge, ReasonNotImage, ReasonExtMismatch, ReasonSymlink, ReasonNotRegular, ReasonUnreadable, ReasonOverLimit}

// fileEntry is one row of the index. Path uses "/" only.
type fileEntry struct {
	Path          string
	Size          int64
	ModTime       time.Time
	Hash          string
	Type          string
	Usable        bool
	Reason        Reason
	ContentReason Reason
}

// fileIndex is the remembered list of the folder. Its entries are guarded by idxMu.
type fileIndex struct {
	entries []*fileEntry
	byPath  map[string]*fileEntry
	limited bool
	parents map[string]*parentNames
}

var idxMu sync.Mutex

var imageExtensions = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".avif": true, ".bmp": true, ".svg": true}

func isReservedName(seg string) bool {
	stem := seg
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	return has(htmlRules.Asset.ReservedNames, asciiLower(stem))
}

func checkSegment(seg string) Reason {
	if !utf8.ValidString(seg) || len(seg) == 0 || len(seg) > 128 || strings.HasPrefix(seg, ".") || strings.HasSuffix(seg, ".") || strings.Contains(seg, "..") {
		return ReasonBadName
	}
	for _, c := range seg {
		if !allowedAssetRune(c) {
			return ReasonBadName
		}
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
		if r := checkSegment(s); r != "" && r != ReasonReservedName {
			return r
		}
	}
	for _, s := range segs {
		if isReservedName(s) {
			return ReasonReservedName
		}
	}
	if !imageExtensions[asciiLower(extOf(segs[len(segs)-1]))] {
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
	sum := sha256.Sum256(data)
	e.Hash = hex.EncodeToString(sum[:])
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
	parents map[string]*parentNames
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
	des, readErr := f.ReadDir(remaining + 1)
	complete := readErr == nil || readErr == io.EOF
	_ = f.Close()
	if len(des) > remaining {
		des = des[:remaining]
		sc.limited = true
		complete = false
	}
	sc.parents[dir] = makeParentNames(des, complete)
	sort.Slice(des, func(i, j int) bool { return des[i].Name() < des[j].Name() })
	for _, de := range des {
		if sc.visited >= MaxScanEntries {
			sc.limited = true
			break
		}
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
	sc := &scanner{root: root, parents: map[string]*parentNames{}}
	sc.walk("", 0)
	sort.Slice(sc.entries, func(i, j int) bool { return sc.entries[i].Path < sc.entries[j].Path })
	idx.parents = sc.parents
	for _, e := range sc.entries {
		e.ContentReason = e.Reason
		applyNames(idx, e)
	}
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
			e.ContentReason = ReasonOverLimit
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
		s.invalidateNames()
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
		e.ContentReason = e.Reason
		applyNames(s.idx, &e)
		if cur, ok := s.idx.byPath[name]; ok {
			*cur = e
		}
	}
}

// Open returns the bytes and type of a usable file. name must equal an index
// path exactly. Only that file is opened.
func (s *FilesStore) Open(name string) ([]byte, string, error) {
	data, typ, _, err := s.OpenWithHash(name)
	return data, typ, err
}

// OpenWithHash returns the bytes, MIME type, and lowercase SHA-256 digest of
// a usable indexed file. It checks the bytes on every open so same-size edits
// with preserved timestamps are detected as well.
func (s *FilesStore) OpenWithHash(name string) ([]byte, string, string, error) {
	if err := s.EnsureIndex(); err != nil {
		return nil, "", "", err
	}
	filesMu.Lock()
	defer filesMu.Unlock()
	e, ok, err := s.lookup(name)
	if err != nil {
		return nil, "", "", err
	}
	if !ok || !e.Usable {
		return nil, "", "", os.ErrNotExist
	}
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return nil, "", "", os.ErrNotExist
	}
	defer root.Close()
	lst, err := lstatPath(root, name)
	if err != nil {
		return nil, "", "", os.ErrNotExist
	}
	data, info, err := readRegular(root, name, lst)
	if err != nil {
		return nil, "", "", os.ErrNotExist
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	if hash == e.Hash && info.Size() == e.Size && info.ModTime().Equal(e.ModTime) && int64(len(data)) == e.Size {
		return data, e.Type, hash, nil
	}
	// The file changed after the index was made: re-validate only this file.
	ne := revalidatedEntry(name, data, info)
	s.update(name, ne)
	if !ne.Usable {
		return nil, "", "", os.ErrNotExist
	}
	return data, ne.Type, ne.Hash, nil
}

// revalidatedEntry makes the index row of a file again from the bytes read
// through an opened handle (data is nil when the file is too large).
func revalidatedEntry(name string, data []byte, info fs.FileInfo) fileEntry {
	ne := fileEntry{Path: name, Size: info.Size(), ModTime: info.ModTime()}
	if data == nil || len(data) > MaxFileSize {
		ne.Reason = ReasonTooLarge
		return ne
	}
	ne.Size = int64(len(data))
	ne.checkContent(data)
	return ne
}

// put stores a row in the index, adding it when the file was not known.
func (s *FilesStore) put(name string, e fileEntry) {
	idxMu.Lock()
	defer idxMu.Unlock()
	if s.idx == nil {
		return
	}
	e.ContentReason = e.Reason
	applyNames(s.idx, &e)
	if cur, ok := s.idx.byPath[name]; ok {
		*cur = e
		return
	}
	ne := e
	s.idx.entries = append(s.idx.entries, &ne)
	s.idx.byPath[name] = &ne
}

// peek returns a copy of the index row of a file, without building the index.
func (s *FilesStore) peek(name string) (fileEntry, bool, bool) {
	idxMu.Lock()
	defer idxMu.Unlock()
	if s.idx == nil {
		return fileEntry{}, false, false
	}
	e, ok := s.idx.byPath[name]
	if !ok {
		return fileEntry{}, false, true
	}
	return *e, true, true
}

// AssetPrefix is the URL path in front of the relative path of an asset.
const AssetPrefix = "/source/files/"

// Versions gives the ?v= version (the first 12 hex digits of the content
// SHA-256) of the usable assets among paths ("/source/files/<path>"). It only
// reads the remembered index: it never scans the folder, and returns nil while
// the index has not been built.
func (s *FilesStore) Versions(paths []string) map[string]string {
	idxMu.Lock()
	defer idxMu.Unlock()
	if s.idx == nil {
		return nil
	}
	out := map[string]string{}
	for _, p := range paths {
		if e, ok := s.idx.byPath[strings.TrimPrefix(p, AssetPrefix)]; ok && strings.HasPrefix(p, AssetPrefix) && e.Usable && len(e.Hash) >= 12 {
			out[p] = e.Hash[:12]
		}
	}
	return out
}

// Available lists the usable assets as "/source/files/<path>". It builds the
// index when that has not been done. A nil set means the folder cannot be read.
func (s *FilesStore) Available() map[string]bool {
	if err := s.EnsureIndex(); err != nil {
		return nil
	}
	idxMu.Lock()
	defer idxMu.Unlock()
	out := map[string]bool{}
	for _, e := range s.idx.entries {
		if e.Usable {
			out[AssetPrefix+e.Path] = true
		}
	}
	return out
}

// Refresh looks at the assets among paths ("/source/files/<path>") with a stat.
// A file whose size or modification time differs from the index (or that is
// not in the index yet, or is gone) is read again and its row replaced. It
// returns true when a usable asset appeared, changed its content or type, or
// stopped being usable. It refreshes the target parents and ancestors under
// filesMu; idxMu is held only when publishing the resulting state.
func (s *FilesStore) Refresh(paths []string) bool {
	filesMu.Lock()
	defer filesMu.Unlock()
	rels := make([]string, 0, len(paths))
	for _, p := range paths {
		if rel := strings.TrimPrefix(p, AssetPrefix); rel != p && CheckPath(rel) == "" {
			rels = append(rels, rel)
		}
	}
	if len(rels) == 0 {
		return false
	}
	if _, _, built := s.peek(rels[0]); !built {
		return false
	}
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		s.invalidateNames()
		return true
	}
	defer root.Close()
	changed := s.refreshNames(root, rels)
	for _, rel := range rels {
		if !s.exactKnown(rel) {
			continue
		}
		old, known, _ := s.peek(rel)
		lst, err := lstatPath(root, rel)
		if err != nil {
			if known && old.Usable {
				s.put(rel, fileEntry{Path: rel, Reason: ReasonUnreadable})
				changed = true
			}
			continue
		}
		// A row over the limit stays so until the next rebuild counts the files again.
		if known && (old.Reason == ReasonOverLimit || lst.Size() == old.Size && lst.ModTime().Equal(old.ModTime)) {
			continue
		}
		data, info, err := readRegular(root, rel, lst)
		var ne fileEntry
		switch {
		case errors.Is(err, errNotRegular):
			ne = fileEntry{Path: rel, Reason: ReasonNotRegular}
		case err != nil:
			ne = fileEntry{Path: rel, Reason: ReasonUnreadable}
		default:
			ne = revalidatedEntry(rel, data, info)
		}
		s.put(rel, ne)
		if (known && old.Usable || ne.Usable) && (old.Usable != ne.Usable || old.Hash != ne.Hash || old.Type != ne.Type) {
			changed = true
		}
	}
	return changed
}

// rows copies the rows of an index. Refresh can replace a row while the index
// is in use, so rows are read under idxMu.
func (s *FilesStore) rows(idx *fileIndex) []fileEntry {
	idxMu.Lock()
	defer idxMu.Unlock()
	out := make([]fileEntry, 0, len(idx.entries))
	for _, e := range idx.entries {
		out = append(out, *e)
	}
	return out
}

// parentNames describes a complete sibling set, independently of file content.
type parentNames struct {
	complete   bool
	names      map[string]bool
	collisions map[string]bool
}

func foldName(s string) string {
	return strings.Map(func(c rune) rune {
		min := c
		for n := unicode.SimpleFold(c); n != c; n = unicode.SimpleFold(n) {
			if n < min {
				min = n
			}
		}
		return min
	}, s)
}
func makeParentNames(des []fs.DirEntry, complete bool) *parentNames {
	p := &parentNames{complete: complete, names: map[string]bool{}, collisions: map[string]bool{}}
	groups := map[string][]string{}
	for _, de := range des {
		n := de.Name()
		p.names[n] = true
		if checkSegment(n) == "" {
			k := foldName(n)
			groups[k] = append(groups[k], n)
		}
	}
	for _, ns := range groups {
		if len(ns) > 1 {
			for _, n := range ns {
				p.collisions[n] = true
			}
		}
	}
	return p
}
func nameReason(idx *fileIndex, path string) Reason {
	r := CheckPath(path)
	if r != "" && r != ReasonBadExtension {
		return r
	}
	parts := strings.Split(path, "/")
	dir := ""
	for _, n := range parts {
		p := idx.parents[dir]
		if p == nil || !p.complete {
			return ReasonUnreadable
		}
		if !p.names[n] {
			return ReasonUnreadable
		}
		if p.collisions[n] {
			return ReasonNameCollision
		}
		if dir != "" {
			dir += "/"
		}
		dir += n
	}
	return r
}
func applyNames(idx *fileIndex, e *fileEntry) {
	e.Reason = e.ContentReason
	if e.ContentReason == ReasonSymlink || e.ContentReason == ReasonNotRegular || e.ContentReason == ReasonTooDeep {
		e.Usable = false
		return
	}
	if r := nameReason(idx, e.Path); r != "" {
		e.Reason = r
	}
	e.Usable = e.Reason == ""
}
func (s *FilesStore) invalidateNames() {
	idxMu.Lock()
	defer idxMu.Unlock()
	if s.idx == nil {
		return
	}
	s.idx.parents = map[string]*parentNames{}
	for _, e := range s.idx.entries {
		applyNames(s.idx, e)
	}
}
func (s *FilesStore) exactKnown(rel string) bool {
	idxMu.Lock()
	defer idxMu.Unlock()
	dir := ""
	for _, n := range strings.Split(rel, "/") {
		p := s.idx.parents[dir]
		if p == nil || !p.complete || !p.names[n] {
			return false
		}
		if dir != "" {
			dir += "/"
		}
		dir += n
	}
	return true
}

// Read parents without idxMu; filesMu serializes the read and publication.
func (s *FilesStore) refreshNames(root *os.Root, rels []string) bool {
	updates := map[string]*parentNames{}
	for _, rel := range rels {
		dir := ""
		for _, n := range strings.Split(rel, "/") {
			p, ok := updates[dir]
			if !ok {
				path := dir
				if path == "" {
					path = "."
				}
				var des []fs.DirEntry
				complete := false
				if _, err := lstatPath(root, path); err == nil {
					if f, err := root.Open(path); err == nil {
						var re error
						des, re = f.ReadDir(MaxScanEntries + 1)
						f.Close()
						complete = (re == nil || re == io.EOF) && len(des) <= MaxScanEntries
					}
				}
				p = makeParentNames(des, complete)
				updates[dir] = p
			}
			if !p.complete || !p.names[n] {
				break
			}
			if dir != "" {
				dir += "/"
			}
			dir += n
		}
	}
	idxMu.Lock()
	defer idxMu.Unlock()
	changed := false
	for dir, p := range updates {
		s.idx.parents[dir] = p
	}
	for _, e := range s.idx.entries {
		old := e.Usable
		applyNames(s.idx, e)
		changed = changed || old != e.Usable
	}
	return changed
}
