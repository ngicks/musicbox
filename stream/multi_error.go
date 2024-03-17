package stream

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

var bufPool = &sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func getBuf() *bytes.Buffer {
	return bufPool.Get().(*bytes.Buffer)
}

func putBuf(b *bytes.Buffer) {
	if b.Cap() > 64*1024 {
		// See https://golang.org/issue/23199
		return
	}
	b.Reset()
	bufPool.Put(b)
}

var _ error = multiError{}
var _ fmt.Formatter = multiError{}

type multiError []error

// NewMultiError wraps errors into single error ignoring nil error in errs.
// If all errors are nil or len(errs) == 0, NewMultiError returns nil.
func NewMultiError(errs []error) error {
	var multiErr multiError
	for _, err := range errs {
		if err != nil {
			multiErr = append(multiErr, err)
		}
	}

	if len(multiErr) == 0 {
		return nil
	}

	return multiErr
}

// NewMultiErrorUnchecked wraps errors into single error.
// As suffix "unchecked" implies it does not do any filtering for errs.
// The returned error is always non nil even if all errors are nil or len(errs) == 0.
func NewMultiErrorUnchecked(errs []error) error {
	return multiError(errs)
}

func (me multiError) str(verb string) string {
	if len(me) == 0 {
		return "MultiError: "
	}

	buf := getBuf()
	defer putBuf(buf)

	_, _ = buf.WriteString("MultiError: ")

	for _, e := range me {
		_, _ = fmt.Fprintf(buf, verb, e)
		_, _ = buf.WriteString(", ")
	}

	buf.Truncate(buf.Len() - 2)

	return buf.String()
}

func (me multiError) Error() string {
	return me.str("%s")
}

func (me multiError) Unwrap() []error {
	return me
}

// Format implements fmt.Formatter.
//
// Format propagates given flags, width, precision and verb into each error.
// Then it concatenates each result with ", " suffix.
//
// Without Format, me is less readable when printed through fmt.*printf family functions.
// e.g. Format produces lines like
// (%+v) MultiError: errors, exampleErr: Foo=foo Bar=bar Baz=baz
// (%#v) MultiError: &errors.errorString{s:"errors"}, &mymodule.exampleErr{Foo:"foo", Bar:"bar", Baz:"baz"}
// instead of (w/o Format)
// (%+v) stream.multiError{(*errors.errorString)(0xc00002c300), (*mymodule.exampleErr)(0xc000102630)}
// (%#v) [824633901824 824634779184]
func (me multiError) Format(state fmt.State, verb rune) {
	var format strings.Builder

	_ = format.WriteByte('%')

	for _, flag := range []byte{'-', '+', '#', ' ', '0'} {
		if state.Flag(int(flag)) {
			_ = format.WriteByte(flag)
		}
	}

	if wid, ok := state.Width(); ok {
		_, _ = format.WriteString(strconv.FormatInt(int64(wid), 10))
	}
	if prec, ok := state.Precision(); ok {
		_ = format.WriteByte('.')
		_, _ = format.WriteString(strconv.FormatInt(int64(prec), 10))
	}

	_, _ = format.WriteRune(verb)

	state.Write([]byte(me.str(format.String())))
}
