package stream

import (
	"fmt"
	"testing"
)

func BenchmarkMultiReadAtSeekCloser_ReadAt_worst(b *testing.B) {
	for _, split := range []int{16, 32, 64, 128, 256} {
		b.Run(fmt.Sprintf("%d_readers", split), func(b *testing.B) {
			r := NewMultiReadAtSeekCloser(prepareReader(randomBytes32KiB, []int{(32 * 1024) / split}, false))

			buf := make([]byte, 128)
			b.ResetTimer()

			for range b.N {
				_, _ = r.ReadAt(buf, int64(len(randomBytes32KiB))-200)
			}
		})
	}
}
