package service

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

	dryRunService, dryRunCtx, err := composeService.DryRunMode(context.Background())
	assert.NilError(t, err)

	out, err := dryRunService.Create(dryRunCtx, api.CreateOptions{})
	assert.NilError(t, err)

	delete(out.Resource, NamedResource{"Network", "default"})
	assert.Assert(t, cmp.DeepEqual(createDryRunOutputResourceMap, out.Resource))
}
