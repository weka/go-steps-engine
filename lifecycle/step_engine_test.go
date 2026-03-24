package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"

	"github.com/weka/go-weka-observability/instrumentation"
)

func SetupLogging(ctx context.Context) (logger logr.Logger, shutdown func(context.Context) error) {
	writer := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly}
	zeroLogger := zerolog.New(writer).Level(zerolog.DebugLevel).With().Timestamp().Logger()
	logger = zerologr.New(&zeroLogger)

	shutdown, err := instrumentation.SetupOTelSDKWithOptions(ctx, "weka-k8s-testing-unittests", "", logger)
	if err != nil {
		panic(fmt.Sprintf("failed to setup OTel SDK: %v", err))
	}
	return
}

type MockSuccess struct {
	CallCount int
	mutex     sync.Mutex
}

func (m *MockSuccess) Run(_ context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.CallCount++
	return nil
}

type MockFail struct {
	CallCount int
	mutex     sync.Mutex
}

func (m *MockFail) Run(_ context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.CallCount++
	return errors.New("mock error")
}

func TestStepEngineSuccess(t *testing.T) {
	ctx := context.Background()

	_, shutdown := SetupLogging(ctx)
	defer shutdown(ctx)

	mockSuccess := &MockSuccess{}

	stepsEngine := StepsEngine{
		Steps: []Step{
			&SimpleStep{
				Name: "test",
				Run:  mockSuccess.Run,
			},
			&ParallelSteps{
				Name: "test-parallel",
				Steps: []Step{
					&SimpleStep{
						Name: "test-parallel-step1",
						Run:  mockSuccess.Run,
					},
					&SimpleStep{
						Name: "test-parallel-step2",
						Run:  mockSuccess.Run,
					},
				},
			},
		},
	}

	err := stepsEngine.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mockSuccess.CallCount != 3 {
		t.Fatalf("unexpected call count: %d", mockSuccess.CallCount)
	}
}

func TestStepEngineFailOnFirstStep(t *testing.T) {
	ctx := context.Background()

	_, shutdown := SetupLogging(ctx)
	defer shutdown(ctx)

	mockFail := &MockFail{}

	stepsEngine := StepsEngine{
		Steps: []Step{
			&SimpleStep{
				Name: "test1",
				Run:  mockFail.Run,
			},
			&SimpleStep{
				Name: "test2",
				Run:  mockFail.Run,
			},
		},
	}

	err := stepsEngine.Run(ctx)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	if mockFail.CallCount != 1 {
		t.Fatalf("unexpected call count: %d", mockFail.CallCount)
	}
}

func TestDynamicStepWithState(t *testing.T) {
	ctx := context.Background()

	_, shutdown := SetupLogging(ctx)
	defer shutdown(ctx)

	mockStateKeeper := &MockStateKeeper{}
	mockSuccess := &MockSuccess{}

	dynamicStep := &DynamicStep{
		Name: "test-dynamic-step",
		State: &State{
			Name:    "test-dynamic-state",
			Reason:  "TestReason",
			Message: "Test message",
		},
		GetStep: func() Step {
			return &SimpleStep{
				Name: "inner-step",
				Run:  mockSuccess.Run,
			}
		},
	}

	// Test that HasState returns true when State is set
	assert.True(t, dynamicStep.HasState())

	// Test GetStepStateName returns the state name
	assert.Equal(t, "test-dynamic-state", dynamicStep.GetStepStateName())

	// Test GetSucceededState returns correct state
	successState := dynamicStep.GetSucceededState()
	assert.NotNil(t, successState)
	assert.Equal(t, "test-dynamic-state", successState.Name)
	assert.Equal(t, "TestReason", successState.Reason)
	assert.Equal(t, "Test message", successState.Message)
	assert.Equal(t, StepStatusSucceeded, successState.Status)

	// Test that ShouldSkip returns false when state doesn't exist
	assert.False(t, dynamicStep.ShouldSkip(ctx, mockStateKeeper))

	// Test fallback to step name when State.Name is empty
	dynamicStepNoStateName := &DynamicStep{
		Name:  "fallback-step",
		State: &State{}, // Empty State.Name
		GetStep: func() Step {
			return &SimpleStep{Name: "inner", Run: mockSuccess.Run}
		},
	}
	assert.Equal(t, "fallback-step", dynamicStepNoStateName.GetStepStateName())
}

func TestGroupedStepsWithState(t *testing.T) {
	logger, shutdown := SetupLogging(context.Background())
	ctx := context.Background()
	defer func() { _ = shutdown(ctx) }()
	ctx = logr.NewContext(ctx, logger)

	mockSuccess := &MockSuccess{}
	mockStateKeeper := &MockStateKeeper{}

	// Create steps without state
	step1 := &SimpleStep{Name: "step1", Run: mockSuccess.Run}
	step2 := &SimpleStep{Name: "step2", Run: mockSuccess.Run}

	// Create grouped steps without state initially
	groupedSteps := &GroupedSteps{
		Name:  "test-group",
		Steps: []Step{step1, step2},
	}

	// GroupedSteps should not have state initially
	assert.False(t, groupedSteps.HasState())
	assert.False(t, groupedSteps.ShouldSkip(ctx, mockStateKeeper))

	// Test that SetState works for the GroupedSteps itself
	state := &State{
		Name:    "group-state",
		Reason:  "TestReason",
		Message: "Test message",
	}

	groupedSteps.SetState(state)

	// Verify that GroupedSteps now has state
	assert.True(t, groupedSteps.HasState())
	assert.Equal(t, "group-state", groupedSteps.GetStepStateName())

	// Verify underlying steps still don't have state
	assert.False(t, step1.HasState())
	assert.False(t, step2.HasState())

	// Verify state details are correct
	groupState := groupedSteps.GetSucceededState()
	assert.NotNil(t, groupState)
	assert.Equal(t, "group-state", groupState.Name)
	assert.Equal(t, "TestReason", groupState.Reason)
	assert.Equal(t, "Test message", groupState.Message)
	assert.Equal(t, StepStatusSucceeded, groupState.Status)

	// Test fallback to step name when State.Name is empty
	groupedStepsNoStateName := &GroupedSteps{
		Name:  "fallback-group",
		Steps: []Step{step1},
		State: &State{}, // Empty State.Name
	}
	assert.Equal(t, "fallback-group", groupedStepsNoStateName.GetStepStateName())
}
