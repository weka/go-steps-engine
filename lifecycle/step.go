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

	// Condition that must be false for the step to be executed, set to True if the step is done succesfully
	Condition   string
	CondReason  string
	CondMessage string

	// Should the step be run if the condition is already true
	// Preconditions will also be evaluated and must be true
	SkipOwnConditionCheck bool

	// Predicates must all be true for the step to be executed
	Predicates []PredicateFunc

	// Continue on predicates false
	AbortOnPredicatesFalse bool

	// Finish execution successfully if operation ran and completed
	FinishOnSuccess bool

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

func (s *SingleStep) HasCondition() bool {
	return s.Condition != ""
}

func (s *SingleStep) GetCondition() Condition {
	return Condition{
		Name:    s.Condition,
		Reason:  s.CondReason,
		Message: s.CondMessage,
	}
}

func (s *SingleStep) ShouldAbortOnFalsePredicates() bool {
	return s.AbortOnPredicatesFalse
}

func (s *SingleStep) GetPredicates() []PredicateFunc {
	if s.Predicates == nil {
		return []PredicateFunc{}
	}
	return s.Predicates
}

func (s *SingleStep) ShouldSkip(object ObjectWithConditions) bool {
	// Check if step is already done or if the condition should be able to run again
	if object != nil && s.Condition != "" && !s.SkipOwnConditionCheck {
		return object.IsConditionTrue(s.Condition)
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
