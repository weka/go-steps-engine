// Parallel Processing Example
//
// This example demonstrates how to use ParallelSteps to execute multiple
// operations concurrently, which is useful for independent tasks that can
// run simultaneously to improve overall execution time.
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

// Simulate data processing tasks
func processDataBatch(batchID string, itemCount int) lifecycle.StepFunc {
	return func(ctx context.Context) error {
		fmt.Printf("[%s] Starting batch %s processing (%d items)\n",
			time.Now().Format("15:04:05.000"), batchID, itemCount)

		// Simulate variable processing time
		processingTime := time.Duration(rand.Intn(500)+200) * time.Millisecond
		time.Sleep(processingTime)

		fmt.Printf("[%s] Completed batch %s processing (%d items) in %v\n",
			time.Now().Format("15:04:05.000"), batchID, itemCount, processingTime)
		return nil
	}
}

// Simulate external API calls
func callExternalAPI(apiName string) lifecycle.StepFunc {
	return func(ctx context.Context) error {
		fmt.Printf("[%s] Calling %s API\n",
			time.Now().Format("15:04:05.000"), apiName)

		// Simulate API call time
		callTime := time.Duration(rand.Intn(300)+100) * time.Millisecond
		time.Sleep(callTime)

		fmt.Printf("[%s] %s API call completed in %v\n",
			time.Now().Format("15:04:05.000"), apiName, callTime)
		return nil
	}
}

func main() {
	ctx := context.Background()

	// Setup logging and OpenTelemetry
	ctx, shutdown := shared.SetupLogging(ctx, "weka-parallel-processing-example")
	defer func() {
		if err := shutdown(ctx); err != nil {
			log.Printf("Failed to shutdown OTel SDK: %v", err)
		}
	}()

	// Create StateKeeper
	stateKeeper := shared.NewInMemoryStateKeeper("parallel-processing")

	// Define the workflow with parallel processing
	steps := []lifecycle.Step{
		&lifecycle.SimpleStep{
			Name:  "initialize",
			State: &lifecycle.State{Name: "initialize"},
			Run: func(ctx context.Context) error {
				fmt.Printf("[%s] Initializing parallel processing system...\n",
					time.Now().Format("15:04:05.000"))
				time.Sleep(100 * time.Millisecond)
				fmt.Printf("[%s] System initialized successfully\n",
					time.Now().Format("15:04:05.000"))
				return nil
			},
		},

		// Parallel data processing batches
		&lifecycle.ParallelSteps{
			Name: "process-data-batches",
			Steps: []lifecycle.Step{
				&lifecycle.SimpleStep{
					Name: "process-batch-1",
					Run:  processDataBatch("batch-1", 1000),
				},
				&lifecycle.SimpleStep{
					Name: "process-batch-2",
					Run:  processDataBatch("batch-2", 1500),
				},
				&lifecycle.SimpleStep{
					Name: "process-batch-3",
					Run:  processDataBatch("batch-3", 800),
				},
				&lifecycle.SimpleStep{
					Name: "process-batch-4",
					Run:  processDataBatch("batch-4", 1200),
				},
			},
		},

		// Parallel API calls
		&lifecycle.ParallelSteps{
			Name: "external-integrations",
			Steps: []lifecycle.Step{
				&lifecycle.SimpleStep{
					Name: "user-service-api",
					Run:  callExternalAPI("UserService"),
				},
				&lifecycle.SimpleStep{
					Name: "notification-service-api",
					Run:  callExternalAPI("NotificationService"),
				},
				&lifecycle.SimpleStep{
					Name: "analytics-service-api",
					Run:  callExternalAPI("AnalyticsService"),
				},
			},
		},

		&lifecycle.SimpleStep{
			Name:  "aggregate-results",
			State: &lifecycle.State{Name: "aggregate-results"},
			Run: func(ctx context.Context) error {
				fmt.Printf("[%s] Aggregating results from all parallel operations...\n",
					time.Now().Format("15:04:05.000"))
				time.Sleep(200 * time.Millisecond)
				fmt.Printf("[%s] Results aggregated successfully\n",
					time.Now().Format("15:04:05.000"))
				return nil
			},
		},

		&lifecycle.SimpleStep{
			Name:  "finalize",
			State: &lifecycle.State{Name: "finalize"},
			Run: func(ctx context.Context) error {
				fmt.Printf("[%s] Finalizing parallel processing workflow...\n",
					time.Now().Format("15:04:05.000"))
				time.Sleep(50 * time.Millisecond)
				fmt.Printf("[%s] Workflow completed successfully\n",
					time.Now().Format("15:04:05.000"))
				return nil
			},
		},
	}

	// Create and run the workflow engine
	engine := &lifecycle.StepsEngine{
		StateKeeper: stateKeeper,
		Steps:       steps,
	}

	fmt.Println("Starting parallel processing workflow...")
	fmt.Println("=====================================")
	startTime := time.Now()

	if err := engine.Run(ctx); err != nil {
		log.Fatalf("Workflow failed: %v", err)
	}

	totalTime := time.Since(startTime)
	fmt.Println("=====================================")
	fmt.Printf("Workflow completed successfully in %v!\n", totalTime)

	// Display final states
	fmt.Println("\nFinal step states:")
	for stepName, state := range stateKeeper.GetAllStates() {
		fmt.Printf("  %-25s: %s\n", stepName, state.Status)
	}

	fmt.Printf("\nNote: Without parallel execution, this would have taken much longer!\n")
	fmt.Printf("Parallel execution saved significant time by running independent tasks concurrently.\n")
}
