package composeloader

import (
	"reflect"
	"testing"

	"github.com/ngicks/musicbox/composeloader/example"
	"gotest.tools/v3/assert"
)

func TestExample(t *testing.T) {
	var (
		set = example.DirSet{
			Foo: "./Foo",
			Bar: "./Bar",
			Baz: "./Baz",
		}
		handle  example.DirHandle
		content example.DirContents
	)

	assert.NilError(t, validPrepareInput(reflect.ValueOf(set), reflect.ValueOf(&handle)))
	assert.NilError(t, validCopyContentsInput(reflect.ValueOf(handle), reflect.ValueOf(content), true))
}
