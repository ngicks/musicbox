package composeloader

import (
	"errors"
	"reflect"
	"testing"

	"github.com/spf13/afero"
	"gotest.tools/v3/assert"
)

func TestPrepare(t *testing.T) {
	type testCase struct {
		name         string
		dirSet       any
		mutableFsSet any
		err          error
	}
}

func TestPrepare_checkValidity(t *testing.T) {
	type testCase struct {
		name         string
		dirSet       any
		mutableFsSet any
		err          error
	}

	for _, tc := range []testCase{
		{
			name: "single field",
			dirSet: dirSet1{
				Foo: "foo",
			},
			mutableFsSet: &mutableFsSet1{},
		},
		{
			name: "2 fields",
			dirSet: dirSet2{
				Foo: "./foo",
				Bar: "./bar",
			},
			mutableFsSet: &mutableFsSet2{},
		},
		{
			name: "specifying absolute path",
			dirSet: dirSet1{
				Foo: "/foo",
			},
			mutableFsSet: &mutableFsSet1{},
			err:          ErrInvalidInput,
		},
		{
			name: "specifying parent directory",
			dirSet: dirSet1{
				Foo: "../foo",
			},
			mutableFsSet: &mutableFsSet1{},
			err:          ErrInvalidInput,
		},
		{
			name:         "specifying empty path",
			dirSet:       dirSet1{},
			mutableFsSet: &mutableFsSet1{},
			err:          ErrInvalidInput,
		},
		{
			name: "mutableFsSet is not pointer",
			dirSet: dirSet1{
				Foo: "foo",
			},
			mutableFsSet: mutableFsSet1{},
			err:          ErrInvalidInput,
		},
		{
			name: "invalid dirSet",
			dirSet: invalidDirSet{
				Foo: "foo",
				Bar: 12,
			},
			mutableFsSet: &mutableFsSet1{},
			err:          ErrInvalidInput,
		},
		{
			name: "invalid mutableFsSet",
			dirSet: dirSet1{
				Foo: "foo",
			},
			mutableFsSet: &invalidMutableFsSet{},
			err:          ErrInvalidInput,
		},
		{
			name: "unmatched field num 1",
			dirSet: dirSet1{
				Foo: "foo",
			},
			mutableFsSet: &mutableFsSet2{},
			err:          ErrInvalidInput,
		},
		{
			name: "unmatched field num 2",
			dirSet: dirSet2{
				Foo: "foo",
				Bar: "bar",
			},
			mutableFsSet: &mutableFsSet1{},
			err:          ErrInvalidInput,
		},
		{
			name: "unmatched field 1",
			dirSet: dirSet2{
				Foo: "foo",
				Bar: "bar",
			},
			mutableFsSet: &mutableFsSet3{},
			err:          ErrInvalidInput,
		},
		{
			name: "unmatched field 2",
			dirSet: dirSet3{
				Foo: "foo",
				Baz: "baz",
			},
			mutableFsSet: &mutableFsSet2{},
			err:          ErrInvalidInput,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkValidity(reflect.ValueOf(tc.dirSet), reflect.ValueOf(tc.mutableFsSet))
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

type mutableFsSet1 struct {
	Foo afero.Fs
}

type mutableFsSet2 struct {
	Foo afero.Fs
	Bar afero.Fs
}

type mutableFsSet3 struct {
	Foo afero.Fs
	Baz afero.Fs
}

type invalidMutableFsSet struct {
	Foo afero.Fs
	Bar int
}
