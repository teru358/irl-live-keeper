package app

import (
	"github.com/teru358/irl-live-keeper/internal/sources"
	"path/filepath"
)

func (a *App) SourcesStore() *sources.Store {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sourcesStore == nil {
		a.sourcesStore = &sources.Store{Path: filepath.Join(a.opts.BaseDir, "sources.yaml")}
	}
	return a.sourcesStore
}

func (b *pageBackend) SourcesStore() *sources.Store { return b.sourceStore }

func (a *App) SourceFilesStore() *sources.FilesStore {
	return &sources.FilesStore{Dir: filepath.Join(a.opts.BaseDir, "source-files")}
}
