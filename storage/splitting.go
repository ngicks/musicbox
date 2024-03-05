package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"github.com/ngicks/musicbox/fsutil"
	"github.com/spf13/afero"
)

// ReadSplitter returns splitted readers sequentially.
type ReaderSplitter interface {
	// Next returns size limited readers.
	// If ok is true, r is non nil and r reads up to given size.
	// Next and r.Read are not goroutine safe.
	// Calling Next before r is fully consumed causes an undefined behavior.
	Next() (r io.Reader, ok bool)
}

// fusedReader wraps io.Reader and once R returns a non nil error,
// fusedReader is melted and any subsequent Read calls returns that error.
type fusedReader struct {
	R   io.Reader
	Err error
}

func (r *fusedReader) Read(p []byte) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}

	n, err := r.R.Read(p)
	if err != nil {
		r.Err = err
	}
	return n, err
}

func (r *fusedReader) Melted() bool {
	return r.Err != nil
}

type splitter struct {
	r    *fusedReader
	size uint
	buf  []byte
}

const minReadSize = 8 * 1024

// SplitReader returns ReaderSplitter splitting at size.
// It will panic if size is 0.
func SplitReader(r io.Reader, size uint) ReaderSplitter {
	if size == 0 {
		panic("0 size in SplitReader")
	}
	return &splitter{
		r:    &fusedReader{R: r},
		size: size,
		buf:  make([]byte, minReadSize),
	}
}

func (s *splitter) Next() (r io.Reader, ok bool) {
	if s.r.Melted() {
		return nil, false
	}

	buf := s.buf
	if s.size < uint(len(buf)) {
		buf = buf[:s.size]
	}
	readAhead, err := s.r.Read(buf)
	buf = buf[:readAhead]

	if readAhead == 0 && errors.Is(err, io.EOF) {
		// In case reader returns (n > 0 , nil) while it has reached EOF,
		// next Read would returns 0, io.EOF
		return nil, false
	}
	return io.LimitReader(io.MultiReader(bytes.NewReader(buf), s.r), int64(s.size)), true
}

