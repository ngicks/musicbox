package stream

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"gotest.tools/v3/assert"
)

var randomBytes []byte

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
}

func prepareReader(b []byte, lens []int) []SizedReaderAt {
	reader := bytes.NewReader(b)
	var sizedReaders []SizedReaderAt
	for i := 0; ; i++ {
		buf := make([]byte, lens[i%len(lens)])
		n, _ := io.ReadAtLeast(reader, buf, 1)
		if n <= 0 {
			break
		}
		sizedReaders = append(sizedReaders, SizedReaderAt{
			R:    bytes.NewReader(buf[:n]),
			Size: int64(n),
		})
	}
	return sizedReaders
}

type onlyWrite struct {
	w io.Writer
}

func (h onlyWrite) Write(p []byte) (n int, err error) {
	return h.w.Write(p)
}

func TestMultiReadAtSeekCloser(t *testing.T) {
	r := NewMultiReadAtSeekCloser(prepareReader(randomBytes, []int{1024}))
	var out bytes.Buffer
	buf := make([]byte, 1024)
	// prevent efficient methods like ReadFrom from being used.
	// Force it to be on boundary.
	_, err := io.CopyBuffer(onlyWrite{&out}, r, buf)
	assert.NilError(t, err)
	assert.Assert(t, len(randomBytes) == out.Len(), "src len = %d, dst len = %d", len(randomBytes), out.Len())
	assert.Assert(t, bytes.Equal(randomBytes, out.Bytes()))
}
