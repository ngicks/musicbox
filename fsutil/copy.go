package fsutil

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

// CopyFS copies from fs.FS to afero.FS.
// The implementation is copied from https://github.com/golang/go/issues/62484
// and is slightly modified.
//
// CopyFS only copies regular files and directories from src.
// File permissions will be copied however their owner remains uid/gid of the process.
func CopyFS(dst afero.Fs, src fs.FS) error {
	buf := getBuf()
	defer putBuf(buf)

	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		target := filepath.FromSlash(path)

		ss, err := d.Info()
		if err != nil {
			return err
		}

		chmod := func() error {
			err = dst.Chmod(target, ss.Mode().Perm())
			if path == "." && os.IsNotExist(err) {
				// Some implementation refuses to create root dir(".").
				// In that case Chmod returns ErrNotExist.
				// Just ignore it.
				err = nil
			}
			if err != nil {
				return fmt.Errorf("failed to chmod created dir, targ = %s, err = %w", target, err)
			}
			return nil
		}

		if d.IsDir() {
			if err := dst.MkdirAll(target, fs.ModePerm); err != nil {
				return err
			}
			if err := chmod(); err != nil {
				return err
			}
			return nil
		}

		if !d.Type().IsRegular() {
			return fmt.Errorf("%w: non regular file is not supported.", ErrBadInput)
		}

		r, err := src.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()

		w, err := dst.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.ModePerm)
		if err != nil {
			return err
		}
		defer func() { _ = w.Close() }()

		err = chmod()
		if err != nil {
			return err
		}

		if _, err := io.CopyBuffer(w, r, *buf); err != nil {
			return fmt.Errorf("copying %s, %w", path, err)
		}

		if err := w.Sync(); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("fsutil.CopyFS: %w", err)
	}
	return nil
}
