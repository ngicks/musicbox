package fsutil

import (
	"os"
	"testing"

	"github.com/spf13/afero"
	"gotest.tools/v3/assert"
)

func TestCopy(t *testing.T) {
	for _, dir := range []string{
		"testdata/fs1",
		"testdata/fs2",
		"testdata/fs3",
		"testdata/fs4",
		"testdata/fs5",
		"testdata/fs6",
		"testdata/fs7",
		"testdata/fs8",
	} {
		t.Run(dir, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "fsutil-test-Copy-*")
			if err != nil {
				panic(err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			dst := afero.NewBasePathFs(afero.NewOsFs(), tmpDir)
			src := ignoreHiddenFile(os.DirFS(dir))

			err = CopyFS(dst, src)
			assert.NilError(t, err)

			eq, err := Equal(src, afero.NewIOFS(dst))
			assert.NilError(t, err)
			assert.Assert(t, eq)
		})
	}

}
