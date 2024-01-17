package storage

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

func PrepareHandle[S, H any](base afero.Fs, pathSet S, initialContents any) (H, error) {
	var handle, zero H

	sRv := reflect.ValueOf(pathSet)
	hRv := reflect.ValueOf(&handle)

	if err := validPrepareInput(sRv, hRv); err != nil {
		return zero, err
	}

	hRv = hRv.Elem()

	for i := 0; i < sRv.NumField(); i++ {
		field := sRv.Field(i)
		// field.String() does not panic upon invoked for non string field.
		// That's not what we want it to be.
		name, path := sRv.Type().Field(i).Name, field.Interface().(string)
		path = filepath.Clean(filepath.FromSlash(path))

		err := base.MkdirAll(path, fs.ModeDir|0o777)
		if err != nil {
			return zero, err
		}

		fsys := afero.NewBasePathFs(base, path)
		aferoField := hRv.FieldByName(name)
		aferoField.Set(reflect.ValueOf(fsys))
	}

	if initialContents != nil {
		icRv := reflect.ValueOf(initialContents)
		err := validCopyContentsInput(hRv, icRv, true)
		if err != nil {
			return zero, err
		}
		err = CopyContents(handle, initialContents)
		if err != nil {
			return zero, err
		}
	}

	return handle, nil

}

// ProjectDir is an option for project dir extracted from Archive.
// All paths must be slash separated even on Window.
type ProjectDir[T any] struct {
	// fs.FS which includes compose.yml and any other related files.
	archive fs.FS
	// prefix path for copy destination of Archive.
	prefix string
	// path for compose.yml in Archive
	composeYml string
	// Target directory in which Load will dump some or all of contents stored in Archive.
	// tempDir is allowed to be empty, in that case return value of os.MkdirTemp("", "some_pat_*") is used instead.
	tempDir string

	dirSet              any
	pathHandle          *T
	initialContents     any
	cleanTempDirOnError bool
}

type ProjectDirOption[T any] func(d *ProjectDir[T])

// WithPrefix sets prefix for archive extraction destination.
// If the last WithPrefix option is applied with non empty prefix,
// The fs is extracted from archive to filepath.Join(tempDir, prefix).
func WithPrefix[T any](prefix string) ProjectDirOption[T] {
	return func(d *ProjectDir[T]) {
		d.prefix = prefix
	}
}

// WithTempDir change tempDir to an arbitrary location.
// If tempDir is empty, d uses os.MkdirTemp().
func WithTempDir[T any](tempDir string) ProjectDirOption[T] {
	return func(d *ProjectDir[T]) {
		d.tempDir = tempDir
	}
}

// WithCleanTempDirOnError decides if d erase all contents under the temp dir
// at an occurrence of an error in Prepare.
func WithCleanTempDirOnError[T any](c bool) ProjectDirOption[T] {
	return func(d *ProjectDir[T]) {
		d.cleanTempDirOnError = c
	}
}

func WithInitialContent[T any](initialContents any) (ProjectDirOption[T], error) {
	var handle T
	if err := validCopyContentsInput(reflect.ValueOf(handle), reflect.ValueOf(initialContents), true); err != nil {
		return nil, err
	}
	return func(d *ProjectDir[T]) {
		d.initialContents = initialContents
	}, nil
}

// PrepareProjectDir returns a newly created ProjectDir.
// archive must contain compose.yml at the path composeYml is pointing to.
//
// dirSet and pathHandle must be flat structs and must have exact same field names to each other.
// For pathHandle is mutated by Prepare, it must be a pointer to a non nil instance of the struct.
//
// Exported fields of dirSet and pathHandle must only be string type, afero.Fs type respectively.
//
// In case caller does not need to mutate prepared dir, arguments can just be both nil.
//
// Note that all paths should be slash separated for better compatibility.
//
// For example, definitions and call signature should be like below:
//
//	type DirSet struct {
//		RuntimeEnvFiles string
//	}
//
//	type pathHandle struct {
//		RuntimeEnvFiles afero.Fs
//	}
//
//	composePath, err := composeloader.PrepareProjectDir[pathHandle](archive, "path/to/compose.yml", DirSet{RuntimeEnvFiles: "runtime_env"})
//
// You can use the code generator to generate those types.
//
//	go run -mod=mod github.com/ngicks/musicbox/composeloader/cmd/gentypes@latest -pkg example -fields Foo,Bar,Baz -o ./example.generated.go
//
// See ./example for generated result.
func PrepareProjectDir[T any](archive fs.FS, composeYml string, dirSet any, opts ...ProjectDirOption[T]) (*ProjectDir[T], error) {
	var handle T
	d := &ProjectDir[T]{
		archive:    archive,
		composeYml: composeYml,
		dirSet:     dirSet,
		pathHandle: &handle,
	}

	for _, opt := range opts {
		opt(d)
	}

	err := d.prepare()
	if err != nil {
		return nil, err
	}

	return d, nil
}

