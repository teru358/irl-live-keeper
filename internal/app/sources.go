package app

import (
	"github.com/teru358/irl-live-keeper/internal/obs"
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

// SourceFilesStore returns the one store of the source-files folder, so that
// its index is kept between requests.
func (a *App) SourceFilesStore() *sources.FilesStore {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.filesStore == nil {
		a.filesStore = &sources.FilesStore{Dir: filepath.Join(a.opts.BaseDir, "source-files")}
	}
	return a.filesStore
}

func statisticsReader(o obsRunner) sources.StatisticsReader {
	if p, ok := o.(interface{ Aux() *obs.Aux }); ok {
		return p.Aux()
	}
	return nil
}
func (b *pageBackend) StatisticsReader() sources.StatisticsReader { return b.statistics }
