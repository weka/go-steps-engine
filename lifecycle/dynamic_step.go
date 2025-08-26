package lifecycle

import (
	"context"

	"github.com/weka/go-steps-engine/throttling"
)

type DynamicStep struct {
	// Name of the step
	Name string

	// State configures step state tracking. If set, the step's execution state will be persisted via StateKeeper.
	// If nil, no state tracking is performed.
	State *State

	// Should the step be run if the state is already succeeded
	// Preconditions will also be evaluated and must be true
	SkipStepStateCheck bool

	// Predicates must all be true for the step to be executed
	Predicates []PredicateFunc
	// Abort the whole flow if one of the predicates is false
	AbortOnPredicatesFalse bool
	// Continue on error
	// If the step fails, the flow will continue
	ContinueOnError bool
	// Should the step finish flow execution successfully
	FinishOnSuccess bool
	// Internal step
	step Step
	// This function will be called to get the step dynamically
	GetStep func() Step
	// The function to execute if the step failed
	OnFail func(context.Context, string, error) error

	Throttling *throttling.ThrottlingSettings

	// fields to pass to the nested steps engine
	StateKeeper StateKeeper
	Throttler   throttling.Throttler
}

func (s *DynamicStep) getStep() Step {
	if s.step == nil {
		s.step = s.GetStep()
		// Propagate DynamicStep's state configuration to the nested step
		if s.HasState() {
			s.step.SetState(s.State)
		}
	}

	return s.step
}

func (s *DynamicStep) GetName() string {
	return s.Name
}

func (s *DynamicStep) ShouldAbortOnFalsePredicates() bool {
	return s.AbortOnPredicatesFalse
}

func (s *DynamicStep) ShouldContinueOnError() bool {
	return s.ContinueOnError
}

func (s *DynamicStep) ShouldFinishOnSuccess() bool {
	return s.FinishOnSuccess
}

func (s *DynamicStep) GetPredicates() []PredicateFunc {
	if s.Predicates == nil {
		return []PredicateFunc{}
	}
	return s.Predicates
}

func (s *DynamicStep) HasState() bool {
	return s.State != nil
}

func (s *DynamicStep) GetStepStateName() string {
	if s.State == nil {
		return ""
	}
	if s.State.Name != "" {
		return s.State.Name
	}
	// Fallback to step name if no name provided in State
	return s.GetName()
}

func (s *DynamicStep) GetSucceededState() *StepState {
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

func (s *DynamicStep) ShouldSkip(ctx context.Context, stateKeeper StateKeeper) bool {
	// Check if step is already done or if it should be able to run again
	if stateKeeper != nil && s.HasState() && !s.SkipStepStateCheck {
		state, _ := stateKeeper.GetStepState(ctx, s.GetStepStateName())
		return state != nil && state.StatusEqual(StepStatusSucceeded)
	}
	return false
}

func (s *DynamicStep) GetFailureCallback() func(context.Context, string, error) error {
	return s.OnFail
}

func (s *DynamicStep) IsThrottled() bool {
	return s.Throttling != nil
}

func (s *DynamicStep) GetThrottlingSettings() *throttling.ThrottlingSettings {
	return s.Throttling
}

func (s *DynamicStep) HasNestedSteps() bool {
	return true
}

func (s *DynamicStep) SetStateKeeperAndThrottler(stateKeeper StateKeeper, throttler throttling.Throttler) {
	s.StateKeeper = stateKeeper
	s.Throttler = throttler

	step := s.getStep()
	if step.HasNestedSteps() {
		step.SetStateKeeperAndThrottler(stateKeeper, throttler)
	}
}

func (s *DynamicStep) RunStep(ctx context.Context) error {
	reconSteps := StepsEngine{
		Steps:       []Step{s.getStep()},
		StateKeeper: s.StateKeeper,
		Throttler:   s.Throttler,
	}
	return reconSteps.Run(ctx)
}

func (s *DynamicStep) SetState(state *State) {
	s.State = state
}
