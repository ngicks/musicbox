package stream

import (
	"errors"
	"fmt"
	"io"
	"slices"
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

type readAtCloser interface {
	io.ReaderAt
	io.ReadSeekCloser
}

var _ io.ReaderAt = (*multiReadAtCloser)(nil)
var _ io.ReadCloser = (*multiReadAtCloser)(nil)

type multiReadAtCloser struct {
	idx        int
	cur        int64 // off - cur = offset in current ReaderAt.
	off        int64 // current offset
	upperLimit int64 // precomputed upper limit
	r          []SizedReaderAt
}

func NewMultiReadAtCloser(readers []SizedReaderAt) readAtCloser {
	var upperLimit int64
	for _, rr := range readers {
		upperLimit += rr.Size
	}
	return &multiReadAtCloser{
		upperLimit: upperLimit,
		r:          slices.Clone(readers),
	}
}

func (r *multiReadAtCloser) Read(p []byte) (int, error) {
	for i, rr := range r.r[r.idx:] {
		if r.off >= r.cur+rr.Size {
			r.cur += rr.Size
			continue
		}
		n, err := rr.R.ReadAt(p, r.off-r.cur)
		r.idx += i
		r.off += int64(n)
		if err == io.EOF && len(r.r) > r.idx {
			err = nil
		}
		return n, err
	}
	return 0, io.EOF
}

var (
	ErrWhence = errors.New("unknown whence")
	ErrOffset = errors.New("invalid offset")
)

func (r *multiReadAtCloser) Seek(offset int64, whence int) (int64, error) {
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
		return 0, fmt.Errorf("Seek: %w", ErrOffset)
	}
	r.off = offset
	var (
		i   int
		rr  SizedReaderAt
		cur int64
	)
	for i, rr = range r.r {
		if r.off >= cur+rr.Size {
			cur += rr.Size
			continue
		}
		break
	}
	r.idx = i
	r.cur = cur

	return r.off, nil
}

func (r *multiReadAtCloser) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 || off >= r.upperLimit {
		return 0, io.EOF
	}

	var cur int64
	for _, rr := range r.r {
		if off >= cur+rr.Size {
			cur += rr.Size
			continue
		}
		n, err := rr.R.ReadAt(p, off-cur)
		if err == io.EOF && len(r.r) > r.idx {
			err = nil
		}
		return n, err
	}
	return 0, io.EOF
}

func (r *multiReadAtCloser) Close() error {
	var errs []error
	for _, rr := range r.r {
		if c, ok := rr.R.(io.Closer); ok {
			errs = append(errs, c.Close())
		}
	}
	return NewMultiError(errs)
}
