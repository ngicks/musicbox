package fsutil

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/afero"
)

var (
	bufPool = &sync.Pool{
		New: func() any {
			buf := make([]byte, 64*1024)
			return &buf
		},
	}
)

func getBuf() *[]byte {
	return bufPool.Get().(*[]byte)
}

func putBuf(buf *[]byte) {
	bufPool.Put(buf)
}

type SafeWriteOptionOption func(o *SafeWriteOption)

func WithTmpDir(tmpDirName string) SafeWriteOptionOption {
	return func(o *SafeWriteOption) {
		o.tmpDirName = tmpDirName
	}
}

func WithDisableMkdir(disableMkdir bool) SafeWriteOptionOption {
	return func(o *SafeWriteOption) {
		o.disableMkdir = disableMkdir
	}
}

func WithDirPerm(dirPerm fs.FileMode) SafeWriteOptionOption {
	return func(o *SafeWriteOption) {
		o.dirPerm = dirPerm.Perm()
	}
}

func WithForcePerm(forcePerm bool) SafeWriteOptionOption {
	return func(o *SafeWriteOption) {
		o.forcePerm = forcePerm
	}
}

func validNonPattern(s string, cat string) error {
	if strings.Contains(s, "*") {
		return fmt.Errorf("%w: %s contains '*'", ErrBadPattern, cat)
	}
	if strings.ContainsFunc(s, func(r rune) bool { return r == '/' || r == filepath.Separator }) {
		// always slash.
		return fmt.Errorf("%w: %s contains path separator '"+string(filepath.Separator)+"'", ErrBadPattern, cat)
	}
	return nil
}

func WithPrefix(prefix string) (SafeWriteOptionOption, error) {
	if err := validNonPattern(prefix, "prefix"); err != nil {
		return nil, fmt.Errorf("WithPrefix: %w", err)
	}
	return func(o *SafeWriteOption) {
		o.prefix = prefix
	}, nil
}

func WithSuffix(suffix string) (SafeWriteOptionOption, error) {
	if err := validNonPattern(suffix, "suffix"); err != nil {
		return nil, fmt.Errorf("WithPrefix: %w", err)
	}
	return func(o *SafeWriteOption) {
		o.suffix = suffix
	}, nil
}

func WithRandomSuffix(randomSuffix bool) SafeWriteOptionOption {
	return func(o *SafeWriteOption) {
		o.randomSuffix = randomSuffix
	}
}

func WithOwner(uid, gid int) SafeWriteOptionOption {
	return func(o *SafeWriteOption) {
		o.uid = uid
		o.gid = gid
	}
}

func WithDefaultPostProcesses(defaultPostProcesses []SafeWritePostProcess) SafeWriteOptionOption {
	copied := make([]SafeWritePostProcess, len(defaultPostProcesses))
	copy(copied, defaultPostProcesses)
	return func(o *SafeWriteOption) {
		o.defaultPostProcesses = copied
	}
}

func WithDisableSync(disableSync bool) SafeWriteOptionOption {
	return func(o *SafeWriteOption) {
		o.disableSync = disableSync
	}
}

type SafeWritePostProcess func(fsys afero.Fs, name string, file afero.File) error

func ValidateClose(r io.Closer) SafeWritePostProcess {
	return func(fsys afero.Fs, name string, file afero.File) error {
		return r.Close()
	}
}

func TeeHasher(r io.Reader, h hash.Hash, expected []byte) (piped io.Reader, validator SafeWritePostProcess) {
	piped = io.TeeReader(r, h)
	validator = ValidateCheckSum(h, expected)
	return
}

func ValidateCheckSum(h hash.Hash, expected []byte) SafeWritePostProcess {
	return func(fsys afero.Fs, name string, file afero.File) error {
		actual := h.Sum(nil)
		if bytes.Equal(expected, actual) {
			return nil
		}
		return fmt.Errorf(
			"%w: expected = %s, actual = %s",
			ErrHashSumMismatch, hex.EncodeToString(expected), hex.EncodeToString(actual),
		)
	}
}

