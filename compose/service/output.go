package service

import (
	"bufio"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/compose-spec/compose-go/v2/types"
)

const (
	DryRunModePrefix = "DRY-RUN MODE - "
)

type Resource string

const (
	ResourceContainer Resource = "Container"
	ResourceVolume    Resource = "Volume"
	ResourceNetwork   Resource = "Network"
)

// Copied from https://github.com/docker/compose/blob/19bbb12fac83e19f3ef888722cbb32825b4088e6/pkg/progress/event.go
type State string

const (
	StateError      State = "Error"
	StateCreating   State = "Creating"
	StateStarting   State = "Starting"
	StateStarted    State = "Started"
	StateWaiting    State = "Waiting"
	StateHealthy    State = "Healthy"
	StateExited     State = "Exited"
	StateRestarting State = "Restarting"
	StateRestarted  State = "Restarted"
	StateRunning    State = "Running"
	StateCreated    State = "Created"
	StateStopping   State = "Stopping"
	StateStopped    State = "Stopped"
	StateKilling    State = "Killing"
	StateKilled     State = "Killed"
	StateRemoving   State = "Removing"
	StateRemoved    State = "Removed"
	StateSkipped    State = "Skipped" // depends_on is set, required is false and dependency service is not running nor present.
	StateRecreate   State = "Recreate"
	StateRecreated  State = "Recreated"
)

var states = []State{
	StateRestarting,
	StateRestarted,
	StateRecreated,
	StateCreating,
	StateStarting,
	StateRecreate,
	StateRemoving,
	StateStopping,
	StateHealthy,
	StateRunning,
	StateCreated,
	StateStopped,
	StateKilling,
	StateRemoved,
	StateSkipped,
	StateWaiting,
	StateStarted,
	StateExited,
	StateKilled,
	StateError,
}

type NamedResource struct {
	Resource Resource
	Name     string
}

func (nr NamedResource) String() string {
	return string(nr.Resource) + ":" + nr.Name
}

type Output struct {
	Resource map[NamedResource]OutputLine
	Out, Err string
}

func (o *Output) ParseOutput(stdout, stderr string, projectName string, project *types.Project, isDryRunMode bool) {
	if o.Resource == nil {
		o.Resource = make(map[NamedResource]OutputLine)
	}
	o.Out = stdout
	o.Err = stderr

	for _, lines := range []string{stdout, stderr} {
		scanner := bufio.NewScanner(strings.NewReader(lines))
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			decoded, err := DecodeComposeOutputLine(line, projectName, project, isDryRunMode)
			if err != nil {
				continue
			}
			o.Resource[NamedResource{decoded.Resource, decoded.Name}] = decoded
		}
	}
}

type OutputLine struct {
	Name       string
	Num        int
	Resource   Resource
	State      State
	Desc       string
	DryRunMode bool
}

func DecodeComposeOutputLine(line string, projectName string, project *types.Project, isDryRunMode bool) (OutputLine, error) {
	orgLine := line

	var decoded OutputLine

	line = strings.TrimLeftFunc(line, unicode.IsSpace)

	var found bool
	line, found = strings.CutPrefix(line, DryRunModePrefix)
	if found || isDryRunMode {
		decoded.DryRunMode = true
	}

	decoded.Resource, line = readResourceType(line)
	if decoded.Resource == "" {
		return OutputLine{}, fmt.Errorf("unknown resource type. input = %s", orgLine)
	}
	decoded.Name, decoded.Num, line = readResourceName(line, projectName, project, decoded.Resource)
	if decoded.Name == "" {
		return OutputLine{}, fmt.Errorf("unknown resource name. input = %s", orgLine)
	}
	decoded.State, decoded.Desc = readState(line)
	if decoded.State == "" {
		return OutputLine{}, fmt.Errorf("unknown state. input = %s", orgLine)
	}

	return decoded, nil
}

func readResourceType(s string) (resource Resource, rest string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)

	switch {
	case strings.HasPrefix(s, string(ResourceContainer)):
		rest, _ = strings.CutPrefix(s, string(ResourceContainer))
		return ResourceContainer, rest
	case strings.HasPrefix(s, string(ResourceVolume)):
		rest, _ = strings.CutPrefix(s, string(ResourceVolume))
		return ResourceVolume, rest
	case strings.HasPrefix(s, string(ResourceNetwork)):
		rest, _ = strings.CutPrefix(s, string(ResourceNetwork))
		return ResourceNetwork, rest
	}
	return "", s
}

func readResourceName(s string, projectName string, project *types.Project, resourceTy Resource) (service string, num int, rest string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	if s[0] == '"' {
		s = s[1:]
	}
	// I don't know why. But volume name is printed vis fmt.*printf variants and it uses the %q formatter.
	// And since volume is not allowed to have space or any special characters, you can safely igore quotation.
	var found bool
	s, found = strings.CutPrefix(s, projectName)
	if found {
		// projectName + ( "_" | "-" ) + serviceName
		s = s[1:]
	}

	switch resourceTy {
	case ResourceContainer:
		for _, serviceCfg := range project.Services {
			if strings.HasPrefix(s, serviceCfg.Name) {
				rest, _ := strings.CutPrefix(s, serviceCfg.Name)
				if rest[0] != '-' {
					continue
				}
				rest = rest[1:]
				var i int
				for i = 0; i < len(rest); i++ {
					if rest[i] == ' ' {
						break
					}
				}
				numStr := rest[0:i]
				num, _ := strconv.ParseInt(numStr, 10, 64)
				rest = rest[i:]
				return serviceCfg.Name, int(num), rest
			}
		}
	case ResourceNetwork:
		networkCfg := project.NetworkNames()
		sort.Strings(networkCfg)
		for i := len(networkCfg) - 1; i >= 0; i-- {
			if strings.HasPrefix(s, networkCfg[i]) && (s[len(networkCfg[i])] == '"' || s[len(networkCfg[i])] == ' ') {
				s, _ = strings.CutPrefix(s, networkCfg[i])
				if s[0] == '"' {
					s = s[1:]
				}
				return networkCfg[i], 0, s
			}

		}
	case ResourceVolume:
		for volumeName := range project.Volumes {
			if strings.HasPrefix(s, volumeName) && (s[len(volumeName)] == '"' || s[len(volumeName)] == ' ') {
				s, _ = strings.CutPrefix(s, volumeName)
				if s[0] == '"' {
					s = s[1:]
				}
				return volumeName, 0, s
			}
		}
	}
	return "", 0, s
}

func readState(s string) (state State, rest string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	for _, ss := range states {
		if strings.HasPrefix(s, string(ss)) {
			s, _ = strings.CutPrefix(s, string(ss))
			return ss, s
		}
	}
	return "", s
}
