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

func CopyFsWithIgnoreNonRegularFile() CopyFsOption {
	return func(o *copyFsOption) {
		o.handleNonRegularFile = nonRegularFileHandlingIgnore
	}
}

func CopyFsWithOverridePermission(chmodIf func(path string) (perm fs.FileMode, ok bool)) CopyFsOption {
	return func(o *copyFsOption) {
		o.chmodIf = chmodIf
	}
}

type copyFsOption struct {
	// ignores non regular files instead of instant error.
	handleNonRegularFile nonRegularFileHandling
	chmodIf              func(path string) (perm fs.FileMode, ok bool)
}

type nonRegularFileHandling string

const (
	nonRegularFileHandlingError  nonRegularFileHandling = "" // default is to return an error.
	nonRegularFileHandlingIgnore nonRegularFileHandling = "ignore"
	// nonRegularFileHandlingTrySymlink nonRegularFileHandling = "try_symlink"
)

// CopyFS copies from fs.FS to afero.FS.
// The implementation is copied from https://github.com/golang/go/issues/62484
// and is slightly modified.
//
// The default behavior of CopyFS is:
//   - returns an error if src contains non regular files
//   - copies permission bits
//   - makes directories with fs.ModePerm (o0777).
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
			perm := ss.Mode().Perm()

			if opt.chmodIf != nil {
				customPerm, ok := opt.chmodIf(target)
				if ok {
					perm = customPerm
				}
			}

			err = dst.Chmod(target, perm)
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
			switch opt.handleNonRegularFile {
			case nonRegularFileHandlingError: // default
				return fmt.Errorf("%w: non regular file is not supported.", ErrBadInput)
			case nonRegularFileHandlingIgnore:
				return nil
				// case nonRegularFileHandlingTrySymlink:
			}
		}

		r, err := src.Open(path)
		if err != nil {
			return err
		}
		closeROnce := once(r.Close)
		defer func() { _ = closeROnce() }()

		w, err := dst.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.ModePerm)
		if err != nil {
			return err
		}
		closeWOnce := once(w.Close)
		defer func() { _ = closeWOnce() }()

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
		if err := closeWOnce(); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("fsutil.CopyFS: %w", err)
	}
	return nil
}

func once[T any](fn func() T) func() T {
	called := false
	var result T
	return func() T {
		if called {
			return result
		}
		called = true
		result = fn()
		return result
	}
}
