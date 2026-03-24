package lifecycle

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/weka/go-weka-observability/instrumentation"
	"go.opentelemetry.io/otel/codes"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/weka/go-steps-engine/throttling"
)

type StepFunc func(ctx context.Context) error

// StepStatus represents the possible states of a step during execution.
// Steps follow a typical lifecycle: pending -> running -> (succeeded|failed)
type StepStatus string

const (
	// StepStatusPending indicates the step has not started execution yet.
	// This is the initial state for all steps.
	StepStatusPending StepStatus = "pending"

	// StepStatusRunning indicates the step is currently being executed.
	// Not all StateKeeper implementations support persisting this state.
	StepStatusRunning StepStatus = "running"

	// StepStatusSucceeded indicates the step completed successfully.
	// This is a terminal state.
	StepStatusSucceeded StepStatus = "succeeded"

	// StepStatusFailed indicates the step failed during execution.
	// This is a terminal state, though steps can be retried from this state.
	StepStatusFailed StepStatus = "failed"
)

// StepState represents the complete state information for a step.
// It contains both the current status and additional context about why
// the step is in that state, which is useful for debugging and monitoring.
type StepState struct {
	// Name is the unique identifier for this step within its execution context
	Name string

	// Status indicates the current lifecycle state of the step
	Status StepStatus

	// Reason provides a brief, machine-readable explanation for the current status.
	// Common values: "Success", "Error", "Timeout", "Cancelled"
	Reason string

	// Message provides a human-readable description of the current status,
	// often including error details when Status is StepStatusFailed
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
	// Append a predicate to the step's predicate list
	AppendPredicate(predicate PredicateFunc)
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
	// Get nested steps (returns nil if HasNestedSteps() is false)
	GetNestedSteps() []Step
	// Should the step be skipped if the state is already true
	ShouldSkip(ctx context.Context, stateKeeper StateKeeper) bool
	// Does the step have a state to track
	HasState() bool
	// Get step state name
	GetStepStateName() string
	// Get the desired step state in case if step succeeded
	GetSucceededState() *StepState
	// Set the stateKeeper and throttler for the step
	SetStateKeeperAndThrottler(stateKeeper StateKeeper, throttler throttling.Throttler)
	// Set the state configuration for the step
	SetState(state *State)
}

// StateKeeper manages the persistent state of steps during execution.
// It provides an abstraction over different storage backends (K8s conditions, databases, etc.)
// allowing the same step execution logic to work with different state persistence mechanisms.
//
// Implementations should be thread-safe and handle concurrent access to the same step states.
// State transitions should be atomic to prevent race conditions in multi-threaded environments.
type StateKeeper interface {
	// GetSummaryAttributes returns key-value pairs that provide contextual information
	// about this state keeper instance, useful for logging and debugging.
	// Common attributes include object identifiers, namespaces, or execution contexts.
	//
	// Example return values:
	//   K8s: {"object_name": "my-pod", "namespace": "default", "kind": "Pod"}
	//   FSDB: {"execution_id": "exec-123", "flow_name": "cluster-ready"}
	GetSummaryAttributes() map[string]string

	// GetSummary returns a human-readable string representation of this state keeper,
	// typically used in error messages and logs. Should be concise but descriptive.
	//
	// Example return values:
	//   "Pod:default:my-pod"
	//   "TestExecution:exec-123:cluster-ready"
	GetSummary() string

	// GetStepState retrieves the current state of a step by name.
	// Returns a StepState with Status=StepStatusPending if the step has not been
	// executed yet or if no state is found.
	//
	// Parameters:
	//   ctx: Context for cancellation and timeouts
	//   stepName: Unique identifier for the step within this execution context
	//
	// Returns:
	//   - StepState with current status, reason, and message
	//   - Error if state retrieval fails (not if step doesn't exist)
	//
	// Thread-safety: Must be safe for concurrent calls
	GetStepState(ctx context.Context, stepName string) (*StepState, error)

	// SetStepState persists the state of a step.
	// The implementation should handle state transitions atomically and may
	// include additional logic like setting timestamps or validating transitions.
	//
	// Parameters:
	//   ctx: Context for cancellation and timeouts
	//   state: Complete state information to persist
	//
	// Returns:
	//   - Error if state persistence fails
	//
	// Thread-safety: Must be safe for concurrent calls
	// Idempotency: Setting the same state multiple times should not cause errors
	SetStepState(ctx context.Context, state *StepState) error

	// SupportsRunningState indicates whether this StateKeeper implementation
	// can meaningfully track and persist StepStatusRunning state.
	//
	// Some backends (like K8s conditions) don't have a natural "running" state
	// and may skip persisting running transitions to avoid unnecessary updates.
	//
	// Returns:
	//   - true if running states should be persisted
	//   - false if running state transitions should be skipped
	SupportsRunningState() bool
}

type stepEngineLogger interface {
	SetValues(keysAndValues ...any)
	SetError(err error, msg string, keysAndValues ...any)
}

type StepsEngine struct {
	StateKeeper StateKeeper
	Throttler   throttling.Throttler
	Steps       []Step
	// Optional context enhancer function (per step)
	WithStepContext func(ctx context.Context, stepName string) context.Context
}

func (r *StepsEngine) Run(ctx context.Context) error {
	var runLogger stepEngineLogger
	if r.StateKeeper != nil {
		var keysAndValues []any
		for k, v := range r.StateKeeper.GetSummaryAttributes() {
			keysAndValues = append(keysAndValues, k, v)
		}

		var spanLogger *instrumentation.SpanLogger
		ctx, spanLogger = instrumentation.CreateLogSpan(ctx, "ReconciliationSteps", keysAndValues...)
		defer spanLogger.End()
		runLogger = spanLogger
	} else {
		runLogger = instrumentation.CurrentSpanLogger(ctx)
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

		stepCtx := ctx
		if r.WithStepContext != nil {
			stepCtx = r.WithStepContext(stepCtx, step.GetName())
		}

		stepCtx, stepLogger := instrumentation.CreateLogSpan(stepCtx, step.GetName())
		stepEnd = stepLogger.End
		defer stepLogger.End() // in case we dont handle it will in terms of closing in for loop

		// Mark the step as running (if it has state and the StateKeeper supports running states)
		if step.HasState() && r.StateKeeper != nil && r.StateKeeper.SupportsRunningState() {
			state := StepState{
				Name:   step.GetStepStateName(),
				Status: StepStatusRunning,
			}
			err := r.StateKeeper.SetStepState(stepCtx, &state)
			if err != nil {
				stepLogger.SetError(err, "Error setting step state to running")
				stepLogger.SetValues("stop_err", err.Error())
				stepEnd()

				return fmt.Errorf("error setting step %s state to running: %w", step.GetStepStateName(), err)
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
					Name:    step.GetStepStateName(),
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
			state, _ := r.StateKeeper.GetStepState(stepCtx, step.GetStepStateName())

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

func (r *StepsEngine) RunAsReconcilerResponse(ctx context.Context) (ctrl.Result, error) {
	logger := instrumentation.CurrentSpanLogger(ctx)

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
		logger.SetError(err, "Error processing reconciliation steps")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	logger.Info("Reconciliation steps completed successfully")
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil // Never fully abort
}
