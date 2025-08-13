package lifecycle

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AbortedByPredicate struct {
	error
}

type WaitError struct {
	Duration time.Duration
	Err      error
}

func (w WaitError) Error() string {
	return "wait-error:" + w.Err.Error()
}

func NewWaitError(err error) error {
	return &WaitError{Err: err}
}

func NewWaitErrorWithDuration(err error, duration time.Duration) error {
	return &WaitError{Err: err, Duration: duration}
}

type StepRunError struct {
	Err     error
	Subject StateKeeper
	Step    Step
}

func (e StepRunError) Error() string {
	if e.Subject != nil {
		return fmt.Sprintf("error reconciling object %s during phase %s: %v",
			e.Subject.GetSummary(),
			e.Step.GetName(),
			e.Err)
	} else {
		return fmt.Sprintf("error reconciling object during phase %s: %v",
			e.Step.GetName(),
			e.Err)
	}
}

type ConditionUpdateError struct {
	Err       error
	Subject   metav1.Object
	Condition metav1.Condition
}

func (e ConditionUpdateError) Error() string {
	return fmt.Sprintf("error updating condition %s for object %s: %v", e.Condition.Type, e.Subject.GetName(), e.Err)
}

type StateError struct {
	Property string
	Message  string
}

func (e StateError) Error() string {
	return fmt.Sprintf("invalid state: %s - %s", e.Property, e.Message)
}

type RetryableError struct {
	Err        error
	RetryAfter time.Duration
}

func (e RetryableError) Error() string {
	return fmt.Sprintf("retryable error: %v, retry after: %s", e.Err, e.RetryAfter)
}

// ExpectedError is an error that is expected to happen during
// reconciliation and should not result in step failure and retry.
type ExpectedError struct {
	Err error
}

func (e *ExpectedError) Error() string {
	return fmt.Sprintf("expected error: %v", e.Err)
}

func NewExpectedError(err error) error {
	return &ExpectedError{Err: err}
}
