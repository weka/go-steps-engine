// Kubernetes Controller Example
//
// This example demonstrates how to use the go-steps-engine in a Kubernetes controller
// with real K8s objects and conditions. It shows how to manage state persistence
// using Kubernetes conditions and handle reconciliation loops.
package main

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/weka/go-steps-engine/examples/shared"
	"github.com/weka/go-steps-engine/lifecycle"
)

// CustomResource represents a custom Kubernetes resource with status conditions
type CustomResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CustomResourceSpec   `json:"spec,omitempty"`
	Status CustomResourceStatus `json:"status,omitempty"`
}

type CustomResourceSpec struct {
	Replicas int    `json:"replicas"`
	Image    string `json:"image"`
}

type CustomResourceStatus struct {
	Conditions    []metav1.Condition `json:"conditions,omitempty"`
	ReadyReplicas int                `json:"readyReplicas"`
}

// DeepCopyObject is required for runtime.Object interface
func (c *CustomResource) DeepCopyObject() runtime.Object {
	return &CustomResource{
		TypeMeta:   c.TypeMeta,
		ObjectMeta: *c.ObjectMeta.DeepCopy(),
		Spec:       c.Spec,
		Status:     c.Status,
	}
}

// CustomResourceReconciler reconciles CustomResource objects
type CustomResourceReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// Reconcile handles reconciliation of CustomResource objects using the steps engine
func (r *CustomResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the CustomResource instance
	customResource := &CustomResource{}
	if err := r.Client.Get(ctx, req.NamespacedName, customResource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Create StateKeeper for this object
	stateKeeper := &lifecycle.K8sObject{
		Client:     r.Client,
		Object:     customResource,
		Conditions: &customResource.Status.Conditions,
	}

	// Define reconciliation steps
	steps := []lifecycle.Step{
		&lifecycle.SingleStep{
			Name: "validate-spec",
			State: &lifecycle.State{
				Name:    "SpecValid",
				Reason:  "ValidationComplete",
				Message: "Spec validation completed successfully",
			},
			Run: func(ctx context.Context) error {
				logger.Info("Validating CustomResource spec")

				// Validate required fields
				if customResource.Spec.Image == "" {
					return fmt.Errorf("spec.image is required")
				}
				if customResource.Spec.Replicas <= 0 {
					return fmt.Errorf("spec.replicas must be greater than 0")
				}

				logger.Info("Spec validation passed", "image", customResource.Spec.Image, "replicas", customResource.Spec.Replicas)
				return nil
			},
		},
		&lifecycle.SingleStep{
			Name: "create-deployment",
			State: &lifecycle.State{
				Name:    "DeploymentReady",
				Reason:  "DeploymentCreated",
				Message: "Deployment created and ready",
			},
			Run: func(ctx context.Context) error {
				logger.Info("Creating/updating deployment")

				// Simulate deployment creation
				time.Sleep(100 * time.Millisecond)

				// In a real controller, you would:
				// 1. Create or update a Deployment object
				// 2. Wait for deployment to be ready
				// 3. Update status with ready replicas

				customResource.Status.ReadyReplicas = customResource.Spec.Replicas
				logger.Info("Deployment is ready", "readyReplicas", customResource.Status.ReadyReplicas)
				return nil
			},
		},
		&lifecycle.SingleStep{
			Name: "create-service",
			State: &lifecycle.State{
				Name:    "ServiceReady",
				Reason:  "ServiceCreated",
				Message: "Service created and ready",
			},
			Run: func(ctx context.Context) error {
				logger.Info("Creating/updating service")

				// Simulate service creation
				time.Sleep(50 * time.Millisecond)

				// In a real controller, you would create or update a Service object
				logger.Info("Service is ready")
				return nil
			},
		},
		&lifecycle.SingleStep{
			Name: "finalize-status",
			State: &lifecycle.State{
				Name:    "Ready",
				Reason:  "AllComponentsReady",
				Message: "All components are ready and healthy",
			},
			Run: func(ctx context.Context) error {
				logger.Info("Finalizing status")

				// Update overall status
				customResource.Status.ReadyReplicas = customResource.Spec.Replicas

				logger.Info("CustomResource is fully ready")
				return nil
			},
		},
	}

	// Create and run the steps engine
	engine := &lifecycle.StepsEngine{
		StateKeeper: stateKeeper,
		Steps:       steps,
	}

	// Run the reconciliation steps and return appropriate controller result
	return engine.RunAsReconcilerResponse(ctx)
}

func main() {
	// Setup logging and OpenTelemetry
	ctx := context.Background()
	_, shutdown := shared.SetupLogging(ctx, "weka-k8s-controller-example")
	defer func() {
		if err := shutdown(ctx); err != nil {
			fmt.Printf("Failed to shutdown OTel SDK: %v\n", err)
		}
	}()

	// Create a fake Kubernetes client for demonstration
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	// Create a sample CustomResource
	customResource := &CustomResource{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResource",
			APIVersion: "example.com/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample-resource",
			Namespace: "default",
		},
		Spec: CustomResourceSpec{
			Replicas: 3,
			Image:    "nginx:1.20",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(customResource).
		Build()

	// Create the reconciler
	reconciler := &CustomResourceReconciler{
		Client: client,
		Scheme: scheme,
	}

	// Set up context and logger

	// Create reconcile request
	req := ctrl.Request{
		NamespacedName: ctrl.Request{}.NamespacedName,
	}
	req.NamespacedName.Name = customResource.Name
	req.NamespacedName.Namespace = customResource.Namespace

	fmt.Println("Starting Kubernetes controller reconciliation example...")
	fmt.Println("========================================================")

	// Run reconciliation
	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		fmt.Printf("Reconciliation failed: %v\n", err)
		return
	}

	fmt.Printf("Reconciliation completed. Result: %+v\n", result)

	// Fetch updated object to show conditions
	updatedResource := &CustomResource{}
	if err := client.Get(ctx, req.NamespacedName, updatedResource); err != nil {
		fmt.Printf("Failed to fetch updated resource: %v\n", err)
		return
	}

	fmt.Println("\nFinal conditions:")
	for _, condition := range updatedResource.Status.Conditions {
		fmt.Printf("  %s: %s (%s) - %s\n",
			condition.Type,
			condition.Status,
			condition.Reason,
			condition.Message)
	}

	fmt.Printf("\nStatus: ReadyReplicas = %d\n", updatedResource.Status.ReadyReplicas)
}
