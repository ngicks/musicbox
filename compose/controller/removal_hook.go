package controller

import (
	"strings"
)

type RemovalHook interface {
	OnRemove(serviceName []string) error
}

var _ RemovalHook = RemovalHookFn(nil)

type RemovalHookFn func(serviceNames []string) error

func (fn RemovalHookFn) OnRemove(serviceNames []string) error {
	return fn(serviceNames)
}

var _ RemovalHook = CombinedRemovalHook{}

type CombinedRemovalHook []RemovalHook

type combinedError []error

func (e combinedError) Error() string {
	var builder strings.Builder
	_, _ = builder.WriteString("combined error: ")
	for i, err := range e {
		if i != 0 {
			_, _ = builder.WriteString(", ")
		}
		_, _ = builder.WriteString(err.Error())
	}
	return builder.String()
}

func (e combinedError) Unwrap() []error {
	return e
}

func (h CombinedRemovalHook) OnRemove(serviceName []string) error {
	var errors []error
	for _, hook := range h {
		err := hook.OnRemove(serviceName)
		if err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		return combinedError(errors)
	}
	return nil
}

var _ RemovalHook = (*RecorderHook)(nil)

type RecorderHook struct {
	history [][]string
}

func (h *RecorderHook) OnRemove(serviceName []string) error {
	h.history = append(h.history, serviceName)
	return nil
}
