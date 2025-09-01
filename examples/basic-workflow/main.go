// Basic Workflow Example
//
// This example demonstrates a simple sequential workflow using the go-steps-engine
// with an in-memory StateKeeper. It shows basic step execution, error handling,
// and state tracking.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/weka/go-steps-engine/examples/shared"
	"github.com/weka/go-steps-engine/lifecycle"
)

func main() {
	ctx := context.Background()

	// Setup logging and OpenTelemetry
	_, shutdown := shared.SetupLogging(ctx, "weka-basic-workflow-example")
	defer func() {
		if err := shutdown(ctx); err != nil {
			log.Printf("Failed to shutdown OTel SDK: %v", err)
		}
	}()

	// Create StateKeeper
	stateKeeper := shared.NewInMemoryStateKeeper("basic-workflow")

	// Define workflow steps
	steps := []lifecycle.Step{
		&lifecycle.SimpleStep{
			Name:  "initialize",
			State: &lifecycle.State{Name: "initialize"},
			Run: func(ctx context.Context) error {
				fmt.Println("Initializing application...")
				time.Sleep(100 * time.Millisecond) // Simulate work
				fmt.Println("Application initialized successfully")
				return nil
			},
		},
		&lifecycle.SimpleStep{
			Name:  "validate-config",
			State: &lifecycle.State{Name: "validate-config"},
			Run: func(ctx context.Context) error {
				fmt.Println("Validating configuration...")
				time.Sleep(50 * time.Millisecond) // Simulate work
				fmt.Println("Configuration validated successfully")
				return nil
			},
		},
		&lifecycle.SimpleStep{
			Name:  "setup-resources",
			State: &lifecycle.State{Name: "setup-resources"},
			Run: func(ctx context.Context) error {
				fmt.Println("Setting up resources...")
				time.Sleep(200 * time.Millisecond) // Simulate work
				fmt.Println("Resources setup completed")
				return nil
			},
		},
		&lifecycle.SimpleStep{
			Name:  "start-services",
			State: &lifecycle.State{Name: "start-services"},
			Run: func(ctx context.Context) error {
				fmt.Println("Starting services...")
				time.Sleep(150 * time.Millisecond) // Simulate work
				fmt.Println("All services started successfully")
				return nil
			},
		},
	}

	// Create and run the workflow engine
	engine := &lifecycle.StepsEngine{
		StateKeeper: stateKeeper,
		Steps:       steps,
	}

	fmt.Println("Starting basic workflow...")
	fmt.Println("==========================")

	if err := engine.Run(ctx); err != nil {
		log.Fatalf("Workflow failed: %v", err)
	}

	fmt.Println("==========================")
	fmt.Println("Workflow completed successfully!")

	// Display final states
	stateKeeper.PrintFinalStates()
}
