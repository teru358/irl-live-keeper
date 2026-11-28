package web

import (
	"strings"
	"time"

	"github.com/teru358/irl-live-keeper/internal/sources"
)

// assetCheckEvery is how often the assets that source bodies use are looked at
// (a stat of each) while an OBS page or the admin sources screen is connected.
const assetCheckEvery = 2 * time.Second

// assetVersions is the lookup of the ?v= versions the rendered pages carry. It
// reads only the remembered index. It is nil when there is no assets folder.
func assetVersions(b Backend) sources.AssetVersions {
	if store, ok := sourceFiles(b); ok {
		return store.Versions
	}
	return nil
}

// watchingSources reports whether an OBS page or the admin sources screen is
// connected. The caller holds e.mu.
func (e *Events) watchingSources() bool {
	for c, active := range e.clients {
		if !active {
			continue
		}
		for topic := range c.topics {
			if topic == "sources" || strings.HasPrefix(topic, "source:") {
				return true
			}
		}
	}
	return false
}

// refreshAssets looks at the assets the saved bodies use, every assetCheckEvery
// while a page is connected. A changed asset updates the index, and the render
// that follows carries its new ?v= and so a new text to send, even when the
// body does not use now. No lock is held while the files are read.
func (e *Events) refreshAssets(now time.Time, list []sources.Source) {
	store, ok := sourceFiles(e.backend)
	if !ok {
		return
	}
	e.mu.Lock()
	due := e.watchingSources() && (e.assetChecked.IsZero() || now.Sub(e.assetChecked) >= assetCheckEvery)
	if due {
		e.assetChecked = now
	}
	e.mu.Unlock()
	if !due {
		return
	}
	defs := sources.Definitions(e.backend.Snapshot())
	seen := map[string]bool{}
	paths := []string{}
	for _, s := range list {
		if s.Broken != nil {
			continue
		}
		for _, p := range sources.ReferencesOf(s.Body, defs).Assets {
			if !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}
	store.Refresh(paths)
}

// sourceUse is a source that uses an asset.
type sourceUse struct{ Name, Title string }

// assetUsage reads the saved sources once and returns, for each asset path
// ("/source/files/<path>", no query), the sources whose bodies use it in any
// branch. A source that no longer compiles still counts as far as it can be
// read. The error says that the sources could not be read.
func assetUsage(b Backend) (map[string][]sourceUse, error) {
	usage := map[string][]sourceUse{}
	v, ok := b.(interface{ SourcesStore() *sources.Store })
	if !ok || v.SourcesStore() == nil {
		return usage, nil
	}
	snapshot := b.Snapshot()
	var readErr error
	_ = v.SourcesStore().Transaction(snapshot, func(list []sources.Source, err error) ([]sources.Source, bool) {
		if err != nil {
			readErr = err
			return nil, false
		}
		defs := sources.Definitions(snapshot)
		for _, src := range list {
			for _, p := range sources.ReferencesOf(src.Body, defs).Assets {
				usage[p] = append(usage[p], sourceUse{src.Name, src.Title})
			}
		}
		return nil, false
	})
	return usage, readErr
}

func useNames(uses []sourceUse) []string {
	out := make([]string, 0, len(uses))
	for _, u := range uses {
		out = append(out, u.Name)
	}
	return out
}
func useTitles(uses []sourceUse) []string {
	out := make([]string, 0, len(uses))
	for _, u := range uses {
		if u.Title != "" {
			out = append(out, u.Title)
		} else {
			out = append(out, u.Name)
		}
	}
	return out
}
