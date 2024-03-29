package stream

import (
	"bytes"
	"crypto/rand"
	"io"
	"sync/atomic"
	"testing"
)

var (
	randomBytes      []byte
	randomBytes32KiB []byte
)

func init() {
	var buf bytes.Buffer
	// Too large buffer size causes OOM Kill.
	// Fuzzing uses num of cpu as worker limit.
	// Say you have 24 logical CPU cores,
	// fuzzing will use 24 workers.
	// So it'll allocate bufSize * 24 bytes.
	// num of core may increase over time.
	const bufSize = (33 * 1024) - 19
	_, err := io.CopyN(&buf, rand.Reader, bufSize)
	if err != nil {
		panic(err)
	}
	randomBytes = buf.Bytes()

	var buf2 bytes.Buffer
	const bufSize2 = 32 * 1024
	_, err = io.CopyN(&buf2, rand.Reader, bufSize2)
	if err != nil {
		panic(err)
	}
	randomBytes32KiB = buf2.Bytes()
}

type readReaderAt interface {
	io.Reader
	io.ReaderAt
}

type closable[T readReaderAt] struct {
	R      T
	Closed atomic.Bool
}

func (c *closable[T]) ReadAt(p []byte, off int64) (int, error) {
	return c.R.ReadAt(p, off)
}

func (c *closable[T]) Read(p []byte) (int, error) {
	return c.R.Read(p)
}

func (c *closable[T]) Close() error {
	c.Closed.Store(true)
	return nil
}

func prepareSplittedReader(b []byte, lens []int) []*closable[*bytes.Reader] {
	reader := bytes.NewReader(b)
	var splitted []*closable[*bytes.Reader]
	for i := 0; ; i++ {
		buf := make([]byte, lens[i%len(lens)])
		n, _ := io.ReadAtLeast(reader, buf, 1)
		if n <= 0 {
			break
		}

		splitted = append(splitted, &closable[*bytes.Reader]{R: bytes.NewReader(buf[:n])})
	}
	return splitted
}

// eofReaderAt basically identical to bytes.Reader
// but it returns n, io.EOF if it has read until EOF.
type eofReaderAt struct {
	r *bytes.Reader
}

func (r *eofReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	n, err = r.r.ReadAt(p, off)
	if err == nil && int64(r.r.Len()) == off+int64(n) {
		err = io.EOF
	}
	return n, err
}
func prepareSizedReader(b []byte, lens []int, useEofReaderAt bool) []SizedReaderAt {
	reader := bytes.NewReader(b)
	var sizedReaders []SizedReaderAt
	for i := 0; ; i++ {
		buf := make([]byte, lens[i%len(lens)])
		n, _ := io.ReadAtLeast(reader, buf, 1)
		if n <= 0 {
			break
		}

		var readerAt io.ReaderAt = bytes.NewReader(buf[:n])
		if useEofReaderAt {
			readerAt = &eofReaderAt{bytes.NewReader(buf[:n])}
		}
		sizedReaders = append(sizedReaders, SizedReaderAt{
			R:    readerAt,
			Size: int64(n),
		})
	}
	return sizedReaders
}

type onlyWrite struct {
	w io.Writer
}

func (w onlyWrite) Write(p []byte) (n int, err error) {
	return w.w.Write(p)
}

type onlyRead struct {
	r io.Reader
}

func (r onlyRead) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

func useEofReaderAtTestCaseName(b bool) string {
	if b {
		return "use_eofReaderAt"
	}
	return "use_bytesReader"
}

func TestMultiReadCloser(t *testing.T) {
	readers := prepareSplittedReader(randomBytes, []int{1024})
	r := NewMultiReadCloser(readers...)
	for _, reader := range readers {
		assertBool(t, !reader.Closed.Load(), "Closed is true")
	}
	bin, err := io.ReadAll(r)
	assertErrorsIs(t, err, nil)
	assertBool(t, bytes.Equal(randomBytes, bin), "bytes.Equal returned false")
	err = r.Close()
	assertErrorsIs(t, err, nil)
	for _, reader := range readers {
		assertBool(t, reader.Closed.Load(), "Closed is false")
	}
}

func TestMultiReadAtSeekCloser_Close(t *testing.T) {
	buf := make([]byte, 10*1024+22)
	{
		r := NewMultiReadAtSeekCloser(prepareSizedReader(randomBytes, []int{1024, 8994}, false))
		_, _ = io.ReadFull(r, buf)
		err := r.Close()
		assertErrorsIs(t, err, nil)
	}
	{
		sized := prepareSizedReader(randomBytes, []int{1024, 6789}, false)
		for i, r := range sized {
			r.R = &closable[*bytes.Reader]{R: r.R.(*bytes.Reader)}
			sized[i] = r
		}
		r := NewMultiReadAtSeekCloser(sized)
		_, _ = io.ReadFull(r, buf)
		err := r.Close()
		assertErrorsIs(t, err, nil)
		for i, r := range sized {
			assertBool(t, r.R.(*closable[*bytes.Reader]).Closed.Load(), "Closed returned index %d", i)
		}
	}
}

