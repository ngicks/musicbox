package stream

import (
	"errors"
	"strings"
	"testing"
)

func assertErrorsIs(t *testing.T, err, target error) {
	if !errors.Is(err, target) {
		t.Fatalf("errors.Is(err, target) returned false, err = %#v, target = %#v", err, target)
	}
}

func assertNotErrorsIs(t *testing.T, err, target error) {
	if errors.Is(err, target) {
		t.Fatalf("errors.Is(err, target) returned true, err = %#v, target = %#v", err, target)
	}
}

func assertErrorsAs[T any](t *testing.T, err error) {
	var e T
	if !errors.As(err, &e) {
		t.Fatalf("errors.As(err, target) returned false, expected to be type %T, but is %#v", e, err)
	}
}

func assertErrorContains(t *testing.T, err error, substr string) {
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("err.Error() does not contains %q, err.Err() = %s,  err = %#v", substr, err.Error(), err)
	}
}

func assertNilInterface(t *testing.T, v any) {
	if v != nil {
		t.Fatalf("not nil: v = %#v, expected to be nil", v)
	}
}

func assertNonNilInterface(t *testing.T, v any) {
	if v == nil {
		t.Fatal("nil: expected to be non nil")
	}
}

func assertBool(t *testing.T, b bool, format string, mgsArgs ...any) {
	if !b {
		t.Fatalf(format, mgsArgs...)
	}
}

func assertEq[T comparable](t *testing.T, x, y T) {
	if x != y {
		t.Fatalf("not equal: left = %v, right = %v", x, y)
	}
}
