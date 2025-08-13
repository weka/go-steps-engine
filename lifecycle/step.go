package lifecycle

import (
	"context"

	"github.com/weka/go-steps-engine/throttling"
	"github.com/weka/go-steps-engine/util"
)

type SingleStep struct {
	// Name of the step
	// NOTE: put explicit name for throttled funcs to ensure it's static and not affected by magic names change
	Name string

	// Step state should not be "succeeded" for the step to be executed
	StepStateName    string
	StepStateReason  string
	StepStateMessage string

	// Should the step be run if the state is already succeeded
	// Preconditions will also be evaluated and must be true
	SkipStepStateCheck bool

	// Predicates must all be true for the step to be executed
	Predicates []PredicateFunc

	// Continue on predicates false
	AbortOnPredicatesFalse bool

	// Finish execution successfully if operation ran and completed
	FinishOnSuccess bool

	// Continue on error
	// If the step fails, the flow will continue, but the step will be marked as failed
	ContinueOnError bool

	// The function to execute
	Run StepFunc

	// The function to execute if the step is failed
	OnFail func(context.Context, string, error) error

	Throttling *throttling.ThrottlingSettings
}

func (s *SingleStep) RunStep(ctx context.Context) error {
	return s.Run(ctx)
}

func (s *SingleStep) ShouldFinishOnSuccess() bool {
	return s.FinishOnSuccess
}

func (s *SingleStep) GetName() string {
	if s.Name == "" {
		// Get name of the function that is run by the step
		return util.GetFunctionName(s.Run)
	}
	return s.Name
}

func (s *SingleStep) HasState() bool {
	return s.StepStateName != ""
}

func (s *SingleStep) GetSucceededState() *StepState {
	return &StepState{
		Name:    s.StepStateName,
		Reason:  s.StepStateReason,
		Message: s.StepStateMessage,
		Status:  StepStatusSucceeded,
	}
}

func (s *SingleStep) ShouldAbortOnFalsePredicates() bool {
	return s.AbortOnPredicatesFalse
}

func (s *SingleStep) ShouldContinueOnError() bool {
	return s.ContinueOnError
}

func (s *SingleStep) GetPredicates() []PredicateFunc {
	if s.Predicates == nil {
		return []PredicateFunc{}
	}
	return s.Predicates
}

func (s *SingleStep) ShouldSkip(ctx context.Context, stateKeeper StateKeeper) bool {
	// Check if step is already done or if it should be able to run again
	if stateKeeper != nil && s.StepStateName != "" && !s.SkipStepStateCheck {
		s, _ := stateKeeper.GetStepState(ctx, s.StepStateName)
		return s != nil && s.StatusEqual(StepStatusSucceeded)
	}
	return false
}

func (s *SingleStep) GetFailureCallback() func(context.Context, string, error) error {
	return s.OnFail
}

func (s *SingleStep) IsThrottled() bool {
	return s.Throttling != nil
}

func (s *SingleStep) GetThrottlingSettings() *throttling.ThrottlingSettings {
	return s.Throttling
}

func (s *SingleStep) HasNestedSteps() bool {
	return false
}

func (s *SingleStep) SetStateKeeperAndThrottler(stateKeeper StateKeeper, throttler throttling.Throttler) {
	panic("SingleStep does not support SetStateKeeperAndThrottler")
}
