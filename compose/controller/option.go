package controller

import "log/slog"

type Option func(c *Controller)

func WithLogger(logger *slog.Logger) Option {
	return func(c *Controller) {
		c.logger = logger
	}
}
