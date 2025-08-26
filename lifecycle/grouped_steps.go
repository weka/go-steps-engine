package lifecycle

import (
	"context"

	"github.com/weka/go-steps-engine/throttling"
)

type GroupedSteps struct {
	// Name of the steps group
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
	// If the grouped steps fail, the flow will continue, but the step will be marked as failed
	ContinueOnError bool
	// Should the step finish flow execution successfully if all steps ran and completed
	FinishOnSuccess bool
	// The steps to execute sequentially (stops on first error)
	Steps []Step
	// The function to execute if the group of steps failed
	OnFail func(context.Context, string, error) error

	Throttling *throttling.ThrottlingSettings

	// fields to pass to the nested steps engine
	StateKeeper StateKeeper
	Throttler   throttling.Throttler
}

func (s *GroupedSteps) GetName() string {
	return s.Name
}

func (s *GroupedSteps) ShouldAbortOnFalsePredicates() bool {
	return s.AbortOnPredicatesFalse
}

func (s *GroupedSteps) ShouldContinueOnError() bool {
	return s.ContinueOnError
}

func (s *GroupedSteps) ShouldFinishOnSuccess() bool {
	return s.FinishOnSuccess
}

func (s *GroupedSteps) GetPredicates() []PredicateFunc {
	if s.Predicates == nil {
		return []PredicateFunc{}
	}
	return s.Predicates
}

func (s *GroupedSteps) HasState() bool {
	return s.State != nil
}

func (s *GroupedSteps) GetStepStateName() string {
	if s.State == nil {
		return ""
	}
	if s.State.Name != "" {
		return s.State.Name
	}
	// Fallback to step name if no name provided in State
	return s.GetName()
}

func (s *GroupedSteps) GetSucceededState() *StepState {
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

func (s *GroupedSteps) ShouldSkip(ctx context.Context, stateKeeper StateKeeper) bool {
	// Check if step is already done or if it should be able to run again
	if stateKeeper != nil && s.HasState() && !s.SkipStepStateCheck {
		state, _ := stateKeeper.GetStepState(ctx, s.GetStepStateName())
		return state != nil && state.StatusEqual(StepStatusSucceeded)
	}
	return false
}

func (s *GroupedSteps) GetFailureCallback() func(context.Context, string, error) error {
	return s.OnFail
}

func (s *GroupedSteps) IsThrottled() bool {
	return s.Throttling != nil
}

func (s *GroupedSteps) GetThrottlingSettings() *throttling.ThrottlingSettings {
	return s.Throttling
}

func (s *GroupedSteps) HasNestedSteps() bool {
	return true
}

func (s *GroupedSteps) SetStateKeeperAndThrottler(stateKeeper StateKeeper, throttler throttling.Throttler) {
	s.StateKeeper = stateKeeper
	s.Throttler = throttler
	for _, step := range s.Steps {
		if step.HasNestedSteps() {
			step.SetStateKeeperAndThrottler(stateKeeper, throttler)
		}
	}
}

func (s *GroupedSteps) RunStep(ctx context.Context) error {
	reconSteps := StepsEngine{
		Steps:       s.Steps,
		StateKeeper: s.StateKeeper,
		Throttler:   s.Throttler,
	}
	return reconSteps.Run(ctx)
}

func (s *GroupedSteps) SetState(state *State) {
	s.State = state
}
