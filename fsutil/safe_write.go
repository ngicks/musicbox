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

func WithForcePerm(forcePerm bool) SafeWriteOptionOption {
	return func(o *SafeWriteOption) {
		o.forcePerm = forcePerm
	}
}

func WithDisableRemoveOnErr(disableRemoveOnErr bool) SafeWriteOptionOption {
	return func(o *SafeWriteOption) {
		o.disableRemoveOnErr = disableRemoveOnErr
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

func WithPrefixSuffix(prefix, suffix string) (SafeWriteOptionOption, error) {
	errPre := validNonPattern(prefix, "prefix")
	errSuf := validNonPattern(suffix, "suffix")
	if errPre != nil || errSuf != nil {
		return nil, fmt.Errorf("WithPrefixSuffix: prefix err = %w, suffix err = %w", errPre, errSuf)
	}

	// TODO: warn if `prefix == "" && suffix == ""` ?

	return func(o *SafeWriteOption) {
		o.prefix = prefix
		o.suffix = suffix
	}, nil
}

func WithRandomPattern(randomPattern string) SafeWriteOptionOption {
	return func(o *SafeWriteOption) {
		o.randomPattern = randomPattern
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
	tmpFileOption

	// If true and parent directories of dst and tmpDirName are non existent, returns an err immediately.
	disableMkdir bool
	// If true, SafeWrite perform Chmod after each file creation.
	forcePerm bool
	// If true, SafeWrite will not try to delete temporary files on an occurrence of an error.
	disableRemoveOnErr bool

	// If non negative number, SafeWrite performs Chown after each file creation.
	uid, gid int
	// validators which would be executed after validators passed to SafeWrite is done successfully.
	defaultPostProcesses []SafeWritePostProcess
	// If true, SafeWrite does not perform sync
	disableSync bool
}

type tmpFileOption struct {
	// If non empty string is set, SafeWrite tries make a directory under fsys root and uses this as temporary file target.
	// Otherwise SafeWrite writes files under filepath.Dir(name).
	tmpDirName string
	// If non empty string is set, SafeWrite adds the prefix and/or suffix to temporary files.
	// Both are not allowed to have an '*'.
	prefix, suffix string
	// If true, name is suffixed with a randomly generate string and the opened file guaranteed not to overwrite any existing file.
	// However this does not prevent generated files from being overwritten, since dst could be name of randomized files.
	randomPattern string
}

func (o tmpFileOption) tempDir(path string) string {
	tmpDir := o.tmpDirName
	if tmpDir == "" {
		// guaranteed to be non empty string
		tmpDir = filepath.Dir(path)
	}
	tmpDir = filepath.Clean(tmpDir)
	return tmpDir
}

func (o tmpFileOption) suffixOrDefault() string {
	if o.suffix != "" {
		return o.suffix
	}

	tmpDir := filepath.Clean(o.tmpDirName)
	if isEmpty(tmpDir) {
		return ".tmp"
	}
	return ""
}

func (o tmpFileOption) matchTmpFile(path string) bool {
	tmpDir := filepath.Clean(o.tmpDirName)
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if !isEmpty(tmpDir) && dir != tmpDir {
		return false
	}
	return strings.HasPrefix(base, o.prefix) && strings.HasSuffix(base, o.suffixOrDefault())
}

func (o tmpFileOption) cleanTmp(fsys afero.Fs) error {
	root := "."
	tmpDir := filepath.Clean(o.tmpDirName)
	if !isEmpty(tmpDir) {
		root = tmpDir
	}

	return fs.WalkDir(afero.NewIOFS(fsys), root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				// tmp dir does not exist.
				return nil
			}
			return err
		}
		if path == root {
			return nil
		}

		if o.matchTmpFile(path) {
			return fsys.RemoveAll(path)
		}
		return nil
	})
}

func (o tmpFileOption) open(
	fsys afero.Fs,
	path string,
	perm fs.FileMode,
	openRandom func(fsys afero.Fs, dir string, pattern string, perm fs.FileMode) (afero.File, error),
	openFile func(name string, flag int, perm fs.FileMode) (afero.File, error),
) (f afero.File, err error) {
	tmpDir := o.tempDir(path)

	name := filepath.Clean(filepath.Base(path))
	if isEmpty(name) {
		return nil, fmt.Errorf("%w", ErrBadInput)
	}

	var openName string
	if o.randomPattern != "" {
		pat := o.randomPattern
		if !strings.Contains(o.randomPattern, "*") {
			pat += "*"
		}
		openName = o.prefix + name + pat + o.suffixOrDefault()
		f, err = openRandom(
			fsys,
			tmpDir,
			openName,
			perm.Perm(),
		)
	} else {
		openName = o.prefix + name + o.suffixOrDefault()
		f, err = openFile(
			filepath.Join(tmpDir, openName),
			os.O_CREATE|os.O_RDWR|os.O_EXCL,
			perm.Perm(),
		)
	}
	if err != nil {
		return nil, fmt.Errorf(
			"%w: dir = %s, base = %s",
			ErrBadName, tmpDir, openName,
		)
	}

	return f, nil
}

func (o tmpFileOption) openTmp(fsys afero.Fs, path string, perm fs.FileMode) (f afero.File, err error) {
	return o.open(fsys, path, perm, OpenFileRandom, fsys.OpenFile)
}

