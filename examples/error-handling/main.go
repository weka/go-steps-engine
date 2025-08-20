// Error Handling and Recovery Example
//
// This example demonstrates various error handling patterns in the go-steps-engine:
// - Expected errors that don't stop the workflow
// - Wait errors for retryable conditions
// - Step failure callbacks for cleanup
// - ContinueOnError for non-critical steps
// - Custom error types and recovery strategies
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/weka/go-steps-engine/examples/shared"
	"github.com/weka/go-steps-engine/lifecycle"
)

// Simulate an external service that might be temporarily unavailable
var serviceAttempts = 0

func callUnreliableService() error {
	serviceAttempts++
	fmt.Printf("  Attempting to call external service (attempt %d)\n", serviceAttempts)

	// Simulate service being unavailable for first few attempts
	if serviceAttempts < 3 {
		return fmt.Errorf("service temporarily unavailable")
	}

	fmt.Printf("  External service call succeeded!\n")
	return nil
}

// Simulate resource cleanup on failure
func cleanupResources(ctx context.Context, stepName string, err error) error {
	fmt.Printf("  🧹 Cleaning up resources due to failure in step '%s': %v\n", stepName, err)
	time.Sleep(100 * time.Millisecond) // Simulate cleanup time
	fmt.Printf("  🧹 Cleanup completed for step '%s'\n", stepName)
	return nil
}

func main() {
	ctx := context.Background()

	// Setup logging and OpenTelemetry
	_, shutdown := shared.SetupLogging(ctx, "weka-error-handling-example")
	defer func() {
		if err := shutdown(ctx); err != nil {
			log.Printf("Failed to shutdown OTel SDK: %v", err)
		}
	}()

	// Create StateKeeper
	stateKeeper := shared.NewInMemoryStateKeeper("error-handling")

	fmt.Println("Error Handling and Recovery Example")
	fmt.Println("==================================")

	// Define workflow with various error handling patterns
	steps := []lifecycle.Step{
		// Step 1: Basic successful step
		&lifecycle.SingleStep{
			Name:  "initialize",
			State: &lifecycle.State{Name: "initialize"},
			Run: func(ctx context.Context) error {
				fmt.Println("🚀 Initializing system...")
				time.Sleep(100 * time.Millisecond)
				return nil
			},
		},

		// Step 2: Expected error that doesn't stop the workflow
		&lifecycle.SingleStep{
			Name:  "check-optional-feature",
			State: &lifecycle.State{Name: "check-optional-feature"},
			Run: func(ctx context.Context) error {
				fmt.Println("🔍 Checking optional feature availability...")

				// Simulate optional feature not being available
				if rand.Float32() < 0.7 { // 70% chance feature is not available
					fmt.Println("  ⚠️  Optional feature not available - this is expected")
					// Return an ExpectedError - this won't stop the workflow
					return lifecycle.NewExpectedError(fmt.Errorf("optional feature not available"))
				}

				fmt.Println("  ✨ Optional feature is available")
				return nil
			},
		},

		// Step 3: Wait error for retryable conditions
		&lifecycle.SingleStep{
			Name:  "wait-for-service",
			State: &lifecycle.State{Name: "wait-for-service"},
			Run: func(ctx context.Context) error {
				fmt.Println("⏳ Waiting for external service to be ready...")

				if err := callUnreliableService(); err != nil {
					fmt.Printf("  ⚠️  Service not ready: %v\n", err)
					// Return a WaitError - this will cause the reconciler to retry
					return lifecycle.NewWaitErrorWithDuration(err, 2*time.Second)
				}

				return nil
			},
		},

		// Step 4: Step that might fail but has cleanup logic
		&lifecycle.SingleStep{
			Name:   "risky-operation",
			State:  &lifecycle.State{Name: "risky-operation"},
			OnFail: cleanupResources,
			Run: func(ctx context.Context) error {
				fmt.Println("⚡ Performing risky operation...")
				time.Sleep(200 * time.Millisecond)

				// Simulate occasional failures
				if rand.Float32() < 0.3 { // 30% chance of failure
					return fmt.Errorf("risky operation failed unexpectedly")
				}

				fmt.Println("  ✅ Risky operation succeeded")
				return nil
			},
		},

		// Step 5: Non-critical step that continues on error
		&lifecycle.SingleStep{
			Name:            "send-notification",
			State:           &lifecycle.State{Name: "send-notification"},
			ContinueOnError: true, // Don't stop the workflow if this fails
			OnFail:          cleanupResources,
			Run: func(ctx context.Context) error {
				fmt.Println("📧 Sending notification...")

				// Simulate notification service being down
				if rand.Float32() < 0.5 { // 50% chance of failure
					return fmt.Errorf("notification service is down")
				}

				fmt.Println("  ✅ Notification sent successfully")
				return nil
			},
		},

		// Step 6: Final validation step
		&lifecycle.SingleStep{
			Name:  "validate-completion",
			State: &lifecycle.State{Name: "validate-completion"},
			Run: func(ctx context.Context) error {
				fmt.Println("🔍 Validating workflow completion...")
				time.Sleep(100 * time.Millisecond)

				// Check if critical steps succeeded
				serviceState, _ := stateKeeper.GetStepState(ctx, "ServiceReady")
				riskyState, _ := stateKeeper.GetStepState(ctx, "RiskyOperationComplete")

				if serviceState.Status != lifecycle.StepStatusSucceeded {
					return fmt.Errorf("critical service is not ready")
				}

				if riskyState.Status != lifecycle.StepStatusSucceeded {
					return fmt.Errorf("risky operation did not complete successfully")
				}

				fmt.Println("  ✅ All critical operations completed successfully")
				return nil
			},
		},

		// Step 7: Finalization
		&lifecycle.SingleStep{
			Name:  "finalize",
			State: &lifecycle.State{Name: "finalize"},
			Run: func(ctx context.Context) error {
				fmt.Println("🎉 Finalizing workflow...")
				time.Sleep(50 * time.Millisecond)
				return nil
			},
		},
	}

	// Create and run the workflow engine
	engine := &lifecycle.StepsEngine{
		StateKeeper: stateKeeper,
		Steps:       steps,
	}

	fmt.Println("\nStarting error-resilient workflow...")
	fmt.Println("------------------------------------")

	// Run the workflow - it will handle errors gracefully
	if err := engine.Run(ctx); err != nil {
		fmt.Printf("\n❌ Workflow failed with error: %v\n", err)

		// In a real application, you might want to:
		// 1. Log the error details
		// 2. Send alerts
		// 3. Perform additional cleanup
		// 4. Retry the entire workflow later

		log.Printf("Workflow execution failed: %v", err)
	} else {
		fmt.Println("\n✅ Workflow completed successfully!")
	}

	// Display final states
	stateKeeper.PrintFinalStates()

	fmt.Println("\nKey Error Handling Patterns Demonstrated:")
	fmt.Println("- Expected errors for optional features")
	fmt.Println("- Wait errors for retryable conditions")
	fmt.Println("- Cleanup callbacks on step failures")
	fmt.Println("- ContinueOnError for non-critical steps")
	fmt.Println("- Final validation of critical components")
}
