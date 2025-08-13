package lifecycle

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/weka/go-weka-observability/instrumentation"
)

// K8sObject implements StateKeeper for Kubernetes objects using metav1.Condition.
// It maps step states to Kubernetes condition states:
//   - StepStatusSucceeded -> ConditionTrue
//   - StepStatusFailed -> ConditionFalse
//   - StepStatusPending -> ConditionUnknown (or condition not present)
//   - StepStatusRunning -> Skipped (K8s conditions don't have a natural running state)
//
// The K8sObject automatically updates the Kubernetes object's status conditions
// when SetStepState is called, providing persistent state tracking that survives
// pod restarts and controller reconciliation cycles.
type K8sObject struct {
	// Client is the Kubernetes client used to update object status
	Client client.Client
	// Object is the Kubernetes object whose conditions will be managed
	Object client.Object
	// Conditions points to the slice of conditions in the object's status
	Conditions *[]metav1.Condition
}

// GetSummaryAttributes returns metadata about this Kubernetes object for logging
func (o *K8sObject) GetSummaryAttributes() map[string]string {
	return map[string]string{
		"object_name":      o.Object.GetName(),
		"object_namespace": o.Object.GetNamespace(),
		"kind":             o.Object.GetObjectKind().GroupVersionKind().Kind,
	}
}

// GetStepState retrieves the current state of a step by mapping from Kubernetes conditions.
// If no condition exists for the given step name, returns a pending state.
func (o *K8sObject) GetStepState(ctx context.Context, conditionType string) (*StepState, error) {
	pendingState := StepState{
		Name:   conditionType,
		Status: StepStatusPending,
	}
	if o.Conditions == nil {
		// If conditions are not set, return pending state
		return &pendingState, nil
	}

	condition := meta.FindStatusCondition(*o.Conditions, conditionType)
	if condition == nil {
		// If condition is not found, return pending state
		return &pendingState, nil
	}

	stepState := k8sConditionToStepState(*condition)
	return &stepState, nil
}

// SetStepState persists a step state by updating the corresponding Kubernetes condition.
// Running states are skipped since K8s conditions don't have a natural running state.
func (o *K8sObject) SetStepState(ctx context.Context, state *StepState) error {
	condition := stepStateToK8sCondition(*state)
	// Skip setting condition "unknown" and running states
	if condition.Status == metav1.ConditionUnknown || state.Status == StepStatusRunning {
		return nil
	}

	return o.setConditions(ctx, condition)
}

// SupportsRunningState returns false because Kubernetes conditions don't have a natural
// "running" state - they are typically either True, False, or Unknown.
func (o *K8sObject) SupportsRunningState() bool {
	return false
}

func (o *K8sObject) setConditions(ctx context.Context, condition metav1.Condition) error {
	ctx, _, end := instrumentation.GetLogSpan(ctx, "setConditions", "condition", condition.Type)
	defer end()

	meta.SetStatusCondition(o.Conditions, condition)
	if err := o.Client.Status().Update(ctx, o.Object); err != nil {
		return &ConditionUpdateError{Err: err, Subject: o.Object, Condition: condition}
	}
	return nil
}

func (o *K8sObject) GetSummary() string {
	kind := o.Object.GetObjectKind().GroupVersionKind().Kind
	name := o.Object.GetName()
	namespace := o.Object.GetNamespace()

	return fmt.Sprintf("%s:%s:%s", kind, namespace, name)
}

func k8sConditionToStepState(condition metav1.Condition) StepState {
	s := StepState{
		Name:    condition.Type,
		Reason:  condition.Reason,
		Message: condition.Message,
		Status:  StepStatusPending,
	}

	if condition.Status == metav1.ConditionTrue {
		s.Status = StepStatusSucceeded
	} else if s.Reason == "Error" {
		s.Status = StepStatusFailed
	} else if s.Reason == "InProgress" {
		s.Status = StepStatusRunning
	}

	return s
}

func stepStateToK8sCondition(state StepState) metav1.Condition {
	condition := metav1.Condition{
		Type:    state.Name,
		Reason:  state.Reason,
		Message: state.Message,
	}

	if state.Status == StepStatusSucceeded {
		condition.Status = metav1.ConditionTrue
	} else if state.Status == StepStatusFailed {
		condition.Status = metav1.ConditionFalse
	} else {
		condition.Status = metav1.ConditionUnknown
	}

	return condition
}

func RunAsReconcilerResponse(ctx context.Context, stepsEngine *StepsEngine) (ctrl.Result, error) {
	ctx, logger, end := instrumentation.GetLogSpan(ctx, "")
	defer end()

	err := stepsEngine.Run(ctx)
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
