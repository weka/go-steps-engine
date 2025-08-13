package lifecycle

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/codes"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/weka/go-weka-observability/instrumentation"

	"github.com/weka/go-steps-engine/throttling"
)

type StepFunc func(ctx context.Context) error

// StepStatus represents the possible states of a step
type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusSucceeded StepStatus = "succeeded"
	StepStatusFailed    StepStatus = "failed"
)

type StepState struct {
	Name    string
	Status  StepStatus
	Reason  string
	Message string
}

func (s *StepState) StatusEqual(other StepStatus) bool {
	return s.Status == other
}

type Step interface {
	// Run the underlying function of the step
	RunStep(ctx context.Context) error
	// Should the step become the last one in the flow if it succeeds
	ShouldFinishOnSuccess() bool
	// Get predicates that must be true for the step to be executed
	GetPredicates() []PredicateFunc
	// Should the whole flow be aborted if one of the predicates is false
	ShouldAbortOnFalsePredicates() bool
	// Should the whole flow proceed if the step fails
	ShouldContinueOnError() bool
	// Get the function that is run if the step fails
	GetFailureCallback() func(context.Context, string, error) error
	// Is step throttled
	IsThrottled() bool
	// Get throttling settings
	GetThrottlingSettings() *throttling.ThrottlingSettings
	// Get step name
	GetName() string
	// If the step has nested steps, return true
	HasNestedSteps() bool
	// Should the step be skipped if the state is already true
	ShouldSkip(ctx context.Context, stateKeeper StateKeeper) bool
	// Does the step have a state to track
	HasState() bool
	// Get the desired step state in case if step succeeded
	GetSucceededState() *StepState
	// Set the stateKeeper and throttler for the step
	SetStateKeeperAndThrottler(stateKeeper StateKeeper, throttler throttling.Throttler)
}

type StateKeeper interface {
	// get state attributes that can be useful for logging
	GetSummaryAttrubutes() map[string]string
	// get information about the state keeper in one string (for error messages or logs)
	GetSummary() string
	// get step state by name
	GetStepState(ctx context.Context, stepName string) (*StepState, error)
	// set step state
	SetStepState(ctx context.Context, state *StepState) error
}

type StepsEngine struct {
	StateKeeper StateKeeper
	Throttler   throttling.Throttler
	Steps       []Step
}

