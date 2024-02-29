package controller

import (
	"io"
	"log/slog"
	"slices"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/ngicks/musicbox/compose/service"
)

type Controller struct {
	mu          sync.Mutex
	logger      *slog.Logger
	service     *service.Service
	removalHook RemovalHook
}

func nopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func New(service *service.Service, removalHook RemovalHook, opts ...Option) *Controller {
	c := &Controller{
		logger:      nopLogger(),
		service:     service,
		removalHook: removalHook,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func enableAllService(p *types.Project) *types.Project {
	p, _ = p.WithServicesEnabled(slices.Concat(p.ServiceNames(), p.DisabledServiceNames())...)
	return p
}
