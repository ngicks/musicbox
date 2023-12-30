package composeloader

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/ngicks/musicbox/fsutil"
	"github.com/spf13/afero"
)

var ErrInvalidInput = errors.New("invalid input")

var aferoFsType = reflect.TypeOf((*afero.Fs)(nil)).Elem()

// ProjectDirOption is an option for project dir extracted from Archive.
// All paths must be slash separated even on Window.
type ProjectDirOption struct {
	// fs.FS which includes compose.yml and any other related files.
	Archive fs.FS
	// prefix path for copy destination of Archive.
	Prefix string
	// path for compose.yml in Archive
	ComposeYml string
	// Target directory in which Load will dump some or all of contents stored in Archive.
	// TempDir is allowed to be empty, in that case return value of os.MkdirTemp("", "some_pat_*") is used instead.
	TempDir string
}

// Err returns a non nil error if o is invalid, otherwise returns nil.
func (o ProjectDirOption) Err() error {
	if o.Archive == nil {
		return fmt.Errorf("%w: Archive is nil", ErrInvalidInput)
	}
	if o.ComposeYml == "" {
		return fmt.Errorf("%w: ComposeYml is empty", ErrInvalidInput)
	}
	if o.Prefix != "" {
		prefix := filepath.Clean(filepath.FromSlash(o.Prefix))
		if filepath.IsAbs(prefix) || strings.Contains(prefix, "..") {
			return fmt.Errorf("%w: Prefix specifies out of TempDir, prefix = %s", ErrInvalidInput, prefix)
		}
	}
	if o.TempDir != "" {
		s, err := os.Stat(o.TempDir)
		if err != nil {
			return fmt.Errorf("%w: taking stat of TempDir failed because of %w", ErrInvalidInput, err)
		}
		if !s.IsDir() {
			return fmt.Errorf("%w: TempDir is not at dir, mode is %b", ErrInvalidInput, s.Mode())
		}
	}
	return nil
}

// localize converts all paths to localized form
// by calling filepath.FromSlash on each path field.
func (o ProjectDirOption) localize() ProjectDirOption {
	return ProjectDirOption{
		Archive:    o.Archive,
		Prefix:     filepath.Clean(filepath.FromSlash(o.Prefix)),
		ComposeYml: filepath.FromSlash(o.ComposeYml),
		TempDir:    filepath.FromSlash(o.TempDir),
	}
}

// Prepare copies contents of Archive into TempDir and mkdir all directories under TempDir specified by distSet,
// returns mutable afero.Fs instances by setting each field of mutableFsSet.
// dirSet and mutableFsInstances flat structs and must be paired.
// For mutableFsSet to be mutated by Prepare, it must be pointer to the struct.
//
// dirSet and mutableFsSet must be paired structs.
// dirSet must have a or more exported string fields.
// mutableFsSet must have exact same field names as dirSet and all must be afero.Fs type.
// Note that all paths must be slash separated.
//
// For example, definitions and call signature should be like below:
//
//	type DirSet struct {
//		RuntimeEnvFiles string
//	}
//
//	type MutableFsSet struct {
//		RuntimeEnvFiles afero.Fs
//	}
//
//	var dirSet DirSet
//	var mutableFsSet MutableFsSet
//	composePath, err := LoadOption{}.Prepare(dirSet, &mutableFsSet)
func (o ProjectDirOption) Prepare(dirSet, mutableFsSet any) (composePath string, err error) {
	localOpt := o.localize()

	if err := localOpt.Err(); err != nil {
		return "", err
	}

	dRv := reflect.ValueOf(dirSet)
	mRv := reflect.ValueOf(mutableFsSet)

	if err := checkValidity(dRv, mRv); err != nil {
		return "", err
	}

	if localOpt.TempDir == "" {
		localOpt.TempDir, err = os.MkdirTemp("", "composeloader_*")
		if err != nil {
			return "", err
		}
		defer func() {
			if err != nil {
				_ = os.RemoveAll(localOpt.TempDir)
			}
		}()
	}

	tempDirPath := localOpt.TempDir
	if localOpt.Prefix != "" {
		tempDirPath = filepath.Join(tempDirPath, localOpt.Prefix)
	}
	tempDir := afero.NewBasePathFs(afero.NewOsFs(), tempDirPath)
	err = fsutil.CopyFS(tempDir, localOpt.Archive)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = fsutil.CleanDir(tempDir, "")
		}
	}()

	composePath = filepath.Join(localOpt.TempDir, localOpt.ComposeYml)
	_, err = tempDir.Stat(localOpt.ComposeYml)
	if err != nil {
		return "", fmt.Errorf("%w: could not read ComposeYml path, %w", ErrInvalidInput, err)
	}

	for i := 0; i < dRv.NumField(); i++ {
		field := dRv.Field(i)
		// field.String() does not panic upon invoked for non string field.
		// That's not what we want it to be.
		name, path := dRv.Type().Field(i).Name, field.Interface().(string)
		path = filepath.Clean(filepath.FromSlash(path))

		err = tempDir.MkdirAll(path, fs.ModeDir&0o777)
		if err != nil {
			return "", err
		}

		fsys := afero.NewBasePathFs(afero.NewOsFs(), filepath.Join(localOpt.TempDir, path))
		aferoField := mRv.FieldByName(name)
		aferoField.Set(reflect.ValueOf(fsys))
	}

	return composePath, nil
}

