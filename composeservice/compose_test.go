package composeservice

import (
	"context"
	"testing"

	"github.com/docker/compose/v2/pkg/api"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestComposeService_dind(t *testing.T) {
	composeService, err := loaderAdditional.LoadComposeService(context.Background())
	assert.NilError(t, err)

	dryRunCtx, err := composeService.DryRunMode(context.Background(), true)
	assert.NilError(t, err)

	out, err := composeService.Create(dryRunCtx, api.CreateOptions{})
	assert.NilError(t, err)

	delete(out.Resource, "Network:default")
	assert.Assert(t, cmp.DeepEqual(createDryRunOutputResourceMap, out.Resource))
}
