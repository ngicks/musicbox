package composeservice

import (
	"context"
	_ "embed"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

var (
	createDryRunOutput = []ComposeOutputLine{
		{DryRunMode: true, ResourceType: Network, Name: "sample network", StateType: Creating},
		{DryRunMode: true, ResourceType: Network, Name: "sample network", StateType: Created},
		{DryRunMode: true, ResourceType: Volume, Name: "sample-volume", StateType: Creating},
		{DryRunMode: true, ResourceType: Volume, Name: "sample-volume", StateType: Created},
		{DryRunMode: true, ResourceType: Container, Name: "sample_service", Num: 1, StateType: Creating},
		{DryRunMode: true, ResourceType: Container, Name: "additional", Num: 1, StateType: Creating},
		{DryRunMode: true, ResourceType: Container, Name: "sample_service", Num: 1, StateType: Created},
		{DryRunMode: true, ResourceType: Container, Name: "additional", Num: 1, StateType: Created},
	}
	createOutput = []ComposeOutputLine{
		{ResourceType: Network, Name: "sample network", StateType: Creating},
		{ResourceType: Network, Name: "sample network", StateType: Created},
		{ResourceType: Volume, Name: "sample-volume", StateType: Creating},
		{ResourceType: Volume, Name: "sample-volume", StateType: Created},
		{ResourceType: Container, Name: "sample_service", Num: 1, StateType: Creating},
		{ResourceType: Container, Name: "additional", Num: 1, StateType: Creating},
		{ResourceType: Container, Name: "additional", Num: 1, StateType: Created},
		{ResourceType: Container, Name: "sample_service", Num: 1, StateType: Created},
	}
	startOutput = []ComposeOutputLine{
		{ResourceType: Container, Name: "sample_service", Num: 1, StateType: Starting},
		{ResourceType: Container, Name: "additional", Num: 1, StateType: Starting},
		{ResourceType: Container, Name: "sample_service", Num: 1, StateType: Started},
		{ResourceType: Container, Name: "additional", Num: 1, StateType: Started},
	}
	recreateDryRunOutput = []ComposeOutputLine{
		{DryRunMode: true, ResourceType: Container, Name: "additional", Num: 1, StateType: Stopping},
		{DryRunMode: true, ResourceType: Container, Name: "additional", Num: 1, StateType: Stopped},
		{DryRunMode: true, ResourceType: Container, Name: "additional", Num: 1, StateType: Removing},
		{DryRunMode: true, ResourceType: Container, Name: "additional", Num: 1, StateType: Removed},
		{DryRunMode: true, ResourceType: Container, Name: "sample_service", Num: 1, StateType: Recreate},
		{DryRunMode: true, ResourceType: Container, Name: "sample_service", Num: 1, StateType: Recreated},
	}
	recreateOutput = []ComposeOutputLine{
		{ResourceType: Container, Name: "additional", Num: 1, StateType: Stopping},
		{ResourceType: Container, Name: "additional", Num: 1, StateType: Stopped},
		{ResourceType: Container, Name: "additional", Num: 1, StateType: Removing},
		{ResourceType: Container, Name: "additional", Num: 1, StateType: Removed},
		{ResourceType: Container, Name: "sample_service", Num: 1, StateType: Recreate},
		{ResourceType: Container, Name: "sample_service", Num: 1, StateType: Recreated},
	}
	restartOutput = []ComposeOutputLine{
		{ResourceType: Container, Name: "sample_service", Num: 1, StateType: Starting},
		{ResourceType: Container, Name: "sample_service", Num: 1, StateType: Started},
	}
	downOutput = []ComposeOutputLine{
		{ResourceType: Container, Name: "sample_service", Num: 1, StateType: Stopping},
		{ResourceType: Container, Name: "sample_service", Num: 1, StateType: Stopped},
		{ResourceType: Container, Name: "sample_service", Num: 1, StateType: Removing},
		{ResourceType: Container, Name: "sample_service", Num: 1, StateType: Removed},
		{ResourceType: Volume, Name: "sample-volume", StateType: Removing},
		{ResourceType: Volume, Name: "sample-volume", StateType: Removed},
		{ResourceType: Network, Name: "sample network", StateType: Removing},
		{ResourceType: Network, Name: "sample network", StateType: Removed},
	}

	createDryRunOutputResourceMap = map[string]ComposeOutputLine{
		"Network:sample network":   {DryRunMode: true, ResourceType: Network, Name: "sample network", StateType: Created},
		"Volume:sample-volume":     {DryRunMode: true, ResourceType: Volume, Name: "sample-volume", StateType: Created},
		"Container:sample_service": {DryRunMode: true, ResourceType: Container, Name: "sample_service", Num: 1, StateType: Created},
		"Container:additional":     {DryRunMode: true, ResourceType: Container, Name: "additional", Num: 1, StateType: Created},
	}
)

func TestOutputString(t *testing.T) {
	assert := assert.New(t)

	project, err := loaderAdditional.Load(context.Background())
	if err != nil {
		panic(err)
	}

	type testCase struct {
		lines     string
		expected  []ComposeOutputLine
		shouldErr bool
	}

	for _, tc := range []testCase{
		{
			lines:    createDryRunTxt,
			expected: createDryRunOutput,
		},
		{
			lines:    create,
			expected: createOutput,
		},
		{
			lines:    start,
			expected: startOutput,
		},
		{
			lines:    recreateDryrun,
			expected: recreateDryRunOutput,
		},
		{
			lines:    recreate,
			expected: recreateOutput,
		},
		{
			lines:     restartDryrun,
			shouldErr: true,
		},
		{
			lines:    restart,
			expected: restartOutput,
		},
		{
			lines:    down,
			expected: downOutput,
		},
		{
			lines:     nonexistentComposeYml,
			shouldErr: true,
		},
	} {
		for idx, line := range strings.Split(tc.lines, "\n") {
			if line == "" {
				break
			}
			decoded, err := DecodeComposeOutputLine(line, "testdata", project, false)
			if tc.shouldErr {
				if err == nil {
					t.Errorf("decoding should cause an error but is nil")
				}
				continue
			}
			if err != nil {
				t.Errorf("decode err = %s", err)
				continue
			}
			if diff := cmp.Diff(decoded, tc.expected[idx]); diff != "" {
				t.Errorf("not equal. diff =%s", diff)
			}
		}
	}

	var out ComposeOutput
	out.ParseOutput("", createDryRunTxt, "testdata", project, false)

	assert.Equal("", out.Out)
	assert.Equal(createDryRunTxt, out.Err)

	if diff := cmp.Diff(createDryRunOutputResourceMap, out.Resource); diff != "" {
		t.Errorf("diff = %s", diff)
	}
}

//go:embed  testdata/00_create-dryrun.txt
var createDryRunTxt string

//go:embed  testdata/01_create.txt
var create string

//go:embed  testdata/02_start.txt
var start string

//go:embed  testdata/03_recreate-dryrun.txt
var recreateDryrun string

//go:embed  testdata/04_recreate.txt
var recreate string

//go:embed  testdata/05_restart-dryrun.txt
var restartDryrun string

//go:embed  testdata/06_restart.txt
var restart string

//go:embed  testdata/07_down.txt
var down string

//go:embed  testdata/08_nonexistent_compose_yml.txt
var nonexistentComposeYml string
