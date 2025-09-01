# Go Steps Engine Examples

This directory contains practical examples demonstrating how to use the go-steps-engine in various scenarios. Each example focuses on specific patterns and use cases to help you understand how to implement step-based workflows effectively.

## Examples Overview

### 1. [Basic Workflow](./basic-workflow/) 
**File:** `basic-workflow/main.go`

A simple sequential workflow demonstrating the fundamental concepts:
- Basic step execution
- In-memory state tracking with MockStateKeeper
- Sequential step processing
- State persistence and retrieval

**Key Concepts:**
- `SimpleStep` usage
- `StateKeeper` interface implementation
- Basic workflow patterns

**Run the example:**
```bash
# From repository root
go run ./examples/basic-workflow

# Or from example directory
cd examples/basic-workflow
go run .
```

### 2. [Kubernetes Controller](./k8s-controller/)
**File:** `k8s-controller/main.go`

Demonstrates integration with Kubernetes controllers using real K8s objects and conditions:
- Kubernetes condition-based state persistence
- Controller reconciliation patterns
- Custom resource management
- Kubernetes client integration

**Key Concepts:**
- `K8sObject` StateKeeper
- Kubernetes conditions mapping
- Controller reconciliation loops
- `RunAsReconcilerResponse()` usage

**Run the example:**
```bash
# From repository root
go run ./examples/k8s-controller

# Or from example directory
cd examples/k8s-controller
go run .
```

### 3. [Parallel Processing](./parallel-processing/)
**File:** `parallel-processing/main.go`

Shows how to execute multiple operations concurrently for improved performance:
- Parallel batch processing
- Concurrent API calls
- Thread-safe state management
- Performance optimization patterns

**Key Concepts:**
- `ParallelSteps` implementation
- Thread-safe StateKeeper
- Concurrent execution patterns
- Performance considerations

**Run the example:**
```bash
# From repository root
go run ./examples/parallel-processing

# Or from example directory
cd examples/parallel-processing
go run .
```

### 4. [Error Handling and Recovery](./error-handling/)
**File:** `error-handling/main.go`

Comprehensive error handling patterns and recovery strategies:
- Expected errors for optional features
- Wait errors for retryable conditions
- Cleanup callbacks on failures
- Non-critical step continuation
- Custom error types

**Key Concepts:**
- `ExpectedError` usage
- `WaitError` for retries
- `OnFail` callbacks
- `ContinueOnError` patterns
- Error recovery strategies

**Run the example:**
```bash
# From repository root
go run ./examples/error-handling

# Or from example directory
cd examples/error-handling
go run .
```

## Running All Examples

To run all examples at once:

```bash
#!/bin/bash
# Run from repository root
echo "Running Basic Workflow Example..."
go run ./examples/basic-workflow
echo -e "\n" + "="*50 + "\n"

echo "Running Kubernetes Controller Example..."  
go run ./examples/k8s-controller
echo -e "\n" + "="*50 + "\n"

echo "Running Parallel Processing Example..."
go run ./examples/parallel-processing
echo -e "\n" + "="*50 + "\n"

echo "Running Error Handling Example..."
go run ./examples/error-handling
```

## Common Patterns Across Examples

### StateKeeper Implementations

Each example demonstrates different StateKeeper implementations:

1. **MockStateKeeper** (basic-workflow): Simple in-memory storage for testing and development
2. **K8sObject** (k8s-controller): Kubernetes condition-based persistence for production controllers  
3. **InMemoryStateKeeper** (parallel-processing): Thread-safe in-memory storage for concurrent access
4. **SimpleStateKeeper** (error-handling): Minimal implementation focusing on error scenarios

### Step Types Used

- **SimpleStep**: Individual sequential operations
- **ParallelSteps**: Concurrent execution of independent operations
- **Error handling**: ExpectedError, WaitError, and failure callbacks

### Best Practices Demonstrated

1. **State Management**
   - Use appropriate StateKeeper for your persistence needs
   - Provide meaningful step names and state messages
   - Handle concurrent access when needed

2. **Error Handling**
   - Use ExpectedError for optional/non-critical failures
   - Use WaitError for retryable conditions
   - Implement cleanup callbacks for resource management
   - Use ContinueOnError for non-critical steps

3. **Performance**
   - Use ParallelSteps for independent operations
   - Consider StateKeeper performance characteristics
   - Implement appropriate timeout and retry strategies

4. **Observability**
   - Provide clear step names and status messages
   - Use structured logging in your step implementations
   - Leverage the built-in OpenTelemetry integration

## Next Steps

After exploring these examples, you can:

1. **Read the main documentation**: Check out the [main README](../README.md) for comprehensive API documentation
2. **Review the tests**: Look at the unit tests in `lifecycle/` for additional usage patterns
3. **Build your own workflow**: Use these examples as templates for your specific use cases
4. **Contribute**: Help improve the library by adding more examples or improving existing ones

## Example Dependencies

Most examples use only the standard library and the go-steps-engine itself. The Kubernetes controller example additionally uses:
- `k8s.io/api`
- `k8s.io/apimachinery`
- `sigs.k8s.io/controller-runtime`

Examples use the main module's dependencies, so no additional setup is required. Simply run them from the repository root:

```bash
go run ./examples/<example-name>
```
