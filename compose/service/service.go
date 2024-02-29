package service

import (
	"bytes"
	"context"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	"github.com/docker/docker/client"
)

// AddDockerComposeLabel changes service.CustomLabels so that is can be found by docker compose v2.
//
// In contrast to compose/v2, AddDockerComposeLabel adds labels also to Disabled services.
func AddDockerComposeLabel(project *types.Project) {
	// Mimicking toProject of cli/cli.
	// Without this, docker compose v2 would lose track of project and therefore would not be able to recreate services.
	customLabel := func(service types.ServiceConfig) map[string]string {
		return map[string]string{
			api.ProjectLabel:     project.Name,
			api.ServiceLabel:     service.Name,
			api.VersionLabel:     api.ComposeVersion,
			api.WorkingDirLabel:  project.WorkingDir,
			api.ConfigFilesLabel: strings.Join(project.ComposeFiles, ","),
			api.OneoffLabel:      "False",
		}
	}

	for i, service := range project.Services {
		service.CustomLabels = customLabel(service)
		project.Services[i] = service
	}

	for i, service := range project.DisabledServices {
		service.CustomLabels = customLabel(service)
		project.DisabledServices[i] = service
	}
}

type Service struct {
	mu          sync.Mutex
	out, err    *bytes.Buffer
	dryRun      bool
	cli         command.Cli
	projectName string
	project     *types.Project
	service     api.Service
}

// NewService returns a new wrapped compose service proxy.
// NewService is not goroutine safe. It mutates given project.
func NewService(
	projectName string,
	project *types.Project,
	dockerCli command.Cli,
) *Service {
	AddDockerComposeLabel(project)

	var bufOut, bufErr = new(bytes.Buffer), new(bytes.Buffer)

	serviceProxy := compose.NewComposeService(dockerCli)

	s := &Service{
		out:         bufOut,
		err:         bufErr,
		cli:         dockerCli,
		dryRun:      false,
		service:     serviceProxy,
		projectName: projectName,
		project:     project,
	}
	s.overrideOutputStreams()
	return s
}

func (s *Service) UpdateProject(mutators ...func(p *types.Project) *types.Project) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cloned, _ := s.project.WithServicesEnabled()
	for _, mut := range mutators {
		cloned = mut(cloned)
	}
	s.project = cloned
}

func (s *Service) Client() client.APIClient {
	return s.cli.Client()
}

func (s *Service) overrideOutputStreams() {
	_ = s.cli.Apply(command.WithOutputStream(s.out), command.WithErrorStream(s.err))
}

func (s *Service) resetBuf() {
	s.out.Reset()
	s.err.Reset()
}

func (s *Service) parseOutput() Output {
	out := Output{}
	out.ParseOutput(s.out.String(), s.err.String(), s.projectName, s.project, s.dryRun)
	return out
}

// Create executes the equivalent to a `compose create`
func (s *Service) Create(ctx context.Context, options api.CreateOptions) (Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.resetBuf()
	err := s.service.Create(ctx, s.project, options)
	return s.parseOutput(), err
}

// Start executes the equivalent to a `compose start`
func (s *Service) Start(ctx context.Context, options api.StartOptions) (Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.resetBuf()
	if options.Project == nil {
		options.Project = s.project
	}
	err := s.service.Start(ctx, s.projectName, options)
	return s.parseOutput(), err
}

// Restart restarts containers
func (s *Service) Restart(ctx context.Context, options api.RestartOptions) (Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.resetBuf()
	if options.Project == nil {
		options.Project = s.project
	}
	err := s.service.Restart(ctx, s.projectName, options)
	return s.parseOutput(), err
}

// Stop executes the equivalent to a `compose stop`
func (s *Service) Stop(ctx context.Context, options api.StopOptions) (Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.resetBuf()
	if options.Project == nil {
		options.Project = s.project
	}
	err := s.service.Stop(ctx, s.projectName, options)
	return s.parseOutput(), err
}

// Down executes the equivalent to a `compose down`
func (s *Service) Down(ctx context.Context, options api.DownOptions) (Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.resetBuf()
	if options.Project == nil {
		options.Project = s.project
	}
	err := s.service.Down(ctx, s.projectName, options)
	return s.parseOutput(), err
}

// Ps executes the equivalent to a `compose ps`
func (s *Service) Ps(ctx context.Context, options api.PsOptions) ([]api.ContainerSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if options.Project == nil {
		options.Project = s.project
	}
	summary, err := s.service.Ps(ctx, s.projectName, options)
	if err != nil {
		return nil, err
	}
	return summary, nil
}

// Kill executes the equivalent to a `compose kill`
func (s *Service) Kill(ctx context.Context, options api.KillOptions) (Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.resetBuf()
	if options.Project == nil {
		options.Project = s.project
	}
	err := s.service.Kill(ctx, s.projectName, options)
	return s.parseOutput(), err
}

// RunOneOffContainer is not exposed here since it calls `signal.Reset` on invocation,
// which removes all signal handlers installed by user code.
// Since it destroys our signal handling planning, we will not be able to rely on it.

// Remove executes the equivalent to a `compose rm`
func (s *Service) Remove(ctx context.Context, options api.RemoveOptions) (Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.resetBuf()
	if options.Project == nil {
		options.Project = s.project
	}
	err := s.service.Remove(ctx, s.projectName, options)
	return s.parseOutput(), err
}

// DryRunMode switches c to dry run mode if dryRun is true.
// Once stepped, implementations might not change back to normal mode even if dryRun is false.
// User must call this only once and only when the user whishes to use dry run client.
func (s *Service) DryRunMode(ctx context.Context) (*Service, context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cloned, _ := s.project.WithServicesEnabled()
	newService := NewService(s.projectName, cloned, s.cli)

	cli, err := command.NewDockerCli()
	if err != nil {
		return nil, nil, err
	}

	options := flags.NewClientOptions()
	options.Context = s.cli.CurrentContext()
	err = cli.Initialize(
		options,
		command.WithInitializeClient(func(cli *command.DockerCli) (client.APIClient, error) {
			return api.NewDryRunClient(s.cli.Client(), s.cli)
		}),
	)
	if err != nil {
		return nil, nil, err
	}

	newService.dryRun = true
	newService.cli = cli
	newService.overrideOutputStreams()
	newService.service = compose.NewComposeService(cli)

	return newService, context.WithValue(ctx, api.DryRunKey{}, true), nil
}
