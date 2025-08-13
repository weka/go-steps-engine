package lifecycle

import (
	"context"

	"github.com/weka/go-steps-engine/throttling"
)

type GroupedSteps struct {
	// Name of the steps group
	Name string
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
	Throttler    throttling.Throttler
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
	return false
}

func (s *GroupedSteps) GetSucceededState() *StepState {
	return nil
}

func (s *GroupedSteps) ShouldSkip(ctx context.Context, object StateKeeper) bool {
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
