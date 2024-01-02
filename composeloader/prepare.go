package composeloader

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"

	"github.com/ngicks/musicbox/fsutil"
	"github.com/spf13/afero"
)

var ErrInvalidInput = errors.New("invalid input")

var (
	aferoFsType = reflect.TypeOf((*afero.Fs)(nil)).Elem()
	fsFsType    = reflect.TypeOf((*fs.FS)(nil)).Elem()
)

// ProjectDir is an option for project dir extracted from Archive.
// All paths must be slash separated even on Window.
type ProjectDir struct {
	// fs.FS which includes compose.yml and any other related files.
	archive fs.FS
	// prefix path for copy destination of Archive.
	prefix string
	// path for compose.yml in Archive
	composeYml string
	// Target directory in which Load will dump some or all of contents stored in Archive.
	// tempDir is allowed to be empty, in that case return value of os.MkdirTemp("", "some_pat_*") is used instead.
	tempDir string

	cleanTempDirOnError bool
}

type ProjectDirOption func(d *ProjectDir)

// WithPrefix sets prefix for archive extraction destination.
// If the last WithPrefix option is applied with non empty prefix,
// The fs is extracted from archive to filepath.Join(tempDir, prefix).
func WithPrefix(prefix string) ProjectDirOption {
	return func(d *ProjectDir) {
		d.prefix = prefix
	}
}

// WithTempDir change tempDir to an arbitrary location.
// If tempDir is empty, d uses os.MkdirTemp().
func WithTempDir(tempDir string) ProjectDirOption {
	return func(d *ProjectDir) {
		d.tempDir = tempDir
	}
}

// WithCleanTempDirOnError sets if d erase all contents under the temp dir
// at an occurrence of an error in Prepare.
func WithCleanTempDirOnError(c bool) ProjectDirOption {
	return func(d *ProjectDir) {
		d.cleanTempDirOnError = c
	}
}

