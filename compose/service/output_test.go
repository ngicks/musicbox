package service

import (
	"context"
	_ "embed"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gotest.tools/v3/assert"
)

var (
	createDryRunOutput = []OutputLine{
		{DryRunMode: true, Resource: ResourceNetwork, Name: "sample network", State: StateCreating},
		{DryRunMode: true, Resource: ResourceNetwork, Name: "sample network", State: StateCreated},
		{DryRunMode: true, Resource: ResourceVolume, Name: "sample-volume", State: StateCreating},
		{DryRunMode: true, Resource: ResourceVolume, Name: "sample-volume", State: StateCreated},
		{DryRunMode: true, Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateCreating},
		{DryRunMode: true, Resource: ResourceContainer, Name: "additional", Num: 1, State: StateCreating},
		{DryRunMode: true, Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateCreated},
		{DryRunMode: true, Resource: ResourceContainer, Name: "additional", Num: 1, State: StateCreated},
	}
	createOutput = []OutputLine{
		{Resource: ResourceNetwork, Name: "sample network", State: StateCreating},
		{Resource: ResourceNetwork, Name: "sample network", State: StateCreated},
		{Resource: ResourceVolume, Name: "sample-volume", State: StateCreating},
		{Resource: ResourceVolume, Name: "sample-volume", State: StateCreated},
		{Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateCreating},
		{Resource: ResourceContainer, Name: "additional", Num: 1, State: StateCreating},
		{Resource: ResourceContainer, Name: "additional", Num: 1, State: StateCreated},
		{Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateCreated},
	}
	startOutput = []OutputLine{
		{Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateStarting},
		{Resource: ResourceContainer, Name: "additional", Num: 1, State: StateStarting},
		{Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateStarted},
		{Resource: ResourceContainer, Name: "additional", Num: 1, State: StateStarted},
	}
	recreateDryRunOutput = []OutputLine{
		{DryRunMode: true, Resource: ResourceContainer, Name: "additional", Num: 1, State: StateStopping},
		{DryRunMode: true, Resource: ResourceContainer, Name: "additional", Num: 1, State: StateStopped},
		{DryRunMode: true, Resource: ResourceContainer, Name: "additional", Num: 1, State: StateRemoving},
		{DryRunMode: true, Resource: ResourceContainer, Name: "additional", Num: 1, State: StateRemoved},
		{DryRunMode: true, Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateRecreate},
		{DryRunMode: true, Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateRecreated},
	}
	recreateOutput = []OutputLine{
		{Resource: ResourceContainer, Name: "additional", Num: 1, State: StateStopping},
		{Resource: ResourceContainer, Name: "additional", Num: 1, State: StateStopped},
		{Resource: ResourceContainer, Name: "additional", Num: 1, State: StateRemoving},
		{Resource: ResourceContainer, Name: "additional", Num: 1, State: StateRemoved},
		{Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateRecreate},
		{Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateRecreated},
	}
	restartOutput = []OutputLine{
		{Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateStarting},
		{Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateStarted},
	}
	downOutput = []OutputLine{
		{Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateStopping},
		{Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateStopped},
		{Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateRemoving},
		{Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateRemoved},
		{Resource: ResourceVolume, Name: "sample-volume", State: StateRemoving},
		{Resource: ResourceVolume, Name: "sample-volume", State: StateRemoved},
		{Resource: ResourceNetwork, Name: "sample network", State: StateRemoving},
		{Resource: ResourceNetwork, Name: "sample network", State: StateRemoved},
	}

	createDryRunOutputResourceMap = map[NamedResource]OutputLine{
		{"Network", "sample network"}:   {DryRunMode: true, Resource: ResourceNetwork, Name: "sample network", State: StateCreated},
		{"Volume", "sample-volume"}:     {DryRunMode: true, Resource: ResourceVolume, Name: "sample-volume", State: StateCreated},
		{"Container", "sample_service"}: {DryRunMode: true, Resource: ResourceContainer, Name: "sample_service", Num: 1, State: StateCreated},
		{"Container", "additional"}:     {DryRunMode: true, Resource: ResourceContainer, Name: "additional", Num: 1, State: StateCreated},
	}
)

func TestOutput(t *testing.T) {
	project, err := loaderAdditional.Load(context.Background())
	if err != nil {
		panic(err)
	}

	type testCase struct {
		lines     string
		expected  []OutputLine
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

	var out Output
	out.ParseOutput("", createDryRunTxt, "testdata", project, false)

	assert.Assert(t, out.Out == "")
	assert.Assert(t, out.Err == createDryRunTxt)

	assert.Assert(t, cmp.Equal(out.Resource, createDryRunOutputResourceMap))
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
