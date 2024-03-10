package stream

import (
	"fmt"
	"strconv"
	"strings"
)

var _ error = multiError{}
var _ fmt.Formatter = multiError{}

type multiError []error

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

func (me multiError) str(verb string) string {
	if len(me) == 0 {
		return "MultiError: "
	}

	var out strings.Builder

	_, _ = out.WriteString("MultiError: ")

	for i, e := range me {
		_, _ = out.WriteString(fmt.Sprintf(verb, e))
		if i != len(me)-1 {
			_, _ = out.WriteString(", ")
		}
	}
	return out.String()
}

func (me multiError) Error() string {
	return me.str("%s")
}

func (me multiError) Unwrap() []error {
	return me
}

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
