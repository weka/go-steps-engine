package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/codes"

	"github.com/weka/go-steps-engine/throttling"
	"github.com/weka/go-weka-observability/instrumentation"
)

type StepFunc func(ctx context.Context) error

type Condition struct {
	Name    string
	Reason  string
	Message string
}

type Step interface {
	// Should the step be skipped if the condition is already true
	ShouldSkip(object ObjectWithConditions) bool
	// Run the underlying function of the step
	RunStep(ctx context.Context) error
	// Should the step become the last one in the flow if it succeeds
	ShouldFinishOnSuccess() bool
	// Get predicates that must be true for the step to be executed
	GetPredicates() []PredicateFunc
	// Should the whole flow be aborted if one of the predicates is false
	ShouldAbortOnFalsePredicates() bool
	// Get the function that is run if the step fails
	GetFailureCallback() func(context.Context, string, error) error
	// Does the step have a condition
	HasCondition() bool
	// Get the condition
	GetCondition() Condition
	// Is step throttled
	IsThrottled() bool
	// Get throttling settings
	GetThrottlingSettings() *throttling.ThrottlingSettings
	// Get step name
	GetName() string
}

type ObjectWithConditions interface {
	GetName() string
	// returns the namespace of the object (if it has one)
	GetNamespace() string
	// get information about object in one string (for error messages or logs)
	GetSummary() string
	IsConditionTrue(conditionName string) bool
	SetConditionTrue(ctx context.Context, condition Condition) error
	SetConditionFalse(ctx context.Context, condition Condition) error
}

type StepsEngine struct {
	Object    ObjectWithConditions
	Throttler throttling.Throttler
	Steps     []Step
}

func (r *StepsEngine) Run(ctx context.Context) error {
	var end func()
	var runLogger *instrumentation.SpanLogger
	if r.Object != nil {
		keysAndValues := []any{"object_name", r.Object.GetName()}
		if r.Object.GetNamespace() != "" {
			keysAndValues = append(keysAndValues, "object_namespace", r.Object.GetNamespace())
		}

		ctx, runLogger, end = instrumentation.GetLogSpan(ctx, "ReconciliationSteps", keysAndValues...)
		defer end()
	} else {
		ctx, runLogger, end = instrumentation.GetLogSpan(ctx, "ReconciliationSteps")
		defer end()
	}

	var stepEnd func()
STEPS:
	for _, step := range r.Steps {
		// setValues does not seem to affect span.
		// TODO: Fix it! but, first need to move to standalone observability lib and fix there if broken
		runLogger.SetValues("last_step", step.GetName())
		if stepEnd != nil {
			stepEnd()
			stepEnd = nil
		}

		if step.ShouldSkip(r.Object) {
			continue STEPS
		}

		// Check preconditions
		for _, predicate := range step.GetPredicates() {
			if !predicate() {
				if step.ShouldAbortOnFalsePredicates() {
					err := fmt.Errorf("aborted: predicate %v is false for step %s", predicate, step.GetName())
					stopErr := &AbortedByPredicate{err}
					runLogger.SetValues("stop_err", stopErr.Error())
					return stopErr
				}

				continue STEPS
			}
		}

		// Throttling handling
		if step.IsThrottled() && r.Object != nil && r.Throttler != nil {
			if !r.Throttler.ShouldRun(step.GetName(), step.GetThrottlingSettings()) {
				continue STEPS
			}
		}

		stepCtx, stepLogger, spanEnd := instrumentation.GetLogSpan(ctx, step.GetName())
		stepEnd = spanEnd
		defer spanEnd() // in case we dont handle it will in terms of closing in for loop

		if err := step.RunStep(stepCtx); err != nil {
			// if the error is not expected error, we should stop the reconciliation,
			// otherwise - continue to the next step
			var expectedError *ExpectedError
			if errors.As(err, &expectedError) {
				stepLogger.Error(err, "Expected error running step")
				stepEnd()

				continue STEPS
			}

			if step.HasCondition() && r.Object != nil {
				condition := step.GetCondition()
				condition.Reason = "Error"
				condition.Message = err.Error()

				setCondError := r.Object.SetConditionFalse(stepCtx, condition)
				if setCondError != nil {
					stepLogger.Debug("error setting reconcile error on object", "step", step.GetName(), "error", setCondError)
					stepLogger.SetError(err, "Error running step")
					stepEnd()
					return setCondError
				}
			}
			if step.GetFailureCallback() != nil {
				onFail := step.GetFailureCallback()
				if fErr := onFail(stepCtx, step.GetName(), err); fErr != nil {
					stepLogger.Error(fErr, "Error running onFail step")
				}
			}
			stepLogger.SetError(err, "Error running step")
			stepLogger.SetValues("stop_err", err.Error())
			runLogger.SetError(err, "Error running step "+step.GetName())
			runLogger.SetValues("stop_err", err.Error())
			stepEnd()
			return &StepRunError{Err: err, Subject: r.Object, Step: step}
		}

		// Update condition in case of success
		if step.HasCondition() && r.Object != nil {
			condition := step.GetCondition()

			if condition.Reason == "" {
				condition.Reason = "Success"
			}
			if condition.Message == "" {
				condition.Message = "Completed successfully"
			}

			err := r.Object.SetConditionTrue(stepCtx, condition)

			if err != nil {
				stepEnd()
				stopErr := &StepRunError{Err: err, Subject: r.Object, Step: step}
				runLogger.SetValues("stop_err", stopErr.Error())
				runLogger.SetError(err, "Error running step "+step.GetName())
				stepLogger.SetError(err, "Error setting condition")
				return stopErr
			}
		}

		stepLogger.SetStatus(codes.Ok, "Step completed successfully")
		if step.ShouldFinishOnSuccess() {
			stepEnd()
			return nil
		}

		if step.IsThrottled() && step.GetThrottlingSettings().EnsureStepSuccess {
			r.Throttler.SetNow(step.GetName())
		}
	}
	return nil
}

func ForceNoError(f StepFunc) StepFunc {
	return func(ctx context.Context) error {
		_, logger, end := instrumentation.GetLogSpan(ctx, "ForceNoError")
		defer end()
		ret := f(ctx)
		if ret != nil {
			logger.SetError(ret, "transient error in step, but forcing no error in flow")
		}
		return nil
	}
}
