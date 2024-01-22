package composeservice

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

type ResourceType string

const (
	Container ResourceType = "Container"
	Volume    ResourceType = "Volume"
	Network   ResourceType = "Network"
)

// Copied from https://github.com/docker/compose/blob/19bbb12fac83e19f3ef888722cbb32825b4088e6/pkg/progress/event.go
type StateType string

const (
	Error      StateType = "Error"
	Creating   StateType = "Creating"
	Starting   StateType = "Starting"
	Started    StateType = "Started"
	Waiting    StateType = "Waiting"
	Healthy    StateType = "Healthy"
	Exited     StateType = "Exited"
	Restarting StateType = "Restarting"
	Restarted  StateType = "Restarted"
	Running    StateType = "Running"
	Created    StateType = "Created"
	Stopping   StateType = "Stopping"
	Stopped    StateType = "Stopped"
	Killing    StateType = "Killing"
	Killed     StateType = "Killed"
	Removing   StateType = "Removing"
	Removed    StateType = "Removed"
	Skipped    StateType = "Skipped" // depends_on is set, required is false and dependency service is not running nor present.
	Recreate   StateType = "Recreate"
	Recreated  StateType = "Recreated"
)

var states = []StateType{
	Restarting,
	Restarted,
	Recreated,
	Creating,
	Starting,
	Recreate,
	Removing,
	Stopping,
	Healthy,
	Running,
	Created,
	Stopped,
	Killing,
	Removed,
	Skipped,
	Waiting,
	Started,
	Exited,
	Killed,
	Error,
}

type ComposeOutput struct {
	Resource map[string]ComposeOutputLine
	Out, Err string
}

func (o *ComposeOutput) ParseOutput(stdout, stderr string, projectName string, project *types.Project, isDryRunMode bool) {
	if o.Resource == nil {
		o.Resource = make(map[string]ComposeOutputLine)
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
			o.Resource[string(decoded.ResourceType)+":"+decoded.Name] = decoded
		}
	}
}

type ComposeOutputLine struct {
	Name         string
	Num          int
	ResourceType ResourceType
	StateType    StateType
	Desc         string
	DryRunMode   bool
}

func DecodeComposeOutputLine(line string, projectName string, project *types.Project, isDryRunMode bool) (ComposeOutputLine, error) {
	orgLine := line

	var decoded ComposeOutputLine

	line = strings.TrimLeftFunc(line, unicode.IsSpace)

	var found bool
	line, found = strings.CutPrefix(line, DryRunModePrefix)
	if found || isDryRunMode {
		decoded.DryRunMode = true
	}

	decoded.ResourceType, line = readResourceType(line)
	if decoded.ResourceType == "" {
		return ComposeOutputLine{}, fmt.Errorf("unknown resource type. input = %s", orgLine)
	}
	decoded.Name, decoded.Num, line = readResourceName(line, projectName, project, decoded.ResourceType)
	if decoded.Name == "" {
		return ComposeOutputLine{}, fmt.Errorf("unknown resource name. input = %s", orgLine)
	}
	decoded.StateType, decoded.Desc = readState(line)
	if decoded.StateType == "" {
		return ComposeOutputLine{}, fmt.Errorf("unknown state. input = %s", orgLine)
	}

	return decoded, nil
}

func readResourceType(s string) (resource ResourceType, rest string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)

	switch {
	case strings.HasPrefix(s, string(Container)):
		rest, _ = strings.CutPrefix(s, string(Container))
		return Container, rest
	case strings.HasPrefix(s, string(Volume)):
		rest, _ = strings.CutPrefix(s, string(Volume))
		return Volume, rest
	case strings.HasPrefix(s, string(Network)):
		rest, _ = strings.CutPrefix(s, string(Network))
		return Network, rest
	}
	return "", s
}

func readResourceName(s string, projectName string, project *types.Project, resourceTy ResourceType) (service string, num int, rest string) {
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
	case Container:
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
	case Network:
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
	case Volume:
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

func readState(s string) (state StateType, rest string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	for _, ss := range states {
		if strings.HasPrefix(s, string(ss)) {
			s, _ = strings.CutPrefix(s, string(ss))
			return ss, s
		}
	}
	return "", s
}
