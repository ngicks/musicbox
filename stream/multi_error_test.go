package stream

import (
	"errors"
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestMultiError(t *testing.T) {
	for _, errs := range [][]error{
		{nil, nil},
		{},
		nil,
	} {
		assert.NilError(t, NewMultiError(errs))
		assert.Assert(t, NewMultiErrorNoCheck(errs) != nil)
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
			e := NewMultiErrorNoCheck(tc.input)
			formatted := fmt.Sprintf("%s\n%v\n%+v\n%#v\n%d\n%T\n%9.3f", e, e, e, e, e, e, e)
			assert.Assert(t, cmp.Equal(formatted, tc.expected))
		})
	}
}

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
