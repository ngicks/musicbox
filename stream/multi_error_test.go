package stream

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"
)

func TestMultiError(t *testing.T) {
	for _, errs := range [][]error{
		{nil, nil},
		{},
		nil,
	} {
		assertNilInterface(t, NewMultiError(errs))
		assertBool(t, NewMultiErrorUnchecked(errs) != nil, "not NewMultiErrorUnchecked(errs) != nil")
	}

	type testCase struct {
		name     string
		input    []error
		expected string
	}
	for _, tc := range []testCase{
		{
			"combined",
			[]error{errors.New("errors"), &exampleErr{"foo", "bar", "baz"}},
			"MultiError: errors, exampleErr: Foo=foo Bar=bar Baz=baz\n" +
				"MultiError: errors, exampleErr: Foo=foo Bar=bar Baz=baz\n" +
				"MultiError: errors, exampleErr: Foo=foo Bar=bar Baz=baz\n" +
				"MultiError: &errors.errorString{s:\"errors\"}, &stream.exampleErr{Foo:\"foo\", Bar:\"bar\", Baz:\"baz\"}\n" +
				"MultiError: &{%!d(string=errors)}, &{%!d(string=foo) %!d(string=bar) %!d(string=baz)}\n" +
				"stream.multiError\n" +
				"MultiError: &{%!f(string=      err)}, &{%!f(string=      foo) %!f(string=      bar) %!f(string=      baz)}",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := NewMultiErrorUnchecked(tc.input)
			formatted := fmt.Sprintf("%s\n%v\n%+v\n%#v\n%d\n%T\n%9.3f", e, e, e, e, e, e, e)
			assertEq(t, tc.expected, formatted)
		})
	}

	nilMultiErr := NewMultiErrorUnchecked(nil)
	assertEq(t, "MultiError: ", nilMultiErr.Error())

	mult := NewMultiErrorUnchecked([]error{
		errors.New("foo"),
		fs.ErrClosed,
		&exampleErr{"foo", "bar", "baz"},
		errExample,
	})

	assertErrorsIs(t, mult, fs.ErrClosed)

	assertErrorsAs[*exampleErr](t, mult)
	assertErrorsIs(t, mult, errExample)
	assertNotErrorsIs(t, mult, errExampleUnknown)
}

var (
	errExample        = errors.New("example")
	errExampleUnknown = errors.New("unknown")
)

type exampleErr struct {
	Foo string
	Bar string
	Baz string
}

func (e *exampleErr) Error() string {
	if e == nil {
		return "exampleErr: nil"
	}
	return fmt.Sprintf("exampleErr: Foo=%s Bar=%s Baz=%s", e.Foo, e.Bar, e.Baz)
}