// Should it use builder pattern?

type SafeWriteOption struct {
	// If non empty string is set, SafeWrite tries make a directory under fsys root and uses this as temporary file target.
	// Otherwise SafeWrite writes files under filepath.Dir(name).
	tmpDirName string
	// If true and parent directories of dst and tmpDirName are non existent, returns an err immediately.
	disableMkdir bool
	// If non zero, SafeWrite uses this perm as an argument for MkdirAll.
	dirPerm fs.FileMode
	// If true, SafeWrite perform Chmod after each file creation.
	forcePerm bool
	// If non empty string is set, SafeWrite adds the prefix and/or suffix to temporary files.
	// SafeWrite adds "_" after prefix and before suffix.
	// Both are not allowed to have an '*'.
	prefix, suffix string
	// If true, name is suffixed with a randomly generate string and the opened file guaranteed not to overwrite any existing file.
	// However this does not prevent generated files from being overwritten, since dst could be name of randomized files.
	randomSuffix bool
	// If non negative number, SafeWrite performs Chown after each file creation.
	uid, gid int
	// validators which would be executed after validators passed to SafeWrite is done successfully.
	defaultPostProcesses []SafeWritePostProcess
	// If true, SafeWrite does not perform sync
	disableSync bool
}

func NewSafeWriteOption(opts ...SafeWriteOptionOption) *SafeWriteOption {
	o := &SafeWriteOption{
		uid: -1,
		gid: -1,
	}

	for _, opt := range opts {
		opt(o)
	}

	return o
}

func (o SafeWriteOption) Apply(opts ...SafeWriteOptionOption) *SafeWriteOption {
	for _, opt := range opts {
		opt(&o)
	}
	return &o
}

func (o SafeWriteOption) SafeWrite(
	fsys afero.Fs,
	dst string,
	perm fs.FileMode,
	r io.Reader,
	postProcesses ...SafeWritePostProcess,
) (err error) {
	// always slash.
	dst = filepath.FromSlash(dst)

	tmpDir := o.tmpDirName
	if tmpDir == "" {
		// guaranteed to be non empty string
		tmpDir = filepath.Dir(dst)
	}

	if !o.disableMkdir {
		err = mkdirAll(fsys, tmpDir, o.dirPerm)
		if err != nil {
			return fmt.Errorf("SafeWrite, mkdirAll: %w", err)
		}
	}

	name := filepath.Base(dst)
	if name == "." || name == string(filepath.Separator) {
		return fmt.Errorf(
			"SafeWrite, open: %w: dir = %s, base = %s",
			ErrBadName, filepath.Dir(dst), filepath.Base(dst),
		)
	}

	var (
		tmpName string
		f       afero.File
	)
	if o.randomSuffix || (o.tmpDirName == "" && o.prefix == "" && o.suffix == "") {
		rand.Uint64()
		f, err = OpenFileRandom(
			fsys,
			tmpDir,
			strings.Join(slices.DeleteFunc([]string{o.prefix, name, "*", o.suffix}, func(s string) bool { return s == "" }), "_"),
			200|perm.Perm(), // at least readable for the process.
		)
		if err == nil {
			tmpName = f.Name()
		}
	} else {
		f, err = fsys.OpenFile(
			filepath.Join(tmpDir, strings.Join([]string{o.prefix, name, "*", o.suffix}, "_")),
			os.O_CREATE|os.O_TRUNC|os.O_RDWR,
			perm.Perm(),
		)
		if err == nil {
			tmpName = f.Name()
		}
	}
	if err != nil {
		return fmt.Errorf("SafeWrite, open: %w", err)
	}

	var closeErr error
	closed := false
	// Multiple calls for Close is documented as undefined.
	closeOnce := func() error {
		if !closed {
			closed = true
			closeErr = f.Close()
		}
		return closeErr
	}

	defer func() {
		_ = closeOnce()
		if err != nil {
			_ = fsys.Remove(tmpName)
		}
	}()

	if o.forcePerm {
		err = fsys.Chmod(tmpName, perm.Perm())
		if err != nil {
			return fmt.Errorf("SafeWrite, chmod: %w", err)
		}
	}

	if o.uid >= 0 || o.gid >= 0 {
		uid, gid := o.uid, o.gid
		if uid < 0 || gid < 0 {
			uid, gid = max(uid, gid), max(uid, gid)
		}
		err = fsys.Chown(tmpName, uid, gid)
		if err != nil {
			return fmt.Errorf("SafeWrite, chown: %w", err)
		}
	}

	buf := getBuf()
	defer putBuf(buf)

	_, err = io.CopyBuffer(f, r, *buf)
	if err != nil {
		return fmt.Errorf("SafeWrite, copy: %w", err)
	}

	for _, pp := range postProcesses {
		err = pp(fsys, tmpDir, f)
		if err != nil {
			return fmt.Errorf("SafeWrite, validator: %w", err)
		}
	}
	for _, pp := range o.defaultPostProcesses {
		err = pp(fsys, tmpDir, f)
		if err != nil {
			return fmt.Errorf("SafeWrite, validator: %w", err)
		}
	}

	if !o.disableSync {
		err = f.Sync()
		if err != nil {
			return fmt.Errorf("SafeWrite, sync: %w", err)
		}
	}

	err = closeOnce()
	if err != nil {
		return fmt.Errorf("SafeWrite, close: %w", err)
	}

	if !o.disableMkdir {
		err = mkdirAll(fsys, filepath.Dir(dst), o.dirPerm)
		if err != nil {
			return fmt.Errorf("SafeWrite, mkdirAll: %w", err)
		}
	}

	err = fsys.Rename(tmpName, dst)
	if err != nil {
		return fmt.Errorf("SafeWrite, rename: %w", err)
	}

	return nil
}

