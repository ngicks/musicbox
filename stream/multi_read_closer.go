package stream

import (
	"errors"
	"fmt"
	"io"
	"slices"
)

var (
	ErrInvalidSize = errors.New("invalid size")
)

var _ io.ReadCloser = (*multiReadCloser)(nil)

type multiReadCloser struct {
	r       io.Reader
	closers []io.ReadCloser
}

func NewMultiReadCloser(r ...io.ReadCloser) io.ReadCloser {
	var readers []io.Reader
	for _, rr := range r {
		readers = append(readers, rr)
	}

	return &multiReadCloser{
		r:       io.MultiReader(readers...),
		closers: r,
	}
}

func (r *multiReadCloser) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

func (r *multiReadCloser) Close() error {
	var errs []error
	for _, c := range r.closers {
		errs = append(errs, c.Close())
	}
	return NewMultiError(errs)
}

type SizedReaderAt struct {
	R    io.ReaderAt
	Size int64
}

type ReadAtReadSeekCloser interface {
	io.ReaderAt
	io.ReadSeekCloser
}

var _ ReadAtReadSeekCloser = (*multiReadAtSeekCloser)(nil)

type multiReadAtSeekCloser struct {
	idx        int
	cur        int64 // off - cur = offset in current ReaderAt.
	off        int64 // current offset
	upperLimit int64 // precomputed upper limit
	r          []SizedReaderAt
}

func NewMultiReadAtSeekCloser(readers []SizedReaderAt) ReadAtReadSeekCloser {
	var upperLimit int64
	for _, rr := range readers {
		upperLimit += rr.Size
	}
	return &multiReadAtSeekCloser{
		upperLimit: upperLimit,
		r:          slices.Clone(readers),
	}
}

func (r *multiReadAtSeekCloser) Read(p []byte) (int, error) {
	var (
		i  int
		rr SizedReaderAt
	)
	for i, rr = range r.r[r.idx:] {
		if r.off < r.cur+rr.Size {
			break
		}
		r.cur += rr.Size
	}

	if r.off >= r.upperLimit {
		return 0, io.EOF
	}

	readerOff := r.off - r.cur
	n, err := rr.R.ReadAt(p, readerOff)

	if n > 0 || err == io.EOF {
		r.idx += i
		r.off += int64(n)
	}

	if err != nil && err != io.EOF {
		return n, err
	}

	switch rem := rr.Size - readerOff; {
	case int64(n) > rem:
		return n, fmt.Errorf("MultiReadAtSeekCloser.Read: %w", ErrInvalidSize)
	case err == io.EOF && n == 0 && rem > 0:
		return n, fmt.Errorf("MultiReadAtSeekCloser.Read: %w", io.ErrUnexpectedEOF)
	case err == io.EOF && len(r.r)-1 > r.idx:
		err = nil
	}

	return n, err
}

var (
	ErrWhence = errors.New("unknown whence")
	ErrOffset = errors.New("invalid offset")
)

func (r *multiReadAtSeekCloser) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	default:
		return 0, fmt.Errorf("Seek: %w = %d", ErrWhence, whence)
	case io.SeekStart:
	case io.SeekCurrent:
		offset += r.off
	case io.SeekEnd:
		offset += r.upperLimit
	}
	if offset < 0 {
		return 0, fmt.Errorf("Seek: %w: negative", ErrOffset)
	}

	r.off = offset

	if r.off >= r.upperLimit {
		r.cur = r.upperLimit
		r.idx = len(r.r)
		return r.off, nil
	}

	var (
		i   int
		rr  SizedReaderAt
		cur int64
	)
	for i, rr = range r.r {
		if r.off < cur+rr.Size {
			break
		}
		cur += rr.Size
	}
	r.idx = i
	r.cur = cur

	return r.off, nil
}

// ReadAt implements io.ReaderAt.
func (r *multiReadAtSeekCloser) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 || off >= r.upperLimit {
		return 0, io.EOF
	}
	maxExceeded := false
	if max := r.upperLimit - off; int64(len(p)) > max {
		maxExceeded = true
		p = p[0:max]
	}
	for {
		nn, err := r.readAt(p, off)
		n += nn
		off += int64(nn)
		if nn == len(p) || err != nil {
			if maxExceeded && err == nil {
				err = io.EOF
			}
			return n, err
		}
		p = p[nn:]
	}
}

// readAt reads from a single ReaderAt at translated offset.
func (r *multiReadAtSeekCloser) readAt(p []byte, off int64) (n int, err error) {
	if off < 0 || off >= r.upperLimit {
		return 0, io.EOF
	}

	var (
		i   int
		rr  SizedReaderAt
		cur int64
	)
	for i, rr = range r.r {
		if off < cur+rr.Size {
			break
		}
		cur += rr.Size
	}

	if off >= r.upperLimit {
		return 0, io.EOF
	}

	readerOff := off - cur
	n, err = rr.R.ReadAt(p, readerOff)

	if err != nil && err != io.EOF {
		return n, err
	}

	switch rem := rr.Size - readerOff; {
	case int64(n) > rem:
		return n, fmt.Errorf("MultiReadAtSeekCloser.Read: %w", ErrInvalidSize)
	case err == io.EOF && n == 0 && rem > 0:
		return n, fmt.Errorf("MultiReadAtSeekCloser.Read: %w", io.ErrUnexpectedEOF)
	case err == io.EOF && len(r.r)-1 > i:
		err = nil
	}
	return n, err
}

func (r *multiReadAtSeekCloser) Close() error {
	var errs []error
	for _, rr := range r.r {
		if c, ok := rr.R.(io.Closer); ok {
			errs = append(errs, c.Close())
		}
	}
	return NewMultiError(errs)
}
