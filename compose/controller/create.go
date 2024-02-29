package controller

import (
	"context"
	"log/slog"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/ngicks/musicbox/compose/service"
)

func (c *Controller) Create(ctx context.Context) (service.Output, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	dryRunService, dryRunCtx, err := c.service.DryRunMode(ctx)
	if err != nil {
		return service.Output{}, err
	}
	dryRunService.UpdateProject(enableAllService)
	output, err := dryRunService.Create(dryRunCtx, api.CreateOptions{
		RemoveOrphans: true,
		Recreate:      api.RecreateDiverged,
	})
	if err != nil {
		return service.Output{}, err
	}
	c.logger.DebugContext(ctx, "dry run result", slog.Any("output", output))

	beingRecreated := make([]string, 0, len(output.Resource))
	for _, state := range output.Resource {
		if state.Resource == service.ResourceContainer && (state.State == service.StateRecreate || state.State == service.StateRecreated) {
			beingRecreated = append(beingRecreated, state.Name)
		}
	}

	err = c.removalHook.OnRemove(beingRecreated)
	if err != nil {
		return service.Output{}, err
	}

	c.service.UpdateProject(enableAllService)
	return c.service.Create(ctx, api.CreateOptions{
		RemoveOrphans: true,
		Recreate:      api.RecreateDiverged,
	})
}
