package controller

import (
	"context"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/api/types/container"
)

// RemoveReplacer removes intermediate replacer containers,
// which would otherwise prevent compose from successfully recreate services.
func (c *Controller) RemoveReplacer(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.service.UpdateProject(enableAllService)

	containers, err := c.service.Ps(ctx, api.PsOptions{All: true})
	if err != nil {
		return err
	}

	if len(containers) == 0 {
		return nil
	}

	ids := make([]string, 0, len(containers))

	for i, maybeReplacer := range containers {
		replaceTarget, ok := maybeReplacer.Labels[api.ContainerReplaceLabel]
		if !ok {
			// completely normal for newly created services.
			continue
		}
		for j, maybeReplaceTarget := range containers {
			if i == j {
				continue
			}
			if maybeReplaceTarget.ID == replaceTarget {
				ids = append(ids, maybeReplacer.ID)
				break
			}
		}
	}

	for _, id := range ids {
		err = c.service.Client().ContainerRemove(ctx, id, container.RemoveOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}
