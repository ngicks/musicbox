package fsutil

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

type CopyFsOption func(o *copyFsOption)

func CopyFsWithIgnoreNonRegularFile(ignoreNonRegular bool) CopyFsOption {
	return func(o *copyFsOption) {
		o.ignoreNonRegularFile = true
	}
}

type copyFsOption struct {
	// ignores non regular files instead of instant error.
	ignoreNonRegularFile bool
}

// CopyFS copies from fs.FS to afero.FS.
// The implementation is copied from https://github.com/golang/go/issues/62484
// and is slightly modified.
//
// The default behavior of CopyFS is:
//   - returns an error if src contains non regular files
//   - copies permission bits
//     -
func CopyFS(dst afero.Fs, src fs.FS, opts ...CopyFsOption) error {
	// in case that the file type of dst does not implement io.ReaderFrom or
	// the file type of src does not implement io.WriterTo.
	// Use 64KiB buffers and reuse them across this package to gain some performance boost.
	buf := getBuf()
	defer putBuf(buf)

	opt := copyFsOption{}
	for _, o := range opts {
		o(&opt)
	}

	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == "." {
			return nil
		}

		target := filepath.FromSlash(path)

		ss, err := d.Info()
		if err != nil {
			return err
		}

		chmod := func() error {
			err = dst.Chmod(target, ss.Mode().Perm())
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
			if opt.ignoreNonRegularFile {
				return nil
			}
			return fmt.Errorf("%w: non regular file is not supported.", ErrBadInput)
		}

		r, err := src.Open(path)
		if err != nil {
			return err
		}

		rClosed := false
		var rCloseErr error
		closeROnce := func() error {
			if !rClosed {
				rClosed = true
				rCloseErr = r.Close()
			}
			return rCloseErr
		}
		defer func() { _ = closeROnce() }()

		w, err := dst.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.ModePerm)
		if err != nil {
			return err
		}
		// TODO: close only once
		defer func() { _ = w.Close() }()

		err = chmod()
		if err != nil {
			return err
		}

		if _, err := io.CopyBuffer(w, r, *buf); err != nil {
			return fmt.Errorf("copying %s, %w", path, err)
		}

		if err := closeROnce(); err != nil {
			return err
		}
		if err := w.Sync(); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("fsutil.CopyFS: %w", err)
	}
	return nil
}
