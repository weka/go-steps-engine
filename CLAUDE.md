# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Testing
- Run all tests: `go test ./...`
- Run tests with verbose output: `go test -v ./...`
- Run specific test: `go test -run TestName ./lifecycle`
- Run tests in a specific package: `go test ./lifecycle`

### Building
- Build the module: `go build ./...`
- Verify module integrity: `go mod verify`
- Tidy dependencies: `go mod tidy`

### Linting and Formatting
- Format code: `go fmt ./...`
- Run vet (static analysis): `go vet ./...`

## Architecture Overview

### Core Concepts

The go-steps-engine is a state-aware step execution framework with these key components:

**StepsEngine** (`lifecycle/step_engine.go`)
- Main execution orchestrator that runs sequences of steps
- Handles state persistence, error recovery, and throttling
- Integrates with OpenTelemetry for observability

**StateKeeper Interface** (`lifecycle/step_engine.go`)
- Abstraction for persisting step states across different backends
- Key implementations:
  - `K8sObject`: Persists state as Kubernetes conditions
  - Database backends: Support full lifecycle including running states
- Not all StateKeepers support running state (check `SupportsRunningState()`)

**Step Types**
- `SingleStep` (`lifecycle/step.go`): Basic sequential execution
- `ParallelSteps` (`lifecycle/parallel_steps.go`): Concurrent execution
- `GroupedSteps` (`lifecycle/grouped_steps.go`): Steps with dependencies
- `DynamicStep` (`lifecycle/dynamic_step.go`): Runtime step generation

### Step Lifecycle States
- `StepStatusPending`: Not started
- `StepStatusRunning`: Currently executing (not supported by all StateKeepers)
- `StepStatusSucceeded`: Completed successfully
- `StepStatusFailed`: Failed execution

### State Persistence Patterns

**Kubernetes Integration**
- Steps map to Kubernetes conditions on custom resources
- State mapping: `StepStatusSucceeded` → `ConditionTrue`, `StepStatusFailed` → `ConditionFalse`
- Running state is skipped (K8s conditions don't support it)
- Condition names use `StateOverrides.Name` field from steps (or step name if not overridden)

**Database Integration** 
- Full lifecycle support including running states
- Supports detailed state tracking and querying

### Error Handling Framework

The engine defines specific error types for different scenarios:
- `ExpectedError`: Won't cause step failure
- `WaitError`: Triggers wait/retry behavior
- `RetryableError`: Custom retry timing

### Key Configuration Options

**Step Configuration**
- `EnableState`: Controls whether step state should be tracked via StateKeeper
- `StateOverrides`: Customizes state attributes (Name, Reason, Message) when EnableState is true
- `ContinueOnError`: Continue flow even if step fails
- `SkipStepStateCheck`: Run step even if already succeeded
- `AbortOnPredicatesFalse`: Control predicate failure behavior
- `FinishOnSuccess`: Complete execution after step succeeds

**Throttling** (`throttling/`)
- Built-in rate limiting capabilities
- Configurable intervals and success enforcement

## Code Conventions

### Testing Patterns
- Use `SetupLogging()` helper from test files for consistent logging setup
- Mock StateKeepers for unit testing individual steps
- Integration tests with real backends for full system validation
- Test files use standard Go testing patterns with `testify` assertions

### StateKeeper Implementation Guidelines
- Implement all interface methods: `GetSummaryAttributes()`, `GetSummary()`, `GetStepState()`, `SetStepState()`, `SupportsRunningState()`
- Consider whether your backend supports running state tracking
- Provide meaningful summary information for debugging

### Step Implementation Best Practices
- Enable state tracking with `EnableState: true` for steps that need persistence
- Use `StateOverrides` to customize condition names, reasons, and messages for K8s
- Use meaningful step names that map well to condition names in K8s (when StateOverrides.Name is not provided)
- Consider using predicates for conditional execution
- Implement proper cleanup in `OnFail` callbacks

### Kubernetes Controller Integration
- Use `RunAsReconcilerResponse()` method for controller-runtime integration
- Create StateKeeper with reference to custom resource and its conditions
- Map reconciliation logic to meaningful step sequences

## Running Examples

Examples are located in the `examples/` directory. Since examples no longer have individual go.mod files, they should be run from the repository root:

```bash
# Run a specific example from the repository root
go run ./examples/basic-workflow
go run ./examples/parallel-processing  
go run ./examples/error-handling
go run ./examples/k8s-controller
```

Or navigate to the example directory and run:

```bash
cd examples/basic-workflow
go run .
```

See `examples/README.md` for detailed information about each example and their specific use cases.

## Project Structure

- `lifecycle/`: Core engine and step implementations
- `throttling/`: Rate limiting and throttling utilities  
- `util/`: Shared utilities (reflection helpers)
- `examples/`: Reference implementations for different use cases
- Tests are co-located with source files using `*_test.go` naming