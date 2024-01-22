package composeservice

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
// Contrary to compose/v2, AddDockerComposeLabel adds labels also to Disabled services.
func AddDockerComposeLabel(project *types.Project) {
	// Mimicking toProject of cli/cli.
	// Without this, docker compose v2 lose track of project and therefore would not be able to recreate services.
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

type ComposeService struct {
	mu          sync.Mutex
	out, err    *bytes.Buffer
	dryRun      bool
	cli         command.Cli
	projectName string
	project     *types.Project
	service     api.Service
}

// NewComposeService returns a new wrapped compose service proxy.
// NewComposeService is not goroutine safe. It mutates given project.
func NewComposeService(
	projectName string,
	project *types.Project,
	dockerCli command.Cli,
) *ComposeService {
	AddDockerComposeLabel(project)

	var bufOut, bufErr = new(bytes.Buffer), new(bytes.Buffer)

	serviceProxy := compose.NewComposeService(dockerCli)

	s := &ComposeService{
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

func (s *ComposeService) overrideOutputStreams() {
	_ = s.cli.Apply(command.WithOutputStream(s.out), command.WithErrorStream(s.err))
}

func (s *ComposeService) resetBuf() {
	s.out.Reset()
	s.err.Reset()
}

func (s *ComposeService) parseOutput() ComposeOutput {
	out := ComposeOutput{}
	out.ParseOutput(s.out.String(), s.err.String(), s.projectName, s.project, s.dryRun)
	return out
}

// Create executes the equivalent to a `compose create`
func (s *ComposeService) Create(ctx context.Context, options api.CreateOptions) (ComposeOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.resetBuf()
	err := s.service.Create(ctx, s.project, options)
	return s.parseOutput(), err
}

// Start executes the equivalent to a `compose start`
func (s *ComposeService) Start(ctx context.Context, options api.StartOptions) (ComposeOutput, error) {
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
func (s *ComposeService) Restart(ctx context.Context, options api.RestartOptions) (ComposeOutput, error) {
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
func (s *ComposeService) Stop(ctx context.Context, options api.StopOptions) (ComposeOutput, error) {
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
func (s *ComposeService) Down(ctx context.Context, options api.DownOptions) (ComposeOutput, error) {
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
func (s *ComposeService) Ps(ctx context.Context, options api.PsOptions) ([]api.ContainerSummary, error) {
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
func (s *ComposeService) Kill(ctx context.Context, options api.KillOptions) (ComposeOutput, error) {
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
func (s *ComposeService) Remove(ctx context.Context, options api.RemoveOptions) (ComposeOutput, error) {
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
// Implementations might not change back to normal mode even if dryRun is false.
// User must call this only once and only when the user whishes to use dry run client.
func (s *ComposeService) DryRunMode(ctx context.Context, dryRun bool) (context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if dryRun {
		cli, err := command.NewDockerCli()
		if err != nil {
			return ctx, err
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
			return ctx, err
		}

		s.dryRun = true
		s.cli = cli
		s.overrideOutputStreams()
		s.service = compose.NewComposeService(s.cli)
	}
	return context.WithValue(ctx, api.DryRunKey{}, dryRun), nil
}
