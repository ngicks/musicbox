package example

import (
	"testing"

	"github.com/ngicks/musicbox/storage"
	"gotest.tools/v3/assert"
)

func TestExample(t *testing.T) {
	var (
		set = DirSet{
			Foo: "./Foo",
			Bar: "./Bar",
			Baz: "./Baz",
		}
		handle  DirHandle
		content DirContents
	)

	assert.NilError(t, storage.ValidatePrepareInput(set, &handle))
	assert.NilError(t, storage.ValidateCopyContentsInput(handle, content, true))
}