func (o tmpFileOption) openTmpDir(fsys afero.Fs, path string, perm fs.FileMode) (f afero.File, err error) {
	return o.open(
		fsys,
		path,
		perm,
		MkdirRandom,
		func(name string, flag int, perm fs.FileMode) (afero.File, error) {
			err := fsys.Mkdir(name, perm|0o300) // writable and executable, since we are creating files under.
			if err != nil {
				return nil, err
			}
			return fsys.Open(name)
		},
	)
}

func isEmpty(path string) bool {
	return path == "" || path == "." || path == string(filepath.Separator)
}

func NewSafeWriteOption(opts ...SafeWriteOptionOption) *SafeWriteOption {
	o := &SafeWriteOption{
		tmpFileOption: tmpFileOption{
			randomPattern: "-*",
		},
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

func (o SafeWriteOption) CleanTmp(fsys afero.Fs) error {
	if err := o.tmpFileOption.cleanTmp(fsys); err != nil {
		return fmt.Errorf("CleanTmp: %w", err)
	}
	return nil
}

func (o SafeWriteOption) safeWrite(
	fsys afero.Fs,
	dst string,
	perm fs.FileMode,
	openTmp func(fsys afero.Fs, path string, perm fs.FileMode) (f afero.File, err error),
	copyTo func(dst afero.File) error,
	postProcesses ...SafeWritePostProcess,
) (err error) {
	// always slash.
	dst = filepath.FromSlash(dst)

	if !o.disableMkdir {
		err = mkdirAll(fsys, o.tempDir(dst), fs.ModePerm)
		// We do not call chmod for dirs since
		// it can be invoked by the caller anytime if they wish to.
		if err != nil {
			return fmt.Errorf("SafeWrite, mkdirAll: %w", err)
		}
	}

	f, err := openTmp(fsys, dst, perm.Perm())
	if err != nil {
		return fmt.Errorf("SafeWrite, %w", err)
	}
	tmpName := f.Name()

	var closeErr error
	closed := false
	// Multiple calls for Close is documented as undefined.
	// Just simple boolean flag is enough since
	// the calling goroutine is only the current g.
	closeOnce := func() error {
		if !closed {
			closed = true
			closeErr = f.Close()
		}
		return closeErr
	}

	defer func() {
		_ = closeOnce()
		if err != nil && !o.disableRemoveOnErr {
			_ = fsys.Remove(tmpName)
		}
	}()

	err = copyTo(f)
	if err != nil {
		return fmt.Errorf("SafeWrite, copy: %w", err)
	}

	if o.forcePerm {
		err = fsys.Chmod(tmpName, perm.Perm()|0o300)
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

	for _, pp := range postProcesses {
		err = pp(fsys, tmpName, f)
		if err != nil {
			return fmt.Errorf("SafeWrite, validator: %w", err)
		}
	}
	for _, pp := range o.defaultPostProcesses {
		err = pp(fsys, tmpName, f)
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
		err = mkdirAll(fsys, filepath.Dir(dst), fs.ModePerm)
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

func (o SafeWriteOption) SafeWrite(
	fsys afero.Fs,
	path string,
	perm fs.FileMode,
	r io.Reader,
	postProcesses ...SafeWritePostProcess,
) (err error) {
	return o.safeWrite(
		fsys,
		path,
		perm,
		o.tmpFileOption.openTmp,
		func(dst afero.File) error {
			b := getBuf()
			defer putBuf(b)
			_, err := io.CopyBuffer(dst, r, *b)
			return err
		},
		postProcesses...,
	)
}

func (o SafeWriteOption) SafeWriteFs(
	fsys afero.Fs,
	dir string,
	perm fs.FileMode,
	src fs.FS,
	postProcesses ...SafeWritePostProcess,
) error {
	return o.safeWrite(
		fsys,
		dir,
		perm,
		o.openTmpDir,
		func(dst afero.File) error {
			return CopyFS(afero.NewBasePathFs(fsys, dst.Name()), src)
		},
		postProcesses...,
	)
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
	return openRandom(
		fsys,
		dir,
		pattern,
		perm,
		func(fsys afero.Fs, name string, perm fs.FileMode) (afero.File, error) {
			return fsys.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, perm|0o200) // at least writable
		},
	)
}

func MkdirRandom(fsys afero.Fs, dir string, pattern string, perm fs.FileMode) (afero.File, error) {
	return openRandom(
		fsys,
		dir,
		pattern,
		perm,
		func(fsys afero.Fs, name string, perm fs.FileMode) (afero.File, error) {
			err := fsys.Mkdir(name, perm)
			if err != nil {
				return nil, err
			}
			return fsys.Open(name)
		},
	)
}

func openRandom(
	fsys afero.Fs,
	dir string,
	pattern string,
	perm fs.FileMode,
	open func(fsys afero.Fs, name string, perm fs.FileMode) (afero.File, error),
) (afero.File, error) {
	if dir == "" {
		dir = "tmp"
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
		random := randomUint32Padded()
		name := filepath.Join(dir, prefix+random+suffix)
		f, err := open(fsys, name, perm.Perm())
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

func randomUint32Padded() string {
	s := strconv.FormatUint(uint64(rand.Uint32()), 10)
	var builder strings.Builder
	builder.Grow(len("4294967295"))
	r := len("4294967295") - len(s)
	for i := 0; i < r; i++ {
		builder.WriteByte('0')
	}
	builder.WriteString(s)
	return builder.String()
}