func (r *StepsEngine) Run(ctx context.Context) error {
	var end func()
	var runLogger *instrumentation.SpanLogger
	if r.StateKeeper != nil {
		var keysAndValues []any
		for k, v := range r.StateKeeper.GetSummaryAttrubutes() {
			keysAndValues = append(keysAndValues, k, v)
		}

		ctx, runLogger, end = instrumentation.GetLogSpan(ctx, "ReconciliationSteps", keysAndValues...)
		defer end()
	} else {
		ctx, runLogger, end = instrumentation.GetLogSpan(ctx, "")
		defer end()
	}

	var stepEnd func()
STEPS:
	for _, step := range r.Steps {
		if step.HasNestedSteps() {
			step.SetStateKeeperAndThrottler(r.StateKeeper, r.Throttler)
		}

		// setValues does not seem to affect span.
		// TODO: Fix it! but, first need to move to standalone observability lib and fix there if broken
		runLogger.SetValues("last_step", step.GetName())
		if stepEnd != nil {
			stepEnd()
			stepEnd = nil
		}

		if step.ShouldSkip(ctx, r.StateKeeper) {
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
		if step.IsThrottled() && r.Throttler != nil {
			throttlingSettings := step.GetThrottlingSettings()

			key := step.GetName()
			if throttlingSettings.PartitionKeyOverride != nil {
				key = *throttlingSettings.PartitionKeyOverride
			}

			if !r.Throttler.ShouldRun(key, throttlingSettings) {
				continue STEPS
			}
		}

		stepCtx, stepLogger, spanEnd := instrumentation.GetLogSpan(ctx, step.GetName())
		stepEnd = spanEnd
		defer spanEnd() // in case we dont handle it will in terms of closing in for loop

		// mark the step as running (if it has state)
		if step.HasState() && r.StateKeeper != nil {
			state := StepState{
				Name:   step.GetName(),
				Status: StepStatusRunning,
			}
			err := r.StateKeeper.SetStepState(stepCtx, &state)
			if err != nil {
				stepLogger.SetError(err, "Error setting step state to running")
				stepLogger.SetValues("stop_err", err.Error())
				stepEnd()

				return fmt.Errorf("error setting step %s state to running: %w", step.GetName(), err)
			}
		}

		if err := step.RunStep(stepCtx); err != nil {
			// if the error is not expected error, we should stop the reconciliation,
			// otherwise - continue to the next step
			var expectedError *ExpectedError
			if stderrors.As(err, &expectedError) {
				stepLogger.Error(err, "Expected error running step")
				stepEnd()

				continue STEPS
			}

			if step.HasState() && r.StateKeeper != nil {
				state := StepState{
					Name:    step.GetName(),
					Reason:  "Error",
					Message: err.Error(),
					Status:  StepStatusFailed,
				}

				setStateError := r.StateKeeper.SetStepState(stepCtx, &state)
				if setStateError != nil {
					stepLogger.Debug("error setting step failure state on object", "step", step.GetName(), "error", setStateError)
					stepLogger.SetError(err, "Error running step")
					stepEnd()
					return setStateError
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
			stepEnd()

			if step.ShouldContinueOnError() {
				// If the step is allowed to continue on error, we go to the next step
				continue STEPS
			}

			runLogger.SetValues("stop_err", err.Error())
			runLogger.SetError(err, "Error running step "+step.GetName())

			return &StepRunError{Err: err, Subject: r.StateKeeper, Step: step}
		}

		// Update state in case of success
		if step.HasState() && r.StateKeeper != nil {
			state, _ := r.StateKeeper.GetStepState(stepCtx, step.GetName())

			if state == nil || !state.StatusEqual(StepStatusSucceeded) {
				state = step.GetSucceededState()

				if state.Reason == "" {
					state.Reason = "Success"
				}
				if state.Message == "" {
					state.Message = "Completed successfully"
				}

				err := r.StateKeeper.SetStepState(stepCtx, state)
				if err != nil {
					stepEnd()
					stopErr := &StepRunError{Err: err, Subject: r.StateKeeper, Step: step}
					runLogger.SetValues("stop_err", stopErr.Error())
					runLogger.SetError(err, "Error running step "+step.GetName())
					stepLogger.SetError(err, "Error setting state")
					return stopErr
				}
			}
		}

		stepLogger.SetStatus(codes.Ok, "Step completed successfully")
		if step.ShouldFinishOnSuccess() {
			stepEnd()
			return nil
		}

		throttlingSettings := step.GetThrottlingSettings()
		if step.IsThrottled() && throttlingSettings.EnsureStepSuccess {
			key := step.GetName()
			if throttlingSettings.PartitionKeyOverride != nil {
				key = *throttlingSettings.PartitionKeyOverride
			}

			r.Throttler.SetNow(key)
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

func (r *StepsEngine) RunAsReconcilerResponse(ctx context.Context) (ctrl.Result, error) {
	ctx, logger, end := instrumentation.GetLogSpan(ctx, "")
	defer end()

	err := r.Run(ctx)
	if err != nil {
		// check if the error is WaitError or AbortError, then return without error, but with 3 seconds wait
		var lastUnpacked *StepRunError
		var unpackTarget error
		unpackTarget = err
		for {
			unpacked, ok := unpackTarget.(*StepRunError)
			if !ok {
				if lastUnpacked == nil {
					break
				}
				if waitError, ok := lastUnpacked.Err.(*WaitError); ok {
					logger.Info("waiting for conditions to be met", "error", err)
					sleepDuration := 3 * time.Second
					if waitError.Duration > 0 {
						sleepDuration = waitError.Duration
					}
					return ctrl.Result{RequeueAfter: sleepDuration}, nil
				}
				if _, ok := lastUnpacked.Err.(*AbortedByPredicate); ok {
					logger.Info("aborted by predicate", "error", err)
					return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
				}
				break
			} else {
				lastUnpacked = unpacked
				unpackTarget = unpacked.Err
			}
		}
		logger.Error(err, "Error processing reconciliation steps")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	logger.Info("Reconciliation steps completed successfully")
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil // Never fully abort
}