func TestMultiReadAtSeekCloser_read_all(t *testing.T) {
	for _, b := range []bool{false, true} {
		t.Run(useEofReaderAtTestCaseName(b), func(t *testing.T) {
			r := NewMultiReadAtSeekCloser(prepareSizedReader(randomBytes, []int{1024}, b))
			var out bytes.Buffer
			buf := make([]byte, 1024)
			// prevent efficient methods like ReadFrom from being used.
			// Force it to be on boundary.
			_, err := io.CopyBuffer(onlyWrite{&out}, onlyRead{r}, buf)
			assertNilInterface(t, err)
			assertBool(t,
				len(randomBytes) == out.Len(),
				"src len = %d, dst len = %d",
				len(randomBytes), out.Len(),
			)
			assertBool(t, bytes.Equal(randomBytes, out.Bytes()), "bytes.Equal returned false")
		})
	}
}

func TestMultiReadAtSeekCloser_ReadAt_reads_all(t *testing.T) {
	for _, b := range []bool{false, true} {
		t.Run(useEofReaderAtTestCaseName(b), func(t *testing.T) {
			r := NewMultiReadAtSeekCloser(prepareSizedReader(randomBytes, []int{1024}, b))
			buf := make([]byte, len(randomBytes))
			n, err := r.ReadAt(buf, 0)
			assertBool(
				t,
				err == nil || err == io.EOF,
				"err is not either of nil or io.EOF, but is %#v",
				err,
			)
			assertBool(t,
				len(randomBytes) == n,
				"src len = %d, read = %d",
				len(randomBytes), n,
			)
			assertBool(t, bytes.Equal(randomBytes, buf), "bytes.Equal returned false")
		})
	}
}

func TestMultiReadAtSeekCloser_ReadAt_reads_over_upper_limit(t *testing.T) {
	r := NewMultiReadAtSeekCloser(prepareSizedReader(randomBytes, []int{1024}, false))
	buf := make([]byte, len(randomBytes))
	n, err := r.ReadAt(buf, 100)
	assertErrorsIs(t, err, io.EOF)
	assertBool(t,
		len(randomBytes)-100 == n,
		"src len = %d, read = %d",
		len(randomBytes), n,
	)
	assertBool(t, bytes.Equal(randomBytes[100:], buf[:n]), "bytes.Equal returned false")
}

func TestMultiReadAtSeekCloser_wrong_size(t *testing.T) {
	type testCase struct {
		name      string // case name
		diff      int    // difference between actual read size and alleged in []SizedReaderAt. will be added to index 3.
		readAtLoc int64  // ReadAt offset where ReadAt return an error specified by err.
		err       error
	}
	for _, tc := range []testCase{
		{
			name:      "200bytes_more",
			diff:      200,
			readAtLoc: 1024*4 + 100,
			err:       io.ErrUnexpectedEOF,
		},
		{
			name:      "200bytes_less",
			diff:      -200,
			readAtLoc: 1024*3 + 700,
			err:       ErrInvalidSize,
		},
	} {
		t.Run("Read_"+tc.name, func(t *testing.T) {
			reader := prepareSizedReader(randomBytes, []int{1024}, false)

			sized := reader[3]
			sized.Size = sized.Size + int64(tc.diff)
			reader[3] = sized

			r := NewMultiReadAtSeekCloser(reader)
			var out bytes.Buffer
			buf := make([]byte, 1024)
			_, err := io.CopyBuffer(&out, r, buf)
			assertErrorContains(t, err, "MultiReadAtSeekCloser.Read:")
			assertErrorsIs(t, err, tc.err)
		})
		t.Run("ReatAt_"+tc.name, func(t *testing.T) {
			reader := prepareSizedReader(randomBytes, []int{1024}, false)

			sized := reader[3]
			sized.Size = sized.Size + int64(tc.diff)
			reader[3] = sized

			r := NewMultiReadAtSeekCloser(reader)
			buf := make([]byte, 1024)
			n, err := r.ReadAt(buf, tc.readAtLoc)
			t.Logf("ReadAt: %d", n)
			assertErrorContains(t, err, "MultiReadAtSeekCloser.Read:")
			assertErrorsIs(t, err, tc.err)
		})
	}
}
