package fsutil

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
)

var fakeErr = errors.New("fake")

// Equal compares l and r and returns true if both have same contents.
// The comparison evaluates mode bits, content of regular files. It ignores mod time.
// Equal returns immediately an error if l has other than directories or regular files.
//
// Performance:
//   - Equal takes stat of every file in l and r.
//   - Also all dirents of directories are read.
//   - When comparing regular files, 2 * 32KiB slices are allocated.
func Equal(l, r fs.FS) (bool, error) {
	equal := true
	err := fs.WalkDir(l, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && d.Type()&fs.ModeType != 0 {
			return fmt.Errorf("fsutil.Equal: only directories and regular files are supported")
		}

		lf, err := l.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = lf.Close() }()

		ls, err := lf.Stat()
		if err != nil {
			return err
		}

		rf, err := r.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = rf.Close() }()

		rs, err := rf.Stat()
		if err != nil {
			return err
		}

		if ls.Mode() != rs.Mode() {
			return fakeErr
		}

		if d.IsDir() {
			ldirents, err := fs.ReadDir(l, path)
			if err != nil {
				return err
			}

			rdirents, err := fs.ReadDir(r, path)
			if err != nil {
				return err
			}

			if len(ldirents) != len(rdirents) {
				return fmt.Errorf("fsutil.Equal: content mismatch, dir = %s", path)
			}
		} else {
			equal, err = sameFile(lf, rf)
			if err != nil {
				return err
			}
			if !equal {
				return fakeErr
			}
		}

		return nil
	})

	if err != nil {
		equal = false
	}
	if errors.Is(err, fakeErr) {
		err = nil
	}

	return equal, err
}

func sameFile(r, l fs.File) (bool, error) {
	rs, err := r.Stat()
	if err != nil {
		return false, err
	}
	ls, err := l.Stat()
	if err != nil {
		return false, err
	}

	rsize, lsize := rs.Size(), ls.Size()

	if rsize != lsize {
		return false, nil
	}

	size := int(rsize)

	bufr, bufl := make([]byte, 32*1024), make([]byte, 32*1024)
	for size > 0 {
		if len(bufr) > size {
			bufr = bufr[:size]
			bufl = bufl[:size]
		}
		_, err := io.ReadFull(r, bufr)
		if err != nil {
			return false, err
		}
		n, err := io.ReadFull(l, bufl)
		if err != nil {
			return false, err
		}

		if !bytes.Equal(bufr, bufl) {
			return false, nil
		}
		size -= n
	}

	return true, nil
}
