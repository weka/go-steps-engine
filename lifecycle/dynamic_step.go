package lifecycle

import (
	"context"

	"github.com/weka/go-steps-engine/throttling"
)

type DynamicStep struct {
	// Name of the step
	Name string
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
	Object    ObjectWithConditions
	Throttler throttling.Throttler
}

func (s *DynamicStep) getStep() Step {
	if s.step == nil {
		s.step = s.GetStep()
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

func (s *DynamicStep) HasCondition() bool {
	return false
}

func (s *DynamicStep) GetCondition() Condition {
	panic("DynamicStep does not have conditions")
}

func (s *DynamicStep) ShouldSkip(_ ObjectWithConditions) bool {
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

func (s *DynamicStep) SetObjectAndThrottler(object ObjectWithConditions, throttler throttling.Throttler) {
	s.Object = object
	s.Throttler = throttler

	step := s.getStep()
	if step.HasNestedSteps() {
		step.SetObjectAndThrottler(object, throttler)
	}
}

func (s *DynamicStep) RunStep(ctx context.Context) error {
	reconSteps := StepsEngine{
		Steps:     []Step{s.getStep()},
		Object:    s.Object,
		Throttler: s.Throttler,
	}
	return reconSteps.Run(ctx)
}
