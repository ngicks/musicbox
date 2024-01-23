package composeloader

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

type Loader struct {
	Config  types.ConfigDetails
	Options [](func(*loader.Options))
}

func FromDir[S, H any](d *ProjectDir[S, H], options []func(*loader.Options)) (*Loader, error) {
	config, err := ConfigFromPath(d.ComposeYmlPath())
	if err != nil {
		return nil, err
	}
	// From v2, we don't need to preload config since
	// we don't need reload every time a *types.Project instance is mutated.
	// In v2, every methods which mutates *types.Project clones itself.
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

// ConfigFromPath converts paths to types.ConfigDetails.
// It only examines readability of paths and makes up types.ConfigDetails from them.
//
// WorkingDir will be set to the parent directory of path.
// Environment will be os.Environ.
//
// If any path is not readable or points to a non regular file,
// it stop and returns the first error encountered.
func ConfigFromPath(path string, additional ...string) (types.ConfigDetails, error) {
	checkPath := func(p string) error {
		f, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("ConfigFromPath: %w", err)
		}
		_ = f.Close()
		s, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("ConfigFromPath: %w", err)
		}
		if !s.Mode().IsRegular() {
			return fmt.Errorf("ConfigFromPath: not a regular file")
		}
		return nil
	}

	if err := checkPath(path); err != nil {
		return types.ConfigDetails{}, err
	}

	paths := []string{filepath.ToSlash(path)}
	if len(additional) > 0 {
		for i, path := range additional {
			if err := checkPath(path); err != nil {
				return types.ConfigDetails{}, err
			}
			additional[i] = filepath.ToSlash(path)
		}
		paths = append(paths, additional...)
	}

	configFiles := types.ToConfigFiles(paths)

	config := types.ConfigDetails{
		WorkingDir:  dir(path),
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
		cloned.WorkingDir = dir(cloned.ConfigFiles[0].Filename)
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

func dir(p string) string {
	dir := filepath.Dir(filepath.ToSlash(p))
	if filepath.IsAbs(dir) {
		return dir
	}
	if dir == "." {
		return "./"
	}
	if strings.HasPrefix(dir, "..") {
		return dir
	}
	if !strings.HasPrefix(dir, "./") {
		return "./" + dir
	}
	return dir
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
