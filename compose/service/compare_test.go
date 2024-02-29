package service

import (
	"context"
	"os"
	"testing"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

var (
	loaderBase, loaderAdditional, loaderAdditional2 *LoaderProxy
)

func init() {
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	var err error

	loaderAdditional2, err = NewLoaderProxy(
		"testdata",
		types.ConfigDetails{
			WorkingDir: "./testdata",
			ConfigFiles: []types.ConfigFile{
				{Filename: "./testdata/compose.yml"},
				{Filename: "./testdata/additional.yml"},
				{Filename: "./testdata/additional2.yml"},
			},
			Environment: types.NewMapping(os.Environ()),
		},
		[]func(*loader.Options){func(o *loader.Options) { o.Profiles = []string{"*"} }},
		nil,
	)
	must(err)
	must(loaderAdditional2.PreloadConfigDetails())

	configDetails := loaderAdditional2.ConfigDetails()

	loaderBase, err = NewLoaderProxy(
		loaderAdditional2.ProjectName(),
		types.ConfigDetails{
			Version:     configDetails.Version,
			WorkingDir:  configDetails.WorkingDir,
			ConfigFiles: []types.ConfigFile{configDetails.ConfigFiles[0]},
			Environment: configDetails.Environment,
		},
		loaderAdditional2.Options(),
		nil,
	)
	must(err)

	loaderAdditional, err = NewLoaderProxy(
		loaderAdditional2.ProjectName(),
		types.ConfigDetails{
			Version:     configDetails.Version,
			WorkingDir:  configDetails.WorkingDir,
			ConfigFiles: []types.ConfigFile{configDetails.ConfigFiles[0], configDetails.ConfigFiles[1]},
			Environment: configDetails.Environment,
		},
		loaderAdditional2.Options(),
		nil,
	)
	must(err)
}

func TestCompareProjectImage(t *testing.T) {
	ctx := context.Background()
	old, _ := loaderAdditional.Load(ctx)
	newer, _ := loaderAdditional2.Load(ctx)

	onlyInOld, addedInNew := CompareProjectImage(old, newer)
	assert.Assert(t, cmp.DeepEqual([]string{"debian:bookworm-20230904"}, onlyInOld))
	assert.Assert(t, cmp.DeepEqual([]string(nil), addedInNew))
}
