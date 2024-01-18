package fsutil

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/spf13/afero"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func ignoreHiddenFile(fsys fs.FS) fs.FS {
	return afero.NewIOFS(afero.NewRegexpFs(afero.FromIOFS{FS: fsys}, regexp.MustCompile(`(?:^|\/)[^.].*`)))
}

func TestSafeWrite(t *testing.T) {
	type testCase struct {
		name               string
		Description        string // optional description for behavior this is just a documentation.
		opts               []SafeWriteOptionOption
		dst                string
		perm               fs.FileMode
		fsys               fs.FS
		content            io.Reader
		pp                 []SafeWritePostProcess
		assertBeforeRename []assertBefore
		assertResult       []assertAfter
	}

	// cases with no error.
	for _, tc := range []testCase{
		{
			name:    "SafeWrite, default option",
			opts:    []SafeWriteOptionOption{},
			dst:     "foo/bar/baz",
			perm:    fs.ModePerm,
			content: bytes.NewBufferString("baz"),
			assertBeforeRename: []assertBefore{
				assertLen(1),
				assertPathPattern(`baz-\d{10}.tmp$`),
			},
			assertResult: []assertAfter{
				assertNilErr(),
				assertContents([]namedContent{{"foo/bar/baz", bytes.NewBufferString("baz")}}),
			},
		},
		{
			name: "SafeWriteFs, default option",
			opts: []SafeWriteOptionOption{},
			dst:  "foo/bar/baz",
			fsys: os.DirFS("testdata/fs6"),
			perm: fs.ModePerm,
			assertBeforeRename: []assertBefore{
				assertLen(1),
				assertPathContains(`^foo/bar/baz-\d{10}.tmp\/`),
			},
			assertResult: []assertAfter{
				assertNilErr(),
				assertFsUnder("foo/bar/baz", ignoreHiddenFile(os.DirFS("testdata/fs6"))),
			},
		},
		{
			name:        "SafeWrite, tmp dir",
			Description: "if tmp dir is non empty, suffix default fallback is disabled.",
			opts:        []SafeWriteOptionOption{WithTmpDir("/tmp")},
			dst:         "foo/bar/baz",
			perm:        fs.ModePerm,
			content:     bytes.NewBufferString("addawd90i2;p"),
			assertBeforeRename: []assertBefore{
				assertLen(1),
				assertPathPattern(`^/?tmp/baz-\d{10}$`),
			},
			assertResult: []assertAfter{
				assertNilErr(),
				assertContents([]namedContent{{"foo/bar/baz", bytes.NewBufferString("addawd90i2;p")}}),
			},
		},
		{
			name:    "SafeWrite, tmp dir with explicit suffix",
			opts:    []SafeWriteOptionOption{WithTmpDir("/tmp"), must(WithPrefixSuffix("pref-", ".tmp"))},
			dst:     "foo/bar/baz",
			perm:    fs.ModePerm,
			content: bytes.NewBufferString("addawd90i2;p"),
			assertBeforeRename: []assertBefore{
				assertLen(1),
				assertPathPattern(`^/?tmp/pref-baz-\d{10}.tmp$`),
			},
			assertResult: []assertAfter{
				assertNilErr(),
				assertContents([]namedContent{{"foo/bar/baz", bytes.NewBufferString("addawd90i2;p")}}),
			},
		},
		{
			name: "SafeWriteFs, tmp dir, explicit suffix, without rand suffix",
			opts: []SafeWriteOptionOption{
				WithTmpDir("tmptmp"),
				must(WithPrefixSuffix("temp-pref-", ".suf")),
				WithRandomPattern(""),
			},
			dst:  "nah/nay",
			fsys: os.DirFS("testdata/fs1"),
			perm: fs.ModePerm,
			assertBeforeRename: []assertBefore{
				assertLen(2), // 2 files
				assertPathContains(`^tmptmp/temp-pref-nay.suf\/`),
			},
			assertResult: []assertAfter{
				assertNilErr(),
				assertFsUnder("nah/nay", os.DirFS("testdata/fs1")),
			},
		},
		{
			name:        "SafeWriteFs, ForcePerm",
			Description: "ForcePerm for SafeWriteFs only invoke chmod on targeted dir, contents under the dir are unaffected",
			opts:        []SafeWriteOptionOption{WithForcePerm(true), WithTmpDir("tmp"), WithRandomPattern("")},
			dst:         "foo",
			fsys:        os.DirFS("testdata/fs4"),
			perm:        0o410,
			assertBeforeRename: []assertBefore{
				assertLen(2), // 2 files
				assertPathContains(`^tmp\/foo`),
				assertPerm("tmp/foo", fs.ModeDir|0410),
			},
			assertResult: []assertAfter{
				assertNilErr(),
				assertFsUnder("foo", os.DirFS("testdata/fs4")),
				assertModeAfter("foo", fs.ModeDir|0410),
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var err error

			tmpDir, err := os.MkdirTemp("", "fsutil-test-SafeWrite-*")
			if err != nil {
				panic(err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			baseFsys := afero.NewBasePathFs(afero.NewOsFs(), tmpDir)

			opt := NewSafeWriteOption(tc.opts...)

			assertCalled := false
			var seenPathsBefore, seenPathsAfter []string
			assertBeforeRename := func(fsys afero.Fs, name string, file afero.File) error {
				assertCalled = true
				err := fs.WalkDir(afero.NewIOFS(fsys), ".", func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if path == "." || d.IsDir() {
						return nil
					}
					seenPathsBefore = append(seenPathsBefore, path)
					return nil
				})
				assert.NilError(t, err)
				for _, assert := range tc.assertBeforeRename {
					assert(t, fsys, seenPathsBefore)
				}
				return nil
			}

			if tc.fsys != nil {
				err = opt.SafeWriteFs(
					baseFsys,
					tc.dst,
					tc.perm,
					ignoreHiddenFile(tc.fsys),
					append(tc.pp, assertBeforeRename)...,
				)
			} else {
				err = opt.SafeWrite(
					baseFsys,
					tc.dst,
					tc.perm,
					tc.content,
					append(tc.pp, assertBeforeRename)...,
				)
			}

			assert.NilError(t, err)
			if tc.assertBeforeRename != nil && !assertCalled {
				t.Fatalf("assertBeforeRename is defined but was not called")
			}

			err = fs.WalkDir(afero.NewIOFS(baseFsys), ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if path == "." || d.IsDir() {
					return nil
				}
				seenPathsAfter = append(seenPathsAfter, path)
				return nil
			})
			assert.NilError(t, err)

			for _, assert := range tc.assertResult {
				assert(t, baseFsys, err)
			}

			for _, path := range seenPathsBefore {
				_ = baseFsys.MkdirAll(filepath.Dir(path), fs.ModePerm)
				f, err := baseFsys.Create(path)
				assert.NilError(t, err)
				assert.NilError(t, f.Close())
			}

			err = opt.CleanTmp(baseFsys)

			assert.NilError(t, err)
			for _, path := range seenPathsBefore {
				_, err := baseFsys.Open(path)
				assert.Assert(t, os.IsNotExist(err))
			}
			for _, path := range seenPathsAfter {
				_, err := baseFsys.Open(path)
				assert.NilError(t, err)
			}
		})
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

type assertBefore func(t *testing.T, fsys afero.Fs, seenPaths []string)

func assertLen(l int) assertBefore {
	return func(t *testing.T, fsys afero.Fs, seenPaths []string) {
		t.Helper()
		assert.Assert(t, cmp.Len(seenPaths, l))
	}
}

func assertPathContains(pat string) assertBefore {
	return func(t *testing.T, fsys afero.Fs, seenPaths []string) {
		t.Helper()
		reg := regexp.MustCompile(pat)
		for _, p := range seenPaths {
			if reg.MatchString(p) {
				return
			}
		}
		t.Fatalf("all path does not matched to pattern %s, paths = %#v", pat, seenPaths)
	}
}

func assertPathPattern(pat string) assertBefore {
	return func(t *testing.T, fsys afero.Fs, seenPaths []string) {
		t.Helper()
		for _, p := range seenPaths {
			assert.Assert(t, cmp.Regexp(pat, p))
		}
	}
}

func assertPerm(path string, mode fs.FileMode) assertBefore {
	return func(t *testing.T, fsys afero.Fs, seenPaths []string) {
		t.Helper()
		s, err := fsys.Stat(path)
		assert.NilError(t, err)
		assert.Assert(t, cmp.Equal(s.Mode(), mode))
	}
}

type assertAfter func(t *testing.T, fsys afero.Fs, err error)

func assertNilErr() assertAfter {
	return func(t *testing.T, fsys afero.Fs, err error) {
		t.Helper()
		assert.NilError(t, err)
	}
}

type namedContent struct {
	path    string
	content *bytes.Buffer
}

func assertContents(namedContents []namedContent) assertAfter {
	return func(t *testing.T, fsys afero.Fs, err error) {
		t.Helper()
		for _, nc := range namedContents {
			f, err := fsys.Open(nc.path)
			assert.NilError(t, err)
			s, err := f.Stat()
			assert.NilError(t, err)
			same, err := sameReader(f, nc.content, s.Size(), int64(nc.content.Len()))
			assert.NilError(t, err)
			assert.Assert(t, same)
		}
	}
}

func assertFsUnder(base string, fsys fs.FS) assertAfter {
	return func(t *testing.T, fsys_ afero.Fs, _ error) {
		t.Helper()
		eq, err := Equal(fsys, afero.NewIOFS(afero.NewBasePathFs(fsys_, base)))
		assert.NilError(t, err)
		assert.Assert(t, eq)
	}
}

func assertModeAfter(path string, mode fs.FileMode) assertAfter {
	return func(t *testing.T, fsys afero.Fs, _ error) {
		t.Helper()
		s, err := fsys.Stat(path)
		assert.NilError(t, err)
		assert.Assert(t, cmp.Equal(s.Mode(), mode))
	}
}
