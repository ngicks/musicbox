package composeloader

import (
	"context"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
)

type Loader struct {
	Config  types.ConfigDetails
	Options [](func(*loader.Options))
}

func FromDir[T any](d *ProjectDir[T], options []func(*loader.Options)) (*Loader, error) {
	config, err := ConfigFromPath(d.ComposeYmlPath())
	if err != nil {
		return nil, err
	}
	config, err = PreloadConfigDetails(config)
	if err != nil {
		return nil, err
	}

	return &Loader{
		Config:  config,
		Options: options,
	}, nil
}

func (l *Loader) Load(ctx context.Context) (*types.Project, error) {
	return loader.LoadWithContext(ctx, l.Config, l.Options...)
}

func (l *Loader) Preload() error {
	config, err := PreloadConfigDetails(l.Config)
	if err != nil {
		return err
	}
	l.Config = config
	return nil
}

func (l *Loader) Reload() error {
	config, err := ReloadConfigDetails(l.Config)
	if err != nil {
		return err
	}
	l.Config = config
	return nil
}

func cloneConfigDetails(conf types.ConfigDetails) types.ConfigDetails {
	return types.ConfigDetails{
		Version:     conf.Version,
		WorkingDir:  conf.WorkingDir,
		ConfigFiles: cloneConfigFiles(conf.ConfigFiles),
		Environment: conf.Environment.Clone(),
	}
}

func cloneConfigFiles(files []types.ConfigFile) []types.ConfigFile {
	out := make([]types.ConfigFile, len(files))
	for idx, f := range files {
		out[idx] = types.ConfigFile{
			Filename: f.Filename,
			Content:  slices.Clone(f.Content),
			Config:   maps.Clone(f.Config),
		}
	}
	return out
}

// ConfigFromPath converts paths to types.ConfigDetails.
// No loading will be done. If contents should be loaded, call PreloadConfigDetails with the returned config.
//
// WorkingDir will be set to the parent of path.
// Environment will be os.Environ.
//
// If any path is not readable or points to non regular file, it returns an error.
func ConfigFromPath(path string, additional ...string) (types.ConfigDetails, error) {
	checkPath := func(p string) error {
		s, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("FromComposeYml: %w", err)
		}
		if s.Mode()&fs.ModeType != 0 {
			return fmt.Errorf("FromComposeYml: path is not a regular file, path = %s", p)
		}
		return nil
	}

	if err := checkPath(path); err != nil {
		return types.ConfigDetails{}, err
	}

	paths := []string{path}
	if len(additional) > 0 {
		for _, path := range additional {
			if err := checkPath(path); err != nil {
				return types.ConfigDetails{}, err
			}
		}
		paths = append(paths, additional...)
	}

	configFiles := types.ToConfigFiles(paths)

	config := types.ConfigDetails{
		WorkingDir:  filepath.Dir(path),
		ConfigFiles: configFiles,
		Environment: types.NewMapping(os.Environ()),
	}

	return config, nil
}

// PreloadConfigDetails loads content and parse content if each corresponding field is not present in given conf.
func PreloadConfigDetails(conf types.ConfigDetails) (types.ConfigDetails, error) {
	cloned := cloneConfigDetails(conf)

	if len(cloned.ConfigFiles) == 0 {
		return types.ConfigDetails{}, fmt.Errorf("PreloadConfigDetails: ConfigFiles must not be empty")
	}

	if cloned.WorkingDir == "" {
		cloned.WorkingDir = filepath.Dir(cloned.ConfigFiles[0].Filename)
	}

	for i, confFile := range cloned.ConfigFiles {
		if len(confFile.Content) == 0 {
			bin, err := os.ReadFile(confFile.Filename)
			if err != nil {
				return types.ConfigDetails{}, fmt.Errorf("PreloadConfigDetails: %w", err)
			}
			confFile.Content = bin
		}
		if len(confFile.Config) == 0 {
			parsed, err := loader.ParseYAML(confFile.Content)
			if err != nil {
				return types.ConfigDetails{}, fmt.Errorf("PreloadConfigDetails: %w", err)
			}
			confFile.Config = parsed
		}
		cloned.ConfigFiles[i] = confFile
	}

	return cloned, nil
}

// ReloadConfigDetails is almost identical to PreloadConfigDetails
// however this function erases each file's Content and Config fields before loading.
func ReloadConfigDetails(conf types.ConfigDetails) (types.ConfigDetails, error) {
	cloned := cloneConfigDetails(conf)

	for i, f := range cloned.ConfigFiles {
		f.Config = nil
		f.Content = nil
		cloned.ConfigFiles[i] = f
	}

	return PreloadConfigDetails(cloned)
}
