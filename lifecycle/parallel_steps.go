package lifecycle

import (
	"context"
	"fmt"

	"github.com/weka/go-weka-observability/instrumentation"

	"github.com/weka/go-steps-engine/throttling"
)

type ParallelStepsError struct {
	Errors []error
}

func (e *ParallelStepsError) Error() string {
	return fmt.Sprintf("parallel steps failed: %v", e.Errors)
}

type ParallelSteps struct {
	// Name of the parallel steps group
	Name string
	// Predicates must all be true for the step to be executed
	Predicates []PredicateFunc
	// Abort the whole flow if one of the predicates is false
	AbortOnPredicatesFalse bool
	// The steps to execute in parallel
	Steps []Step
	// The function to execute if the step is failed
	OnFail func(context.Context, string, error) error
	// fields to pass to the nested steps engine
	StateKeeper StateKeeper
	Throttler   throttling.Throttler
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

func (s *ParallelSteps) ShouldContinueOnError() bool {
	// In parallel steps, we do not continue on error, we collect all errors
	return false
}

func (s *ParallelSteps) GetPredicates() []PredicateFunc {
	if s.Predicates == nil {
		return []PredicateFunc{}
	}
	return s.Predicates
}

func (s *ParallelSteps) AppendPredicate(predicate PredicateFunc) {
	s.Predicates = append(s.Predicates, predicate)
}

func (s *ParallelSteps) HasState() bool {
	return false
}

func (s *ParallelSteps) GetStepStateName() string {
	panic("not supported for ParallelSteps")
}

func (s *ParallelSteps) GetSucceededState() *StepState {
	panic("not supported for ParallelSteps")
}

func (s *ParallelSteps) ShouldSkip(ctx context.Context, object StateKeeper) bool {
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

func (s *ParallelSteps) HasNestedSteps() bool {
	return true
}

func (s *ParallelSteps) GetNestedSteps() []Step {
	return s.Steps
}

func (s *ParallelSteps) SetStateKeeperAndThrottler(stateKeeper StateKeeper, throttler throttling.Throttler) {
	s.StateKeeper = stateKeeper
	s.Throttler = throttler

	for _, step := range s.Steps {
		if step.HasNestedSteps() {
			step.SetStateKeeperAndThrottler(stateKeeper, throttler)
		}
	}
}

func (s *ParallelSteps) ValidateParallelSteps() error {
	if len(s.Steps) == 0 {
		return fmt.Errorf("no steps provided for parallel execution")
	}
	for _, step := range s.Steps {
		if step.ShouldFinishOnSuccess() {
			return fmt.Errorf("step %s in parallel steps cannot have ShouldFinishOnSuccess=true", step.GetName())
		}
		if step.ShouldContinueOnError() {
			return fmt.Errorf("step %s in parallel steps cannot have ShouldContinueOnError=true", step.GetName())
		}
	}
	return nil
}

func (s *ParallelSteps) RunStep(ctx context.Context) error {
	ctx, _, end := instrumentation.GetLogSpan(ctx, "RunParallelSteps", "parallel_steps_group", s.Name)
	defer end()

	if err := s.ValidateParallelSteps(); err != nil {
		return err
	}

	// Run all steps in parallel
	errs := make(chan error, len(s.Steps))
	for _, step := range s.Steps {
		go func(ctx context.Context, step Step) {
			stepCtx, stepLogger, spanEnd := instrumentation.GetLogSpan(ctx, step.GetName())
			defer spanEnd()

			// Check preconditions
			for _, predicate := range step.GetPredicates() {
				if !predicate() {
					stepLogger.Info("Skipping step due to predicate returning false")
					errs <- nil
					return
				}
			}

			err := step.RunStep(stepCtx)
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

func (s *ParallelSteps) SetState(state *State) {
	panic("ParallelSteps does not support SetState")
}
