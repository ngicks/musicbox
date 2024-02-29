package fsutil

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

var fakeErr = errors.New("fake")

// Equal compares l and r and returns true if both have same contents.
//
// The comparison evaluates
//   - mode bits of dirents
//   - content of directory
//   - content of regular files
//
// Equal returns immediately an error if l has other than directories or regular files.
//
// Note that mode bits of the root directory is ignored since often it is not controlled.
//
// Performance:
//   - Equal takes stat of every file in l and r.
//   - Also all dirents of directories are read.
//   - Files are entirely read
func Equal(l, r fs.FS, opts ...CopyFsOption) (bool, error) {
	opt := newCopyFsOption(opts...)

	err := fs.WalkDir(l, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && d.Type().Type() != 0 {
			switch opt.handleNonRegularFile {
			case nonRegularFileHandlingError: // default
				return fmt.Errorf("%w: only directories and regular files are supported", ErrBadInput)
			case nonRegularFileHandlingIgnore:
				return nil
				// case nonRegularFileHandlingTrySymlink:
			}
		}

		var lf, rf fs.File
		// no mode bits comparison for root dir.
		if path != "." {
			// comparing mode bits.
			lf, err = l.Open(path)
			if err != nil {
				return err
			}
			defer func() { _ = lf.Close() }()

			ls, err := lf.Stat()
			if err != nil {
				return err
			}

			rf, err = r.Open(path)
			if err != nil {
				if os.IsNotExist(err) {
					return fakeErr
				}
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
		}

		if d.IsDir() {
			lDirents, err := fs.ReadDir(l, path)
			if err != nil {
				return err
			}

			rDirents, err := fs.ReadDir(r, path)
			if err != nil {
				return err
			}

			if len(lDirents) != len(rDirents) {
				return fakeErr
			}
		} else {
			equal, err := sameFile(lf, rf)
			if err != nil {
				return err
			}
			if !equal {
				return fakeErr
			}
		}

		return nil
	})

	var equal = true
	if err != nil {
		equal = false
	}
	if errors.Is(err, fakeErr) {
		err = nil
	}
	if err != nil {
		err = fmt.Errorf("fsutil.Equal: %w", err)
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

	rSize := rs.Size()
	lSize := ls.Size()

	return sameReader(l, r, lSize, rSize)
}

func sameReader(l, r io.Reader, lSize, rSize int64) (same bool, err error) {
	if rSize != lSize {
		return false, nil
	}

	if rSize == 0 {
		return true, nil
	}

	size := int(rSize)

	bufRefL, bufRefR := getBuf(), getBuf()
	defer func() {
		putBuf(bufRefL)
		putBuf(bufRefR)
	}()

	bufL, bufR := *bufRefL, *bufRefR
	for size > 0 {
		if len(bufR) > size {
			bufR = bufR[:size]
			bufL = bufL[:size]
		}
		_, err := io.ReadFull(r, bufR)
		if err != nil {
			return false, err
		}
		n, err := io.ReadFull(l, bufL)
		if err != nil {
			return false, err
		}

		if !bytes.Equal(bufR, bufL) {
			return false, nil
		}
		size -= n
	}

	return true, nil
}
