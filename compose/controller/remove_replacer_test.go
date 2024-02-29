package controller

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	composeV2Types "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	compose "github.com/ngicks/musicbox/compose/service"
	"github.com/ngicks/musicbox/compose/testhelper"
	"gotest.tools/v3/assert"
)

func TestRemoveReplacer_dind(t *testing.T) {
	projectName := "orchestrator-controller-remove-replacer-test"

	// almost same tests are run twice.
	// I am not totally sure why but after return of compose create it magically removes the intermediate container
	// while it returns an error.
	// This does not mean we do not need this method; this could be a thing that only happen in test setups.
	//
	// The first is for ensuring we are successfully causing the target problem,
	// where intermediate containers left behind prevents compose from replacing services.
	//
	// The second is to confirm our code successfully revert the situation back to the normal.

	testFn := func(fn func(t *testing.T, oldController, newController *Controller) error) {
		testhelper.RunComposeTest(
			projectName,
			[]string{"./testdata/compose.yml"},
			func(loader *compose.LoaderProxy) {
				var (
					err error
				)

				oldService, _ := loader.LoadComposeService(
					context.Background(),
					func(p *composeV2Types.Project) error {
						p, _ = p.WithServicesEnabled(slices.Concat(p.ServiceNames(), p.DisabledServiceNames())...)
						return nil
					},
				)
				newLoader, _ := compose.NewLoaderProxy(
					loader.ProjectName(),
					func() composeV2Types.ConfigDetails {
						conf := loader.ConfigDetails()
						conf.ConfigFiles = append(conf.ConfigFiles, composeV2Types.ConfigFile{
							Filename: "./testdata/additive_pre.yml",
						})
						return conf
					}(),
					loader.Options(),
					nil,
				)
				newService, _ := newLoader.LoadComposeService(
					context.Background(),
					func(p *composeV2Types.Project) error {
						p, _ = p.WithServicesEnabled(slices.Concat(p.ServiceNames(), p.DisabledServiceNames())...)
						return nil
					},
				)

				logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

				oldController := New(oldService, &RecorderHook{}, WithLogger(logger))
				newController := New(newService, &RecorderHook{}, WithLogger(logger))

				_, err = newController.Create(context.Background())
				assert.NilError(t, err)

				findPre := func() types.ContainerJSON {
					client := loader.DockerCli().Client()
					containers, err := client.ContainerList(context.Background(), types.ContainerListOptions{
						All: true,
						Filters: filters.NewArgs(
							filters.Arg("label", fmt.Sprintf("%s=%s", api.ProjectLabel, projectName)),
							filters.Arg("label", fmt.Sprintf("%s=%s", api.ServiceLabel, "fake_pre")),
						),
					})
					if err != nil {
						panic(err)
					}

					detail, err := client.ContainerInspect(context.Background(), containers[0].ID)
					if err != nil {
						panic(err)
					}

					return detail
				}

				client := loader.DockerCli().Client()

				newPreCont := findPre()
				assert.NilError(t, err)

				_, err = oldController.Create(context.Background())
				assert.NilError(t, err)

				oldPreCont := findPre()

				// sleeping to ensure the container being created has newer Created (which is unix second).
				time.Sleep(time.Second)

				newPreCont.Config.Labels[api.ContainerReplaceLabel] = oldPreCont.ID
				_, err = client.ContainerCreate(
					context.Background(),
					newPreCont.Config,
					newPreCont.HostConfig,
					nil,
					nil,
					oldPreCont.ID[:12]+"_"+strings.TrimPrefix(oldPreCont.Name, "/"),
				)
				assert.NilError(t, err)

				assert.NilError(t, fn(t, oldController, newController))
			})
	}

	testFn(func(t *testing.T, oldController, newController *Controller) error {
		_, err := newController.Create(context.Background())
		if err == nil {
			return fmt.Errorf("newController.Create must return error")
		}
		return nil
	})

	testFn(func(t *testing.T, oldController, newController *Controller) error {
		err := newController.RemoveReplacer(context.Background())
		if err != nil {
			return err
		}
		_, err = newController.Create(context.Background())
		return err
	})
}
