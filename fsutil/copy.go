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
// with slightly modification to accept afero.FS as dst.
//
// Caveats:
// There's no safety for power failure.
// In case safety is important for the caller,
// dst should be set to a temporarily created directory.
// On completion, the caller should call rename(2), or equivalent syscalls for
// caller's platform, to its final destination, making the operation near atomic.
// Note that this method does not allow dst to have existing files and merging them.
func CopyFS(dst afero.Fs, src fs.FS) error {
	var buf []byte
	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		targ := filepath.FromSlash(path)
		if d.IsDir() {
			if err := dst.MkdirAll(targ, 0777); err != nil {
				return err
			}

			ss, err := d.Info()
			if err != nil {
				return err
			}
			ds, err := dst.Stat(targ)
			if err != nil {
				return err
			}

			if ds.Mode().Perm() == ss.Mode().Perm() {
				return nil
			}

			err = dst.Chmod(targ, ss.Mode().Perm())
			if path == "." && os.IsNotExist(err) {
				// Some implementation refuses to create root dir(".").
				// In that case Chmod returns ErrNotExist.
				// Just ignore it.
				err = nil
			}
			if err != nil {
				return fmt.Errorf("failed to chmod created dir, targ = %s, err = %w", targ, err)
			}
			return nil
		}

		r, err := src.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()

		if d.Type()&fs.ModeType != 0 {
			return fmt.Errorf("non regular file is not supported.")
		}

		info, err := r.Stat()
		if err != nil {
			return err
		}

		w, err := dst.OpenFile(targ, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode()&0777)
		if err != nil {
			return err
		}

		if buf == nil {
			//TODO: check if w implements ReaderFrom or r implements WriterTo. Skip allocation either implements the interface.
			buf = make([]byte, 64*1024)
		}
		if _, err := io.CopyBuffer(w, r, buf); err != nil {
			_ = w.Close()
			return fmt.Errorf("copying %s, %w", path, err)
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
