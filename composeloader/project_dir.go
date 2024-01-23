package composeloader

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ngicks/musicbox/fsutil"
	"github.com/ngicks/musicbox/storage"
	"github.com/spf13/afero"
)

// ProjectDir is handle for a directory that contains compose.yml and all relevant files.
type ProjectDir[S, H any] struct {
	baseDir     string
	composePath string
	pathSet     S
	pathHandle  H
}

type ComposeArchive struct {
	Archive     fs.FS
	ComposePath string
}

func prepareProjectDirErr(format string, args ...any) error {
	return fmt.Errorf("PrepareProjectDir: "+format, args...)
}

func wrapErr(err error) error {
	return prepareProjectDirErr("%w", err)
}

func NewSimpleProjectDir(
	dir string,
	archive ComposeArchive,
) (*ProjectDir[any, any], error) {
	return PrepareProjectDir[any, any](dir, "", archive, nil, nil)
}

func PrepareProjectDir[S, H any](
	dir string,
	archivePath string,
	archive ComposeArchive,
	pathSet S,
	initialContent any,
) (*ProjectDir[S, H], error) {
	if dir == "" {
		tempDir, err := os.MkdirTemp("", "composeloader-project-*")
		if err != nil {
			return nil, wrapErr(err)
		}
		dir = tempDir
	}

	base := afero.NewBasePathFs(afero.NewOsFs(), dir)

	var err error
	err = base.MkdirAll(archivePath, fs.ModePerm)
	if err != nil {
		return nil, wrapErr(err)
	}

	archiveFsys := afero.NewBasePathFs(base, archivePath)

	err = fsutil.CopyFS(archiveFsys, archive.Archive)
	if err != nil {
		return nil, wrapErr(err)
	}

	// TODO: check existence of compose.yml?

	handle, err := storage.PrepareHandle[S, H](base, pathSet, initialContent)
	if err != nil {
		return nil, wrapErr(err)
	}

	return &ProjectDir[S, H]{
		baseDir:     dir,
		composePath: filepath.Join(archivePath, archive.ComposePath),
		pathSet:     pathSet,
		pathHandle:  handle,
	}, nil
}

func (d *ProjectDir[S, H]) ComposeYmlPath() string {
	return filepath.Join(d.baseDir, d.composePath)
}

func (d *ProjectDir[S, H]) PathSet() S {
	return d.pathSet
}

func (d *ProjectDir[S, H]) PathHandle() H {
	return d.pathHandle
}

func (d *ProjectDir[S, H]) Dir() string {
	return d.baseDir
}