func checkValidity(dRv, mRv reflect.Value) error {
	if dRv.Kind() != reflect.Struct {
		return fmt.Errorf(
			"%w: dirSet must be a struct type but kind is %s",
			ErrInvalidInput, dRv.Kind(),
		)
	}

	if mRv.Kind() != reflect.Pointer || mRv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf(
			"%w: mutableFsSet must be a pointer type pointing to a struct but kind is %s",
			ErrInvalidInput, mRv.Kind(),
		)
	}

	mRv = mRv.Elem()

	if dRv.NumField() != mRv.NumField() {
		return fmt.Errorf(
			"%w: unmatched NumField, dirSet and mutableFsSet must have exact same keyed exported fields,"+
				" dirSet has %d fields, mutableFsSet has %d fields.",
			ErrInvalidInput, dRv.NumField(), mRv.NumField(),
		)
	}

	mFieldSet := make(map[string]struct{})
	for i := 0; i < mRv.NumField(); i++ {
		mFieldSet[mRv.Type().Field(i).Name] = struct{}{}
		field := mRv.Type().Field(i)
		if !field.Type.Implements(aferoFsType) {
			return fmt.Errorf(
				"%w: mutableFsSet must only have exported afero.Fs field, but is %T",
				ErrInvalidInput, field,
			)
		}
	}

	for i := 0; i < dRv.NumField(); i++ {
		// It does not need to be exact same layout (definition order).
		dirSetField := dRv.Field(i)
		dirSetFieldName := dRv.Type().Field(i).Name
		if dirSetField.Kind() != reflect.String {
			return fmt.Errorf(
				"%w: dirSet must only have exported string fields, but field %s has %s field",
				ErrInvalidInput, dirSetFieldName, dirSetField.Kind(),
			)
		}
		if _, ok := mFieldSet[dirSetFieldName]; !ok {
			return fmt.Errorf(
				"%w: dirSet and mutableFsSet must have exact same keyed exported fields, but field %s does not exist in mutableFsSet",
				ErrInvalidInput, dirSetFieldName,
			)
		}

		v := dirSetField.Interface().(string)
		if v == "" {
			return fmt.Errorf("%w: dirSet is specifying empty directory", ErrInvalidInput)
		}
		v = filepath.Clean(filepath.FromSlash(v))
		if filepath.IsAbs(v) || strings.Contains(v, "..") {
			return fmt.Errorf("%w: dirSet is specifying absolute directory or parent directory.", ErrInvalidInput)
		}
	}

	return nil
}
