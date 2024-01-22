package composeservice

import (
	"context"
	"os"
	"testing"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestPreloadConfigDetails(t *testing.T) {
	projectName := "testdata"
	loadedNormally, err := loader.LoadWithContext(
		context.Background(),
		types.ConfigDetails{
			WorkingDir: "./testdata",
			ConfigFiles: []types.ConfigFile{
				{Filename: "./testdata/compose.yml"},
				{Filename: "./testdata/additional.yml"},
			},
			Environment: types.NewMapping(os.Environ()),
		},
		func(o *loader.Options) {
			o.SetProjectName(projectName, true)
		},
	)
	if err != nil {
		assert.NilError(t, err)
	}

	confDetail, err := PreloadConfigDetails(types.ConfigDetails{
		ConfigFiles: []types.ConfigFile{
			{Filename: "./testdata/compose.yml"},
			{Filename: "./testdata/additional.yml"},
		},
		Environment: types.NewMapping(os.Environ()),
	})
	if err != nil {
		assert.NilError(t, err)
	}

	cachedConfig, err := loader.LoadWithContext(
		context.Background(),
		confDetail,
		func(o *loader.Options) {
			o.SetProjectName(projectName, true)
		},
	)
	if err != nil {
		assert.NilError(t, err)
	}

	assert.Assert(t, cmp.DeepEqual(loadedNormally, cachedConfig))
}