// Err returns a non nil error if o is invalid, otherwise returns nil.
func (o ProjectDir[T]) err() error {
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

	if err := validPrepareInput(reflect.ValueOf(o.dirSet), reflect.ValueOf(o.pathHandle)); err != nil {
		return err
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

func ValidatePrepareInput(pathSet, pathHandle any) error {
	return validPrepareInput(reflect.ValueOf(pathSet), reflect.ValueOf(pathHandle))
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
			"%w: pathHandle must be a pointer type pointing to a struct but kind is %s",
			ErrInvalidInput, hRv.Kind(),
		)
	}

	hRv = hRv.Elem()

	if sRv.NumField() != hRv.NumField() {
		return fmt.Errorf(
			"%w: unmatched NumField, dirSet and pathHandle must have exact same keyed exported fields,"+
				" dirSet has %d fields, pathHandle has %d fields.",
			ErrInvalidInput, sRv.NumField(), hRv.NumField(),
		)
	}

	hFieldSet := make(map[string]struct{})
	for i := 0; i < hRv.NumField(); i++ {
		hFieldSet[hRv.Type().Field(i).Name] = struct{}{}
		field := hRv.Type().Field(i)
		if !field.Type.Implements(aferoFsType) {
			return fmt.Errorf(
				"%w: pathHandle must only have exported afero.Fs field, but is %s",
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
				"%w: dirSet and pathHandle must have exact same keyed exported fields, but field %s does not exist in pathHandle",
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

// localize converts all paths to localized form
// by calling filepath.FromSlash on each path field.
func (o ProjectDir[T]) localize() ProjectDir[T] {
	o.prefix = filepath.Clean(filepath.FromSlash(o.prefix))
	o.composeYml = filepath.Clean(filepath.FromSlash(o.composeYml))
	o.tempDir = filepath.Clean(filepath.FromSlash(o.tempDir))
	return o
}

// Prepare copies contents of Archive into a temp directory and
// mkdir all directories specified by dirSet under the temp directory.
// Handlers for those created directories are returned through pathHandle as mutable afero.Fs instances.
func (d *ProjectDir[T]) prepare() (err error) {
	if err := d.err(); err != nil {
		return err
	}

	*d = d.localize()

	sRv := reflect.ValueOf(d.dirSet)
	hRv := reflect.ValueOf(d.pathHandle)

	hRv = hRv.Elem()

	if isEmpty(d.tempDir) {
		tempDir, err := os.MkdirTemp("", "composeloader_*")
		if err != nil {
			return err
		}
		d.tempDir = tempDir
		defer func() {
			if d.cleanTempDirOnError && err != nil {
				_ = os.RemoveAll(d.tempDir)
			}
		}()
	}

	composeDirPath := d.tempDir
	if !isEmpty(d.prefix) {
		composeDirPath = filepath.Join(composeDirPath, d.prefix)
	}
	composeDir := afero.NewBasePathFs(afero.NewOsFs(), composeDirPath)
	err = fsutil.CopyFS(composeDir, d.archive)
	if err != nil {
		return err
	}
	defer func() {
		if d.cleanTempDirOnError && err != nil {
			_ = fsutil.CleanDir(composeDir, "")
		}
	}()

	_, err = composeDir.Stat(d.composeYml)
	if err != nil {
		return fmt.Errorf("%w: could not read ComposeYml path, %w", ErrInvalidInput, err)
	}

	tempDir := afero.NewBasePathFs(afero.NewOsFs(), d.tempDir)
	if sRv.Kind() == reflect.Struct {
		for i := 0; i < sRv.NumField(); i++ {
			field := sRv.Field(i)
			// field.String() does not panic upon invoked for non string field.
			// That's not what we want it to be.
			name, path := sRv.Type().Field(i).Name, field.Interface().(string)
			path = filepath.Clean(filepath.FromSlash(path))

			err = tempDir.MkdirAll(path, fs.ModeDir&0o777)
			if err != nil {
				return err
			}

			fsys := afero.NewBasePathFs(afero.NewOsFs(), filepath.Join(d.tempDir, path))
			aferoField := hRv.FieldByName(name)
			aferoField.Set(reflect.ValueOf(fsys))
		}
	}

	if d.initialContents != nil {
		err = CopyContents(d.Handle(), d.initialContents)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *ProjectDir[T]) Dir() string {
	return d.tempDir
}

func (d *ProjectDir[T]) ComposeYmlPath() string {
	composeDirPath := d.tempDir
	if !isEmpty(d.prefix) {
		composeDirPath = filepath.Join(composeDirPath, d.prefix)
	}
	return filepath.Join(composeDirPath, d.composeYml)
}

func (d *ProjectDir[T]) Handle() *T {
	return d.pathHandle
}
