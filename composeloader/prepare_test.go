package composeloader

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"testing"

	"github.com/ngicks/musicbox/fsutil"
	"github.com/spf13/afero"
	"gotest.tools/v3/assert"
)

var (
	//go:embed testdata/project/compose.yml
	composeYmlBin []byte
)

func TestPrepare(t *testing.T) {
	type testCase struct {
		name        string
		archive     fs.FS
		composeYml  string
		options     []ProjectDirOption[projectDirHandle]
		expected    fs.FS
		checkResult []func(tempDir, composeYml string, handle *projectDirHandle) error
	}

	projectDir := os.DirFS("testdata/project")

	preMadeTempDir, err := os.MkdirTemp("", "composeloader-test-*")
	if err != nil {
		panic(err)
	}

	content := projectContent{RuntimeEnvFiles: os.DirFS("testdata/runtime_env")}
	initialContentOption, err := WithInitialContent[projectDirHandle](content)
	if err != nil {
		panic(err)
	}

	for _, tc := range []testCase{
		{
			name:       "prefixed",
			archive:    projectDir,
			composeYml: "compose.yml",
			options:    []ProjectDirOption[projectDirHandle]{WithPrefix[projectDirHandle]("foo"), initialContentOption},
			expected:   os.DirFS("testdata/expected/prefixed"),
		},
		{
			name:       "non-prefixed",
			archive:    projectDir,
			composeYml: "compose.yml",
			options:    []ProjectDirOption[projectDirHandle]{WithTempDir[projectDirHandle](preMadeTempDir), initialContentOption},
			expected:   os.DirFS("testdata/expected/non-prefixed"),
			checkResult: [](func(tempDir string, composeYml string, handle *projectDirHandle) error){
				func(tempDir, composeYml string, handle *projectDirHandle) error {
					if tempDir != preMadeTempDir {
						return fmt.Errorf("tempDir is not one set with an option, expected = %s, actual = %s", preMadeTempDir, tempDir)
					}
					return nil
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.options == nil {
				tc.options = []ProjectDirOption[projectDirHandle]{}
			}

			dir, err := PrepareProjectDir[projectDirHandle](
				tc.archive,
				tc.composeYml,
				projectDirSet{
					RuntimeEnvFiles: "runtime_env",
				},
				tc.options...,
			)
			assert.NilError(t, err)

			defer func() {
				err := os.RemoveAll(dir.Dir())
				if err != nil {
					t.Logf("tempDir removal failed = %#v", err)
				}
			}()

			bin, err := os.ReadFile(dir.ComposeYmlPath())
			assert.NilError(t, err)
			assert.Assert(t, bytes.Equal(composeYmlBin, bin))

			eq, err := fsutil.Equal(os.DirFS(dir.Dir()), tc.expected)
			assert.NilError(t, err)
			assert.Assert(t, eq)

			if tc.checkResult != nil {
				for _, checker := range tc.checkResult {
					assert.NilError(t, checker(dir.Dir(), dir.ComposeYmlPath(), dir.Handle()))
				}
			}
		})
	}
}

type projectDirSet struct {
	RuntimeEnvFiles string
}

type projectDirHandle struct {
	RuntimeEnvFiles afero.Fs
}

type projectContent struct {
	RuntimeEnvFiles fs.FS
}

func TestPrepare_validPrepareInput(t *testing.T) {
	type testCase struct {
		name string
		s    any
		h    any
		err  error
	}

	for _, tc := range []testCase{
		{
			name: "both nil",
			s:    nil,
			h:    nil,
		},
		{
			name: "single field",
			s: dirSet1{
				Foo: "foo",
			},
			h: &dirHandle1{},
		},
		{
			name: "2 fields",
			s: dirSet2{
				Foo: "./foo",
				Bar: "./bar",
			},
			h: &dirHandle2{},
		},
		{
			name: "specifying absolute path",
			s: dirSet1{
				Foo: "/foo",
			},
			h:   &dirHandle1{},
			err: ErrInvalidInput,
		},
		{
			name: "specifying non local directory",
			s: dirSet1{
				Foo: "../foo",
			},
			h:   &dirHandle1{},
			err: ErrInvalidInput,
		},
		{
			name: "specifying empty path",
			s:    dirSet1{},
			h:    &dirHandle1{},
			err:  ErrInvalidInput,
		},
		{
			name: "dirHandle is not pointer",
			s: dirSet1{
				Foo: "foo",
			},
			h:   dirHandle1{},
			err: ErrInvalidInput,
		},
		{
			name: "invalid dirSet",
			s: invalidDirSet{
				Foo: "foo",
				Bar: 12,
			},
			h:   &dirHandle1{},
			err: ErrInvalidInput,
		},
		{
			name: "invalid dirHandle",
			s: dirSet1{
				Foo: "foo",
			},
			h:   &invalidDirHandle{},
			err: ErrInvalidInput,
		},
		{
			name: "unmatched field num 1",
			s: dirSet1{
				Foo: "foo",
			},
			h:   &dirHandle2{},
			err: ErrInvalidInput,
		},
		{
			name: "unmatched field num 2",
			s: dirSet2{
				Foo: "foo",
				Bar: "bar",
			},
			h:   &dirHandle1{},
			err: ErrInvalidInput,
		},
		{
			name: "unmatched field 1",
			s: dirSet2{
				Foo: "foo",
				Bar: "bar",
			},
			h:   &dirHandle3{},
			err: ErrInvalidInput,
		},
		{
			name: "unmatched field 2",
			s: dirSet3{
				Foo: "foo",
				Baz: "baz",
			},
			h:   &dirHandle2{},
			err: ErrInvalidInput,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validPrepareInput(reflect.ValueOf(tc.s), reflect.ValueOf(tc.h))
			if tc.err == nil {
				assert.NilError(t, err)
			} else {
				assert.Assert(
					t,
					errors.Is(err, tc.err),
					"expected = %#v, actual = %#v",
					tc.err, err,
				)
			}
		})
	}
}

type dirSet1 struct {
	Foo string
}

type dirSet2 struct {
	Foo string
	Bar string
}

type dirSet3 struct {
	Foo string
	Baz string
}

type invalidDirSet struct {
	Foo string
	Bar int
}

type dirHandle1 struct {
	Foo afero.Fs
}

type dirHandle2 struct {
	Foo afero.Fs
	Bar afero.Fs
}

type dirHandle3 struct {
	Foo afero.Fs
	Baz afero.Fs
}

type invalidDirHandle struct {
	Foo afero.Fs
	Bar int
}
