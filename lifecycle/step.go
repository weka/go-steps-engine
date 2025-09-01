package lifecycle

import (
	"context"

	"github.com/weka/go-steps-engine/throttling"
	"github.com/weka/go-steps-engine/util"
)

// State configures step state tracking. If set, the step's execution state will be persisted via StateKeeper.
// Name field is required when State is set.
type State struct {
	// Name of the state condition. Required when State is set.
	Name string
	// Reason for the state when step succeeds. If empty, uses default.
	Reason string
	// Message for the state when step succeeds. If empty, uses default.
	Message string
}

type SimpleStep struct {
	// Name of the step
	// NOTE: put explicit name for throttled funcs to ensure it's static and not affected by magic names change
	Name string

	// State configures step state tracking. If set, the step's execution state will be persisted via StateKeeper.
	// If nil, no state tracking is performed.
	State *State

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

func (s *SimpleStep) RunStep(ctx context.Context) error {
	return s.Run(ctx)
}

func (s *SimpleStep) ShouldFinishOnSuccess() bool {
	return s.FinishOnSuccess
}

func (s *SimpleStep) GetName() string {
	if s.Name == "" {
		// Get name of the function that is run by the step
		return util.GetFunctionName(s.Run)
	}
	return s.Name
}

func (s *SimpleStep) HasState() bool {
	return s.State != nil
}

func (s *SimpleStep) GetStepStateName() string {
	if s.State == nil {
		return ""
	}
	if s.State.Name != "" {
		return s.State.Name
	}
	// Fallback to step name if no name provided in State
	return s.GetName()
}

func (s *SimpleStep) GetSucceededState() *StepState {
	if s.State == nil {
		return nil
	}

	return &StepState{
		Name:    s.GetStepStateName(),
		Reason:  s.State.Reason,
		Message: s.State.Message,
		Status:  StepStatusSucceeded,
	}
}

func (s *SimpleStep) ShouldAbortOnFalsePredicates() bool {
	return s.AbortOnPredicatesFalse
}

func (s *SimpleStep) ShouldContinueOnError() bool {
	return s.ContinueOnError
}

func (s *SimpleStep) GetPredicates() []PredicateFunc {
	if s.Predicates == nil {
		return []PredicateFunc{}
	}
	return s.Predicates
}

func (s *SimpleStep) ShouldSkip(ctx context.Context, stateKeeper StateKeeper) bool {
	// Check if step is already done or if it should be able to run again
	if stateKeeper != nil && s.HasState() && !s.SkipStepStateCheck {
		state, _ := stateKeeper.GetStepState(ctx, s.GetStepStateName())
		return state != nil && state.StatusEqual(StepStatusSucceeded)
	}
	return false
}

func (s *SimpleStep) GetFailureCallback() func(context.Context, string, error) error {
	return s.OnFail
}

func (s *SimpleStep) IsThrottled() bool {
	return s.Throttling != nil
}

func (s *SimpleStep) GetThrottlingSettings() *throttling.ThrottlingSettings {
	return s.Throttling
}

func (s *SimpleStep) HasNestedSteps() bool {
	return false
}

func (s *SimpleStep) SetStateKeeperAndThrottler(stateKeeper StateKeeper, throttler throttling.Throttler) {
	panic("SimpleStep does not support SetStateKeeperAndThrottler")
}

func (s *SimpleStep) SetState(state *State) {
	s.State = state
}
