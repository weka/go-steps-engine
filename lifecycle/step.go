package lifecycle

import (
	"context"

	"github.com/weka/go-steps-engine/throttling"
	"github.com/weka/go-steps-engine/util"
)

// StepStateOverrides allows customizing step state attributes.
// When EnableState is true and overrides are not provided, sensible defaults are used.
type StepStateOverrides struct {
	// Name of the state condition. If empty, defaults to step name.
	Name string
	// Reason for the state when step succeeds. If empty, uses default.
	Reason string
	// Message for the state when step succeeds. If empty, uses default.
	Message string
}

type SingleStep struct {
	// Name of the step
	// NOTE: put explicit name for throttled funcs to ensure it's static and not affected by magic names change
	Name string

	// EnableState controls whether step state should be tracked.
	// When true, the step's execution state will be persisted via StateKeeper.
	EnableState bool

	// StateOverrides allows customizing state attributes when EnableState is true.
	// If not provided or fields are empty, sensible defaults are used.
	StateOverrides StepStateOverrides

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
	return s.EnableState
}

func (s *SingleStep) GetStepStateName() string {
	if !s.EnableState {
		return ""
	}
	if s.StateOverrides.Name != "" {
		return s.StateOverrides.Name
	}
	// Fallback to step name if no override provided
	return s.GetName()
}

func (s *SingleStep) GetSucceededState() *StepState {
	if !s.EnableState {
		return nil
	}

	reason := ""
	if s.StateOverrides.Reason != "" {
		reason = s.StateOverrides.Reason
	}

	message := ""
	if s.StateOverrides.Message != "" {
		message = s.StateOverrides.Message
	}

	return &StepState{
		Name:    s.GetStepStateName(),
		Reason:  reason,
		Message: message,
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
	if stateKeeper != nil && s.HasState() && !s.SkipStepStateCheck {
		state, _ := stateKeeper.GetStepState(ctx, s.GetStepStateName())
		return state != nil && state.StatusEqual(StepStatusSucceeded)
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
