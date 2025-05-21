package lifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/weka/go-weka-observability/instrumentation"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sObject is an implementation of ObjectWithConditions for Kubernetes objects
type K8sObject struct {
	Client     client.Client
	Object     client.Object
	Conditions *[]metav1.Condition
}

func (o *K8sObject) GetName() string {
	return o.Object.GetName()
}

func (o *K8sObject) GetNamespace() string {
	return o.Object.GetNamespace()
}

func (o *K8sObject) IsConditionTrue(conditionType string) bool {
	return meta.IsStatusConditionTrue(*o.Conditions, conditionType)
}

func (o *K8sObject) SetConditionTrue(ctx context.Context, condition Condition) error {
	return o.setConditions(ctx, metav1.Condition{
		Type:    condition.Name,
		Status:  metav1.ConditionTrue,
		Reason:  condition.Reason,
		Message: condition.Message,
	})
}

func (o *K8sObject) SetConditionFalse(ctx context.Context, condition Condition) error {
	return o.setConditions(ctx, metav1.Condition{
		Type:    condition.Name,
		Status:  metav1.ConditionFalse,
		Reason:  condition.Reason,
		Message: condition.Message,
	})
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
	kind := ""
	// cast to metav1.Type
	t, ok := o.Object.(metav1.Type)
	if ok {
		kind = t.GetKind()
	}

	return fmt.Sprintf("%s:%s", o.Object.GetName(), kind)
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
