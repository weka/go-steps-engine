package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestResultFor(t *testing.T) {
	step := &SimpleStep{Name: "s"}
	wrap := func(err error) error { return &StepRunError{Err: err, Step: step} }
	tests := []struct {
		name string
		err  error
		want time.Duration
	}{
		{"nil", nil, 30 * time.Second},
		{"wait without duration", wrap(NewWaitError(errors.New("x"))), 3 * time.Second},
		{"wait with duration", wrap(NewWaitErrorWithDuration(errors.New("x"), 15*time.Second)), 15 * time.Second},
		{"nested step errors", wrap(wrap(NewWaitErrorWithDuration(errors.New("x"), 7*time.Second))), 7 * time.Second},
		{"aborted by predicate", wrap(&AbortedByPredicate{}), 3 * time.Second},
		{"plain error", errors.New("boom"), 10 * time.Second},
		{"step error with plain cause", wrap(errors.New("boom")), 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResultFor(context.Background(), tt.err).RequeueAfter; got != tt.want {
				t.Fatalf("RequeueAfter = %v, want %v", got, tt.want)
			}
		})
	}
}