// NewProjectDir returns a newly created ProjectDir.
// archive must contain compose.yml at the path composeYml is pointing to.
func NewProjectDir(archive fs.FS, composeYml string, opts ...ProjectDirOption) *ProjectDir {
	d := &ProjectDir{
		archive:    archive,
		composeYml: composeYml,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// Err returns a non nil error if o is invalid, otherwise returns nil.
func (o ProjectDir) Err() error {
	if o.archive == nil {
		return fmt.Errorf("%w: archive is nil", ErrInvalidInput)
	}

	if isEmpty(o.composeYml) {
		return fmt.Errorf("%w: composeYml is empty", ErrInvalidInput)
	} else if !filepath.IsLocal(o.composeYml) {
		return newNotLocalErr("composeYml", o.composeYml)
	}

	if !isEmpty(o.prefix) && !filepath.IsLocal(o.prefix) {
		return newNotLocalErr("prefix", o.prefix)
	}

	if !isEmpty(o.tempDir) {
		s, err := os.Stat(o.tempDir)
		if err != nil {
			return fmt.Errorf("%w: taking stat of tempDir failed because of %w", ErrInvalidInput, err)
		}
		if !s.IsDir() {
			return fmt.Errorf("%w: tempDir is not a dir, mode is %b", ErrInvalidInput, s.Mode())
		}
	}

	return nil
}

func isEmpty(s string) bool {
	// filepath.Clean converts "" to "."
	return s == "" || s == "."
}

func newNotLocalErr(name, path string) error {
	return fmt.Errorf("%w: %s is not a local path, path = %s", ErrInvalidInput, name, path)
}

// localize converts all paths to localized form
// by calling filepath.FromSlash on each path field.
func (o ProjectDir) localize() ProjectDir {
	o.prefix = filepath.Clean(filepath.FromSlash(o.prefix))
	o.composeYml = filepath.Clean(filepath.FromSlash(o.composeYml))
	o.tempDir = filepath.Clean(filepath.FromSlash(o.tempDir))
	return o
}

// Prepare copies contents of Archive into a temp directory and
// mkdir all directories specified by dirSet under the temp directory.
// Handlers for those created directories are returned through dirHandle as mutable afero.Fs instances.
//
// dirSet and dirHandle must be flat structs and must have exact same field names to each other.
// For dirHandle is mutated by Prepare, it must be a pointer to a non nil instance of the struct.
//
// Exported fields of dirSet and dirHandle must only be string type, afero.Fs type respectively.
//
// In case caller does not need to mutate prepared dir, arguments can just be both nil.
//
// Note that all paths must be slash separated for better compatibility.
//
// For example, definitions and call signature should be like below:
//
//	type DirSet struct {
//		RuntimeEnvFiles string
//	}
//
//	type DirHandle struct {
//		RuntimeEnvFiles afero.Fs
//	}
//
//	var set DirSet
//	var handle DirHandle
//	composePath, err := composeloader.NewProjectDir(archive, "path/to/compose.yml").Prepare(set, &handle)
func (o ProjectDir) Prepare(dirSet, dirHandle any) (dir, composePath string, err error) {
	if err := o.Err(); err != nil {
		return "", "", err
	}

	lo := o.localize()

	sRv := reflect.ValueOf(dirSet)
	hRv := reflect.ValueOf(dirHandle)

	if err := validPrepareInput(sRv, hRv); err != nil {
		return "", "", err
	}

	hRv = hRv.Elem()

	if isEmpty(lo.tempDir) {
		lo.tempDir, err = os.MkdirTemp("", "composeloader_*")
		if err != nil {
			return "", "", err
		}
		defer func() {
			if lo.cleanTempDirOnError && err != nil {
				_ = os.RemoveAll(lo.tempDir)
			}
		}()
	}

	composeDirPath := lo.tempDir
	if !isEmpty(lo.prefix) {
		composeDirPath = filepath.Join(composeDirPath, lo.prefix)
	}
	composeDir := afero.NewBasePathFs(afero.NewOsFs(), composeDirPath)
	err = fsutil.CopyFS(composeDir, lo.archive)
	if err != nil {
		return "", "", err
	}
	defer func() {
		if lo.cleanTempDirOnError && err != nil {
			_ = fsutil.CleanDir(composeDir, "")
		}
	}()

	composePath = filepath.Join(composeDirPath, lo.composeYml)
	_, err = composeDir.Stat(lo.composeYml)
	if err != nil {
		return "", "", fmt.Errorf("%w: could not read ComposeYml path, %w", ErrInvalidInput, err)
	}

	tempDir := afero.NewBasePathFs(afero.NewOsFs(), lo.tempDir)
	if sRv.Kind() == reflect.Struct {
		for i := 0; i < sRv.NumField(); i++ {
			field := sRv.Field(i)
			// field.String() does not panic upon invoked for non string field.
			// That's not what we want it to be.
			name, path := sRv.Type().Field(i).Name, field.Interface().(string)
			path = filepath.Clean(filepath.FromSlash(path))

			err = tempDir.MkdirAll(path, fs.ModeDir&0o777)
			if err != nil {
				return "", "", err
			}

			fsys := afero.NewBasePathFs(afero.NewOsFs(), filepath.Join(lo.tempDir, path))
			aferoField := hRv.FieldByName(name)
			aferoField.Set(reflect.ValueOf(fsys))
		}
	}

	return lo.tempDir, composePath, nil
}

func validPrepareInput(sRv, hRv reflect.Value) error {
	if sRv.Kind() == reflect.Invalid && hRv.Kind() == reflect.Invalid {
		// both nil indicates no path and no handle.
		return nil
	}

	if sRv.Kind() != reflect.Struct {
		return fmt.Errorf(
			"%w: dirSet must be a struct type but kind is %s",
			ErrInvalidInput, sRv.Kind(),
		)
	}

	if hRv.Kind() != reflect.Pointer || hRv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf(
			"%w: dirHandle must be a pointer type pointing to a struct but kind is %s",
			ErrInvalidInput, hRv.Kind(),
		)
	}

	hRv = hRv.Elem()

	if sRv.NumField() != hRv.NumField() {
		return fmt.Errorf(
			"%w: unmatched NumField, dirSet and dirHandle must have exact same keyed exported fields,"+
				" dirSet has %d fields, dirHandle has %d fields.",
			ErrInvalidInput, sRv.NumField(), hRv.NumField(),
		)
	}

	hFieldSet := make(map[string]struct{})
	for i := 0; i < hRv.NumField(); i++ {
		hFieldSet[hRv.Type().Field(i).Name] = struct{}{}
		field := hRv.Type().Field(i)
		if !field.Type.Implements(aferoFsType) {
			return fmt.Errorf(
				"%w: dirHandle must only have exported afero.Fs field, but is %s",
				ErrInvalidInput, field.Type.String(),
			)
		}
	}

	for i := 0; i < sRv.NumField(); i++ {
		// It does not need to be exact same layout (definition order).
		dirSetField := sRv.Field(i)
		dirSetFieldName := sRv.Type().Field(i).Name
		if dirSetField.Kind() != reflect.String {
			return fmt.Errorf(
				"%w: dirSet must only have exported string fields, but field %s has %s field",
				ErrInvalidInput, dirSetFieldName, dirSetField.Kind(),
			)
		}
		if _, ok := hFieldSet[dirSetFieldName]; !ok {
			return fmt.Errorf(
				"%w: dirSet and dirHandle must have exact same keyed exported fields, but field %s does not exist in dirHandle",
				ErrInvalidInput, dirSetFieldName,
			)
		}

		v := dirSetField.Interface().(string)
		if v == "" {
			return fmt.Errorf("%w: dirSet specifies empty directory", ErrInvalidInput)
		}
		if !filepath.IsLocal(v) {
			return fmt.Errorf("%w: dirSet specifies absolute directory or parent directory.", ErrInvalidInput)
		}
	}

	return nil
}
