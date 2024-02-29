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

const reverseComposeYaml = `services:
  enabled:
    image: ubuntu:jammy-20230624
    profiles:
      - enabled
    depends_on:
      dependency:
        condition: service_healthy
        restart: true
  dependency:
    image: ubuntu:jammy-20230624
    profiles:
     - disabled
  disabled:
    image: ubuntu:jammy-20230624
    profiles:
      - disabled
    depends_on:
      dependency:
        condition: service_healthy
        restart: true
`

func TestReverse(t *testing.T) {
	type testCase struct {
		name         string
		enabledInSrc []string
		enabledInDst []string
	}
	for _, tc := range []testCase{
		{
			enabledInSrc: []string{"dependency", "enabled"},
			enabledInDst: []string{"disabled"},
		},
		{
			enabledInSrc: []string{"dependency", "disabled", "enabled"},
			enabledInDst: nil,
		},
		{
			enabledInSrc: nil,
			enabledInDst: []string{"dependency", "disabled", "enabled"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			src := loadFromString(reverseComposeYaml)

			// initializing all as enabled without env resolution.
			for k, v := range src.DisabledServices {
				src.Services[k] = v
			}
			src.DisabledServices = nil

			if len(tc.enabledInSrc) > 0 {
				src, err = src.WithSelectedServices(tc.enabledInSrc, types.IncludeDependencies)
				assert.NilError(t, err)
			} else {
				src = src.WithServicesDisabled(src.ServiceNames()...)
			}

			dst, err := Reverse(src)
			assert.NilError(t, err)

			assert.Assert(t, cmp.DeepEqual(tc.enabledInSrc, src.ServiceNames()))
			assert.Assert(t, cmp.DeepEqual(tc.enabledInDst, dst.ServiceNames()))

			assert.Assert(
				t,
				cmp.DeepEqual(
					map[string]struct{}{"dependency": {}, "disabled": {}, "enabled": {}},
					toSet(src.AllServices()),
				),
			)
			assert.Assert(
				t,
				cmp.DeepEqual(
					map[string]struct{}{"dependency": {}, "disabled": {}, "enabled": {}},
					toSet(dst.AllServices()),
				),
			)
		})
	}
}

func loadFromString(composeYmlStr string) *types.Project {
	loaded, err := loader.LoadWithContext(
		context.Background(),
		types.ConfigDetails{
			WorkingDir: "./testdata",
			ConfigFiles: []types.ConfigFile{
				{
					Filename: "./testdata/whatever.yml",
					Content:  []byte(composeYmlStr),
				},
			},
			Environment: types.NewMapping(os.Environ()),
		},
		func(o *loader.Options) {
			o.SetProjectName("example_compose", true)
		},
	)
	if err != nil {
		panic(err)
	}
	return loaded
}

func toSet(services map[string]types.ServiceConfig) map[string]struct{} {
	out := make(map[string]struct{}, len(services))
	for _, s := range services {
		out[s.Name] = struct{}{}
	}
	return out
}