// mkdirAll calls MkdirAll on fsys.
// If dir is an invalid value ("" || "." || filepath.Separator),
func mkdirAll(fsys afero.Fs, dir string, perm fs.FileMode) error {
	// Some implementation might refuses to make "."
	// Implementations might also refuse other empty paths.
	// We are avoiding calling Mkdir on them.
	if emptyDir(dir) {
		return nil
	}
	perm = perm.Perm()
	if perm == 0 {
		perm = fs.ModePerm // 0o777
	}
	err := fsys.MkdirAll(dir, perm)
	if err != nil {
		return err
	}
	return nil
}

func emptyDir(dir string) bool {
	return dir == "" || dir == "." || dir == string(filepath.Separator) || dir == filepath.VolumeName(dir)+string(filepath.Separator)
}

func OpenFileRandom(fsys afero.Fs, dir string, pattern string, perm fs.FileMode) (afero.File, error) {
	if dir == "" {
		dir = os.TempDir()
	}

	if strings.Contains(pattern, string(filepath.Separator)) {
		return nil, fmt.Errorf("%w: pattern containers path separators", ErrBadPattern)
	}

	var prefix, suffix string
	if i := strings.LastIndex(pattern, "*"); i < 0 {
		prefix = pattern
	} else {
		prefix, suffix = pattern[:i], pattern[i+1:]
	}

	attempt := 0
	for {
		random := strconv.FormatUint(rand.Uint64(), 10)
		name := filepath.Join(dir, prefix+random+suffix)
		f, err := fsys.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 600|perm.Perm())
		if err == nil {
			return f, nil
		}
		if os.IsExist(err) {
			attempt++
			if attempt < 10000 {
				continue
			} else {
				return nil, fmt.Errorf(
					"%w: max retry exceeded while opening %s",
					ErrMaxRetry, filepath.Join(dir, prefix+"*"+suffix),
				)
			}
		} else {
			return nil, err
		}
	}
}
