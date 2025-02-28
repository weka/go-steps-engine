package lifecycle

import (
	"context"
	"fmt"

	"github.com/weka/go-steps-engine/throttling"
	"github.com/weka/go-weka-observability/instrumentation"
)

type ParallelStepsError struct {
	Errors []error
}

func (e *ParallelStepsError) Error() string {
	return fmt.Sprintf("parallel steps failed: %v", e.Errors)
}

type SimpleStep struct {
	// Name of the step
	Name string
	// Predicates must all be true for the step to be executed
	Predicates []PredicateFunc
	// The function to execute
	Run StepFunc
}

type ParallelSteps struct {
	// Name of the parallel steps group
	Name string
	// Predicates must all be true for the step to be executed
	Predicates []PredicateFunc
	// Abort the whole flow if one of the predicates is false
	AbortOnPredicatesFalse bool
	// The steps to execute in parallel
	Steps []SimpleStep
	// The function to execute if the step is failed
	OnFail func(context.Context, string, error) error
}

func (s *ParallelSteps) GetName() string {
	return s.Name
}

func (s *ParallelSteps) ShouldAbortOnFalsePredicates() bool {
	return s.AbortOnPredicatesFalse
}

func (s *ParallelSteps) ShouldFinishOnSuccess() bool {
	return false
}

func (s *ParallelSteps) GetPredicates() []PredicateFunc {
	if s.Predicates == nil {
		return []PredicateFunc{}
	}
	return s.Predicates
}

func (s *ParallelSteps) HasCondition() bool {
	return false
}

func (s *ParallelSteps) GetCondition() Condition {
	panic("ParallelSteps do not have conditions")
}

func (s *ParallelSteps) ShouldSkip(_ ObjectWithConditions) bool {
	return false
}

func (s *ParallelSteps) GetFailureCallback() func(context.Context, string, error) error {
	return s.OnFail
}

func (s *ParallelSteps) IsThrottled() bool {
	return false
}

func (s *ParallelSteps) GetThrottlingSettings() *throttling.ThrottlingSettings {
	return nil
}

func (s *ParallelSteps) RunStep(ctx context.Context) error {
	ctx, _, end := instrumentation.GetLogSpan(ctx, "RunParallelSteps", "parallel_steps_group", s.Name)
	defer end()

	// Run all steps in parallel
	errs := make(chan error, len(s.Steps))
	for _, step := range s.Steps {
		go func(ctx context.Context, step SimpleStep) {
			stepCtx, stepLogger, spanEnd := instrumentation.GetLogSpan(ctx, step.Name)
			defer spanEnd()

			// Check preconditions
			for _, predicate := range step.Predicates {
				if !predicate() {
					return
				}
			}

			err := step.Run(stepCtx)
			if err != nil {
				stepLogger.SetError(err, "step failed")
				stepLogger.SetValues("stop_err", err.Error())
			}
			errs <- err
		}(ctx, step)
	}

	// Wait for all steps to finish
	var errors []error
	for range s.Steps {
		err := <-errs
		if err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return &ParallelStepsError{Errors: errors}
	}
	return nil
}
