package stream

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

var _ error = multiError{}
var _ fmt.Formatter = multiError{}

type multiError []error

// NewMultiError filters non nil errors and returns a wrapped error.
// Any nil error in errs will be ignored.
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

// NewMultiErrorUnchecked wraps multiple error.
// Unlike NewMultiError, NewMultiErrorUnchecked returns non nil error
// even if errs do not contains any non nil error.
func NewMultiErrorUnchecked(errs []error) error {
	return multiError(errs)
}

func (me multiError) str(verb string) string {
	if len(me) == 0 {
		return "MultiError: "
	}

	var out bytes.Buffer

	_, _ = out.WriteString("MultiError: ")

	for _, e := range me {
		_, _ = fmt.Fprintf(&out, verb, e)
		_, _ = out.WriteString(", ")
	}

	out.Truncate(out.Len() - 2)

	return out.String()
}

func (me multiError) Error() string {
	return me.str("%s")
}

func (me multiError) Unwrap() []error {
	return me
}

// Format implements fmt.Formatter.
// Format propagates given format
// Without Format, me is less readable when printed through fmt.*printf family functions.
// e.g. It prints
// (%+v) MultiError: errors, exampleErr: Foo=foo Bar=bar Baz=baz
// (%#v) MultiError: &errors.errorString{s:"errors"}, &mymodule.exampleErr{Foo:"foo", Bar:"bar", Baz:"baz"}
// rather than (w/o Format)
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
