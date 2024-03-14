package stream

import (
	"bytes"
	"encoding/hex"
	"io"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func FuzzMultiReadAtSeekCloser_Read(f *testing.F) {
	f.Add(6602, 23109, 7697586)
	f.Fuzz(func(t *testing.T, len1, len2, len3 int) {
		if min(len1, len2, len3) <= 0 { // too small len might cause this test longer.
			t.Skip()
		}
		t.Logf("seed: %d, %d, %d", len1, len2, len3)
		r := NewMultiReadAtSeekCloser(prepareReader(randomBytes, []int{len1, len2, len3}))
		dst, err := io.ReadAll(r)
		assert.NilError(t, err)
		assert.Assert(t, len(randomBytes) == len(dst), "src len = %d, dst len = %d", len(randomBytes), len(dst))
		assert.Assert(t, bytes.Equal(randomBytes, dst))
	})
}

func FuzzMultiReadAtSeekCloser_ReadAt(f *testing.F) {
	f.Add(1025, 7777, 30787, 0, 255, 6666)
	f.Fuzz(func(t *testing.T, len1, len2, len3, loc1, loc2, loc3 int) {
		if min(len1, len2, len3) <= 0 ||
			min(loc1, loc2, loc3) < 0 { // ReadAt / Seek loc cannot be less than 0
			t.Skip()
		}
		t.Logf("len: %d, %d, %d", len1, len2, len3)
		t.Logf("loc: %d, %d, %d", loc1, loc2, loc3)
		lens := []int{len1, len2, len3}
		locs := [...]int{loc1, loc2, loc3}

		r := NewMultiReadAtSeekCloser(prepareReader(randomBytes, lens))
		for _, loc := range locs {
			{
				bin, err := io.ReadAll(io.NewSectionReader(r, int64(loc), 1024))
				assert.NilError(t, err)
				split := randomBytes[:]
				if loc >= len(randomBytes) {
					split = split[len(randomBytes):]
				} else {
					split = split[loc:]
				}
				if len(split) >= 1024 {
					split = split[:1024]
				}
				assert.Assert(t, len(bin) == len(split), "left = %d, right = %d", len(bin), len(split))
				assert.Assert(t, bytes.Equal(bin, split), "left = %s, right = %s", hex.EncodeToString(bin), hex.EncodeToString(split))
			}
		}
	})
}

func FuzzMultiReadAtSeekCloser_Seek(f *testing.F) {
	f.Add(1025, 7777, 30787, 0, 255, 6666)
	f.Fuzz(func(t *testing.T, len1, len2, len3, loc1, loc2, loc3 int) {
		if min(len1, len2, len3) <= 0 || // len must be greater than 0
			min(loc1, loc2, loc3) < 0 { // ReadAt / Seek loc cannot be less than 0
			t.Skip()
		}
		t.Logf("len: %d, %d, %d", len1, len2, len3)
		t.Logf("loc: %d, %d, %d", loc1, loc2, loc3)
		lens := []int{len1, len2, len3}
		locs := [...]int{loc1, loc2, loc3}

		for _, loc := range locs {
			for i := 0; i < 6; i++ {
				off := int64(loc)
				if i >= 3 {
					off *= -1
				}

				t.Logf("off = %d", off)
				var whence int
				switch i % 3 {
				case 0:
					whence = io.SeekStart
					t.Logf("whence = SeekStart")
				case 1:
					whence = io.SeekCurrent
					t.Logf("whence = SeekCurrent")
				case 2:
					whence = io.SeekEnd
					t.Logf("whence = SeekEnd")
				}

				mult := NewMultiReadAtSeekCloser(prepareReader(randomBytes, lens))
				org := bytes.NewReader(randomBytes)

				buf1, buf2 := make([]byte, len1), make([]byte, len1)

				_, rErr := io.ReadFull(mult, buf1)
				_, orgErr := io.ReadFull(org, buf1)
				assert.Assert(t, rErr == nil && orgErr == nil || rErr != nil && orgErr != nil, "left = %v, right = %v", rErr, orgErr)

				n, err := mult.Seek(off, whence)
				bufN, bufErr := org.Seek(off, whence)
				if err != nil {
					assert.Assert(t, bufErr != nil)
					continue
				}
				assert.Assert(t, cmp.Equal(n, bufN))

				_, rErr = io.ReadFull(mult, buf1)
				_, orgErr = io.ReadFull(org, buf2)
				assert.Assert(t, rErr == nil && orgErr == nil || rErr != nil && orgErr != nil, "left = %v, right = %v", rErr, orgErr)
				if rErr == io.EOF {
					assert.Assert(t, cmp.Equal(rErr, orgErr))
				}
				if rErr == nil {
					assert.Assert(t, bytes.Equal(buf1, buf2))
				}
			}
		}
	})
}
