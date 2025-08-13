# Go Steps Engine

A flexible, state-aware step execution engine for Go applications. The engine provides a framework for executing sequences of operations with persistent state tracking, error handling, and support for different execution patterns.

## Features

- **State Persistence**: Pluggable state backends (Kubernetes conditions, databases, etc.)
- **Flexible Execution**: Support for sequential, parallel, and grouped step execution
- **Error Recovery**: Built-in retry logic, graceful error handling, and step continuation
- **Throttling**: Built-in rate limiting and throttling capabilities
- **Observability**: OpenTelemetry integration for tracing and logging
- **Kubernetes Native**: First-class support for Kubernetes controller patterns

## Quick Start

```go
package main

import (
    "context"
    
    "github.com/weka/go-steps-engine/lifecycle"
)

func main() {
    ctx := context.Background()
    
    // Create steps
    steps := []lifecycle.Step{
        &lifecycle.SingleStep{
            Name: "initialize",
            Run: func(ctx context.Context) error {
                // Your initialization logic here
                return nil
            },
        },
        &lifecycle.SingleStep{
            Name: "process",
            Run: func(ctx context.Context) error {
                // Your processing logic here
                return nil
            },
        },
    }
    
    // Create and run engine
    engine := &lifecycle.StepsEngine{
        Steps: steps,
    }
    
    if err := engine.Run(ctx); err != nil {
        panic(err)
    }
}
```

## Architecture

### Core Components

#### StepsEngine
The main execution engine that runs a sequence of steps. It handles:
- Step execution order and dependencies
- State persistence through StateKeeper
- Error handling and recovery
- Throttling and rate limiting

#### StateKeeper Interface
Provides an abstraction for persisting step states across different backends:

```go
type StateKeeper interface {
    GetSummaryAttributes() map[string]string
    GetSummary() string
    GetStepState(ctx context.Context, stepName string) (*StepState, error)
    SetStepState(ctx context.Context, state *StepState) error
    SupportsRunningState() bool
}
```

#### Step Types
- **SingleStep**: Basic sequential step execution
- **ParallelSteps**: Execute multiple steps concurrently
- **GroupedSteps**: Execute steps in groups with dependencies
- **DynamicStep**: Steps that generate other steps dynamically

### State Backends

#### Kubernetes StateKeeper (`K8sObject`)
Persists step states as Kubernetes conditions on custom resources:

```go
// Create a StateKeeper for a Kubernetes object
stateKeeper := &lifecycle.K8sObject{
    Client:     kubeClient,
    Object:     myCustomResource,
    Conditions: &myCustomResource.Status.Conditions,
}

engine := &lifecycle.StepsEngine{
    StateKeeper: stateKeeper,
    Steps:       steps,
}
```

