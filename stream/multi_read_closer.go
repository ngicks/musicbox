package stream

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
)

var (
	ErrInvalidSize = errors.New("invalid size")
)

var _ io.ReadCloser = (*multiReadCloser[io.ReadCloser])(nil)

type multiReadCloser[T io.ReadCloser] struct {
	r       io.Reader
	closers []T
}

func NewMultiReadCloser[T io.ReadCloser](r ...T) io.ReadCloser {
	var readers []io.Reader
	for _, rr := range r {
		readers = append(readers, rr)
	}

	return &multiReadCloser[T]{
		r:       io.MultiReader(readers...),
		closers: r,
	}
}

func (r *multiReadCloser[T]) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

func (r *multiReadCloser[T]) Close() error {
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

type FileLike interface {
	Stat() (fs.FileInfo, error)
	io.ReaderAt
}

// SizedReadersFromFileLike constructs []SizedReaderAt from file like objects.
// For example, *os.File and afero.File implement FileLike.
func SizedReadersFromFileLike[T FileLike](files []T) ([]SizedReaderAt, error) {
	sizedReaders := make([]SizedReaderAt, len(files))
	for i, f := range files {
		s, err := f.Stat()
		if err != nil {
			return nil, err
		}
		sizedReaders[i] = SizedReaderAt{
			R:    f,
			Size: s.Size(),
		}
	}
	return sizedReaders, nil
}

type ReadAtSizer interface {
	io.ReaderAt
	Size() int64
}

// SizedReadersFromReadAtSizer constructs []SizedReaderAt from ReaderAt with Size method.
// For example, *io.SectionReader implements ReadAtSizer.
func SizedReadersFromReadAtSizer[T ReadAtSizer](readers []T) []SizedReaderAt {
	sizedReaders := make([]SizedReaderAt, len(readers))
	for i, r := range readers {
		sizedReaders[i] = SizedReaderAt{
			R:    r,
			Size: r.Size(),
		}
	}
	return sizedReaders
}

type sizedReaderAt struct {
	SizedReaderAt
	accum int64 // starting offset of this reader from head of readers.
}

type ReadAtReadSeekCloser interface {
	io.ReaderAt
	io.ReadSeekCloser
}

var _ ReadAtReadSeekCloser = (*multiReadAtSeekCloser)(nil)

type multiReadAtSeekCloser struct {
	idx        int   // idx of current sizedReaderAt which is pointed by off.
	off        int64 // current offset
	upperLimit int64 // precomputed upper limit
	r          []sizedReaderAt
}

func NewMultiReadAtSeekCloser(readers []SizedReaderAt) ReadAtReadSeekCloser {
	translated := make([]sizedReaderAt, len(readers))
	var accum = int64(0)
	for i, rr := range readers {
		translated[i] = sizedReaderAt{
			SizedReaderAt: rr,
			accum:         accum,
		}
		accum += rr.Size
	}
	return &multiReadAtSeekCloser{
		upperLimit: accum,
		r:          translated,
	}
}

func (r *multiReadAtSeekCloser) Read(p []byte) (int, error) {
	if r.off >= r.upperLimit {
		return 0, io.EOF
	}

	i := search(r.off, r.r[r.idx:])
	rr := r.r[r.idx:][i]

	readerOff := r.off - rr.accum
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
		r.idx = len(r.r)
		return r.off, nil
	}

	r.idx = search(r.off, r.r)

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

	i := search(off, r.r)
	if i < 0 {
		return 0, io.EOF
	}

	rr := r.r[i]
	readerOff := off - rr.accum
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

var searchThreshold int = 32

func search(off int64, readers []sizedReaderAt) int {
	if len(readers) > searchThreshold {
		return binarySearch(off, readers)
	}

	// A simple benchmark has shown that slice look up is faster when readers are not big enough.
	// The threshold exists between 32 and 64.
	for i, rr := range readers {
		if rr.accum <= off && off < rr.accum+rr.Size {
			return i
		}
	}
	return -1
}

func binarySearch(off int64, readers []sizedReaderAt) int {
	i, found := sort.Find(len(readers), func(i int) int {
		switch {
		case off < readers[i].accum:
			return -1
		case readers[i].accum <= off && off < readers[i].accum+readers[i].Size:
			return 0
		default: // r.accum+r.Size <= off:
			return 1
		}
	})
	if !found {
		return -1
	}
	return i
}
