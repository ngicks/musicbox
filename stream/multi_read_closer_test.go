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
	_, err := io.CopyN(&buf, rand.Reader, (218*1024*1024)-19)
	if err != nil {
		panic(err)
	}
	randomBytes = buf.Bytes()
}

func FuzzMultiReadCloser(f *testing.F) {
	f.Add(1024, 23109, 7697586)
	f.Fuzz(func(t *testing.T, len1, len2, len3 int) {
		t.Logf("seed: %d, %d, %d", len1, len2, len3)

		reader := bytes.NewReader(randomBytes)
		lens := []int{len1, len2, len3}
		var sizedReaders []SizedReaderAt
		for i := 0; ; i++ {
			buf := make([]byte, lens[i%3])
			n, _ := io.ReadAtLeast(reader, buf, 1)
			if n <= 0 {
				break
			}
			sizedReaders = append(sizedReaders, SizedReaderAt{
				R:    bytes.NewReader(buf[:n]),
				Size: int64(n),
			})
		}

		r := NewMultiReadAtCloser(sizedReaders)

		dst, err := io.ReadAll(r)
		if err != nil {
			panic(err)
		}

		assert.Assert(t, len(randomBytes) == len(dst), "src len = %d, dst len = %d", len(randomBytes), len(dst))
		assert.Assert(t, bytes.Equal(randomBytes, dst))
	})

}