**State Mapping:**
- `StepStatusSucceeded` → `ConditionTrue`
- `StepStatusFailed` → `ConditionFalse`
- `StepStatusPending` → `ConditionUnknown` or missing condition
- `StepStatusRunning` → Skipped (K8s conditions don't support running state)

#### Database StateKeeper (FSDB)
Persists step states in a database with full lifecycle tracking:

```go
// Example implementation for database state keeper
type FsdbStateKeeper struct {
    stateManager *StateManager
    executionID  string
}

// Supports all step states including running state
func (o *FsdbStateKeeper) SupportsRunningState() bool {
    return true
}
```

## Usage Patterns

### Basic Sequential Execution

```go
steps := []lifecycle.Step{
    &lifecycle.SingleStep{
        Name:        "validate-input",
        EnableState: true,
        Run:         validateInput,
    },
    &lifecycle.SingleStep{
        Name:        "process-data",
        EnableState: true,
        Run:         processData,
        // This step depends on validate-input succeeding
    },
    &lifecycle.SingleStep{
        Name:        "save-results",
        EnableState: true,
        Run:         saveResults,
    },
}
```

### Parallel Execution

```go
parallelStep := &lifecycle.ParallelSteps{
    Name: "parallel-processing",
    Steps: []lifecycle.SimpleStep{
        {Name: "process-batch-1", Run: processBatch1},
        {Name: "process-batch-2", Run: processBatch2},
        {Name: "process-batch-3", Run: processBatch3},
    },
}
```

### State-Aware Steps

```go
step := &lifecycle.SingleStep{
    Name:        "deploy-app",
    EnableState: true,
    StateOverrides: lifecycle.StepStateOverrides{
        Name:    "AppDeployed",           // Condition name in K8s
        Reason:  "DeploymentReady",
        Message: "Application deployed successfully",
    },
    Run: deployApplication,
    // Skip this step if AppDeployed condition is already True
}
```

### Error Handling

```go
step := &lifecycle.SingleStep{
    Name: "risky-operation",
    Run: riskyOperation,
    ContinueOnError: true,  // Continue even if this step fails
    OnFail: func(ctx context.Context, stepName string, err error) error {
        // Custom cleanup or logging
        log.Printf("Step %s failed: %v", stepName, err)
        return nil
    },
}
```

### Predicates and Conditional Execution

```go
step := &lifecycle.SingleStep{
    Name: "conditional-step",
    Run: conditionalOperation,
    Predicates: []lifecycle.PredicateFunc{
        func() bool {
            return someCondition() // Only run if this returns true
        },
    },
    AbortOnPredicatesFalse: false, // Skip instead of aborting
}
```

### Throttling

```go
step := &lifecycle.SingleStep{
    Name: "rate-limited-operation",
    Run: expensiveOperation,
    Throttling: &throttling.ThrottlingSettings{
        Interval:          time.Minute * 5,
        EnsureStepSuccess: true,
    },
}
```

## Advanced Features

### Custom StateKeeper Implementation

```go
type CustomStateKeeper struct {
    // Your custom state storage
}

func (c *CustomStateKeeper) GetSummaryAttributes() map[string]string {
    return map[string]string{"backend": "custom", "version": "1.0"}
}

func (c *CustomStateKeeper) GetSummary() string {
    return "CustomStateKeeper:v1.0"
}

func (c *CustomStateKeeper) GetStepState(ctx context.Context, stepName string) (*lifecycle.StepState, error) {
    // Retrieve state from your backend
}

func (c *CustomStateKeeper) SetStepState(ctx context.Context, state *lifecycle.StepState) error {
    // Persist state to your backend
}

func (c *CustomStateKeeper) SupportsRunningState() bool {
    return true // or false, depending on your backend capabilities
}
```

### Kubernetes Controller Integration

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Get your custom resource
    obj := &v1.MyCustomResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Create StateKeeper
    stateKeeper := &lifecycle.K8sObject{
        Client:     r.Client,
        Object:     obj,
        Conditions: &obj.Status.Conditions,
    }
    
    // Define reconciliation steps
    steps := []lifecycle.Step{
        &lifecycle.SingleStep{
            Name:        "validate-spec",
            EnableState: true,
            StateOverrides: lifecycle.StepStateOverrides{
                Name: "SpecValid",
            },
            Run: r.validateSpec,
        },
        &lifecycle.SingleStep{
            Name:        "deploy-resources",
            EnableState: true,
            StateOverrides: lifecycle.StepStateOverrides{
                Name: "ResourcesDeployed",
            },
            Run: r.deployResources,
        },
        &lifecycle.SingleStep{
            Name:        "wait-ready",
            EnableState: true,
            StateOverrides: lifecycle.StepStateOverrides{
                Name: "Ready",
            },
            Run: r.waitForReady,
        },
    }
    
    // Run steps engine
    engine := &lifecycle.StepsEngine{
        StateKeeper: stateKeeper,
        Steps:       steps,
    }
    
    return engine.RunAsReconcilerResponse(ctx)
}
```

## Error Types

The engine defines several error types for different scenarios:

```go
// Expected errors that don't cause step failure
err := lifecycle.NewExpectedError(someError)

// Errors that should cause a wait/retry
err := lifecycle.NewWaitError(someError) 
err := lifecycle.NewWaitErrorWithDuration(someError, 30*time.Second)

// Retryable errors with custom retry timing
err := &lifecycle.RetryableError{
    Err:        someError,
    RetryAfter: time.Minute * 5,
}
```

## Testing

### Unit Testing Steps

```go
func TestMyStep(t *testing.T) {
    ctx := context.Background()
    
    // Create mock StateKeeper if needed
    stateKeeper := &MockStateKeeper{}
    
    step := &lifecycle.SingleStep{
        Name: "test-step",
        Run: func(ctx context.Context) error {
            // Your step logic
            return nil
        },
    }
    
    engine := &lifecycle.StepsEngine{
        StateKeeper: stateKeeper,
        Steps:       []lifecycle.Step{step},
    }
    
    err := engine.Run(ctx)
    assert.NoError(t, err)
}
```

### Integration Testing with Real StateKeeper

```go
func TestWithRealStateKeeper(t *testing.T) {
    // Set up real Kubernetes client or database connection
    // Run full integration test
}
```

## Best Practices

1. **State Management**
   - Use meaningful step names that map well to condition names
   - Provide clear reasons and messages for debugging
   - Consider whether your StateKeeper needs to track running states

2. **Error Handling**
   - Use appropriate error types (`ExpectedError`, `WaitError`, etc.)
   - Implement proper cleanup in failure callbacks
   - Consider using `ContinueOnError` for non-critical steps

3. **Performance**
   - Use throttling for expensive operations
   - Consider parallel execution for independent steps
   - Monitor state persistence overhead

4. **Observability**
   - Leverage the built-in OpenTelemetry integration
   - Use meaningful step names for tracing
   - Include context in state messages

5. **Testing**
   - Test steps individually with mock StateKeepers
   - Include integration tests with real backends
   - Test error scenarios and recovery paths

## Contributing

1. Follow Go best practices and conventions
2. Add tests for new features
3. Update documentation for interface changes
4. Consider backward compatibility for public APIs
