package lifecycle

import (
	"errors"
	"testing"
)

type targetError struct{}

func (*targetError) Error() string { return "target" }

func TestErrorsAsReachesCauseThroughEngineErrors(t *testing.T) {
	target := &targetError{}
	for name, err := range map[string]error{
		"pointer": &StepRunError{Err: &WaitError{Err: target}},
		"value":   StepRunError{Err: WaitError{Err: target}},
	} {
		var got *targetError
		if !errors.As(err, &got) || got != target {
			t.Errorf("%s: errors.As did not reach the cause", name)
		}
	}
}
