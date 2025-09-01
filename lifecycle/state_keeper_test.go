package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// MockObject represents a simple Kubernetes object for testing
type MockObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            MockStatus `json:"status,omitempty"`
}

type MockStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (m *MockObject) DeepCopyObject() runtime.Object {
	return &MockObject{
		TypeMeta:   m.TypeMeta,
		ObjectMeta: *m.ObjectMeta.DeepCopy(),
		Status:     m.Status,
	}
}

// MockStateKeeper provides a simple in-memory implementation for testing
type MockStateKeeper struct {
	states               map[string]*StepState
	supportsRunningState bool
	updateError          error
	summary              string
	mutex                sync.RWMutex
}

func NewMockStateKeeper() *MockStateKeeper {
	return &MockStateKeeper{
		states:               make(map[string]*StepState),
		supportsRunningState: true,
		summary:              "MockStateKeeper:test",
	}
}

func (m *MockStateKeeper) GetSummaryAttributes() map[string]string {
	return map[string]string{
		"type":    "mock",
		"version": "1.0",
	}
}

func (m *MockStateKeeper) GetSummary() string {
	return m.summary
}

func (m *MockStateKeeper) GetStepState(ctx context.Context, stepName string) (*StepState, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if state, exists := m.states[stepName]; exists {
		return state, nil
	}
	return &StepState{
		Name:   stepName,
		Status: StepStatusPending,
	}, nil
}

func (m *MockStateKeeper) SetStepState(ctx context.Context, state *StepState) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.updateError != nil {
		return m.updateError
	}

	// Create a copy to avoid external modifications
	stateCopy := &StepState{
		Name:    state.Name,
		Status:  state.Status,
		Reason:  state.Reason,
		Message: state.Message,
	}
	m.states[state.Name] = stateCopy
	return nil
}

func (m *MockStateKeeper) SupportsRunningState() bool {
	return m.supportsRunningState
}

// Test helper methods
func (m *MockStateKeeper) SetUpdateError(err error) {
	m.updateError = err
}

func (m *MockStateKeeper) SetSupportsRunningState(supports bool) {
	m.supportsRunningState = supports
}

func (m *MockStateKeeper) GetStoredStates() map[string]*StepState {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[string]*StepState)
	for k, v := range m.states {
		result[k] = v
	}
	return result
}

// StateTransitionRecorder tracks state transitions for testing
type StateTransitionRecorder struct {
	*MockStateKeeper
	transitions []StepState
	mutex       sync.Mutex
}

func NewStateTransitionRecorder() *StateTransitionRecorder {
	return &StateTransitionRecorder{
		MockStateKeeper: NewMockStateKeeper(),
		transitions:     []StepState{},
	}
}

func (s *StateTransitionRecorder) SetStepState(ctx context.Context, state *StepState) error {
	s.mutex.Lock()
	s.transitions = append(s.transitions, *state)
	s.mutex.Unlock()
	return s.MockStateKeeper.SetStepState(ctx, state)
}

func (s *StateTransitionRecorder) GetTransitions() []StepState {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	result := make([]StepState, len(s.transitions))
	copy(result, s.transitions)
	return result
}

// Tests for MockStateKeeper
func TestMockStateKeeper_BasicOperations(t *testing.T) {
	ctx := context.Background()
	keeper := NewMockStateKeeper()

	// Test initial state
	state, err := keeper.GetStepState(ctx, "test-step")
	require.NoError(t, err)
	assert.Equal(t, "test-step", state.Name)
	assert.Equal(t, StepStatusPending, state.Status)

	// Test setting state
	newState := &StepState{
		Name:    "test-step",
		Status:  StepStatusRunning,
		Reason:  "Starting",
		Message: "Step is starting",
	}
	err = keeper.SetStepState(ctx, newState)
	require.NoError(t, err)

	// Test retrieving updated state
	retrievedState, err := keeper.GetStepState(ctx, "test-step")
	require.NoError(t, err)
	assert.Equal(t, StepStatusRunning, retrievedState.Status)
	assert.Equal(t, "Starting", retrievedState.Reason)
	assert.Equal(t, "Step is starting", retrievedState.Message)
}

func TestMockStateKeeper_MultipleSteps(t *testing.T) {
	ctx := context.Background()
	keeper := NewMockStateKeeper()

	steps := []string{"step1", "step2", "step3"}
	statuses := []StepStatus{StepStatusSucceeded, StepStatusFailed, StepStatusRunning}

	// Set different states for different steps
	for i, stepName := range steps {
		state := &StepState{
			Name:    stepName,
			Status:  statuses[i],
			Reason:  "TestReason",
			Message: "Test message",
		}
		err := keeper.SetStepState(ctx, state)
		require.NoError(t, err)
	}

	// Verify all states
	for i, stepName := range steps {
		retrievedState, err := keeper.GetStepState(ctx, stepName)
		require.NoError(t, err)
		assert.Equal(t, statuses[i], retrievedState.Status)
	}
}

func TestMockStateKeeper_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	keeper := NewMockStateKeeper()

	// Test setting error
	testError := assert.AnError
	keeper.SetUpdateError(testError)

	state := &StepState{
		Name:   "test-step",
		Status: StepStatusSucceeded,
	}
	err := keeper.SetStepState(ctx, state)
	assert.Equal(t, testError, err)

	// Test that state wasn't saved due to error
	retrievedState, err := keeper.GetStepState(ctx, "test-step")
	require.NoError(t, err)
	assert.Equal(t, StepStatusPending, retrievedState.Status)
}

func TestMockStateKeeper_RunningStateSupport(t *testing.T) {
	keeper := NewMockStateKeeper()

	// Test default (supports running state)
	assert.True(t, keeper.SupportsRunningState())

	// Test changing support
	keeper.SetSupportsRunningState(false)
	assert.False(t, keeper.SupportsRunningState())
}

// Tests for K8sObject StateKeeper
func TestK8sObject_BasicOperations(t *testing.T) {
	ctx := context.Background()

	// Create a fake Kubernetes client
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create a mock object
	obj := &MockObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MockObject",
			APIVersion: "test/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-object",
			Namespace: "test-namespace",
		},
	}

	// Create K8sObject StateKeeper
	stateKeeper := &K8sObject{
		Client:     client,
		Object:     obj,
		Conditions: &obj.Status.Conditions,
	}

	// Test summary attributes
	attrs := stateKeeper.GetSummaryAttributes()
	assert.Equal(t, "test-object", attrs["object_name"])
	assert.Equal(t, "test-namespace", attrs["object_namespace"])

	// Test summary string
	summary := stateKeeper.GetSummary()
	assert.Contains(t, summary, "test-object")
	assert.Contains(t, summary, "test-namespace")

	// Test supports running state
	assert.False(t, stateKeeper.SupportsRunningState())

	// Test getting step state (should return pending for non-existent condition)
	state, err := stateKeeper.GetStepState(ctx, "TestCondition")
	require.NoError(t, err)
	assert.Equal(t, "TestCondition", state.Name)
	assert.Equal(t, StepStatusPending, state.Status)
}

func TestK8sObject_ConditionMapping(t *testing.T) {
	ctx := context.Background()

	// Create object with existing conditions
	obj := &MockObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MockObject",
			APIVersion: "test/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-object",
			Namespace: "test-namespace",
		},
		Status: MockStatus{
			Conditions: []metav1.Condition{
				{
					Type:    "TestCondition",
					Status:  metav1.ConditionTrue,
					Reason:  "Success",
					Message: "Test successful",
				},
				{
					Type:    "FailedCondition",
					Status:  metav1.ConditionFalse,
					Reason:  "Error",
					Message: "Test failed",
				},
			},
		},
	}

	stateKeeper := &K8sObject{
		Object:     obj,
		Conditions: &obj.Status.Conditions,
	}

	// Test succeeded condition
	state, err := stateKeeper.GetStepState(ctx, "TestCondition")
	require.NoError(t, err)
	assert.Equal(t, StepStatusSucceeded, state.Status)
	assert.Equal(t, "Success", state.Reason)
	assert.Equal(t, "Test successful", state.Message)

	// Test failed condition
	state, err = stateKeeper.GetStepState(ctx, "FailedCondition")
	require.NoError(t, err)
	assert.Equal(t, StepStatusFailed, state.Status)
	assert.Equal(t, "Error", state.Reason)
	assert.Equal(t, "Test failed", state.Message)
}

func TestK8sObject_RunningStateHandling(t *testing.T) {
	obj := &MockObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-object",
			Namespace: "test-namespace",
		},
	}

	stateKeeper := &K8sObject{
		Object:     obj,
		Conditions: &obj.Status.Conditions,
	}

	// Test that running state is skipped
	ctx := context.Background()
	runningState := &StepState{
		Name:   "TestStep",
		Status: StepStatusRunning,
		Reason: "InProgress",
	}

	// This should not return an error, but should skip setting the condition
	err := stateKeeper.SetStepState(ctx, runningState)
	assert.NoError(t, err)

	// Verify no conditions were added
	assert.Empty(t, obj.Status.Conditions)
}

func TestStepState_StatusEqual(t *testing.T) {
	state := &StepState{
		Status: StepStatusSucceeded,
	}

	assert.True(t, state.StatusEqual(StepStatusSucceeded))
	assert.False(t, state.StatusEqual(StepStatusFailed))
	assert.False(t, state.StatusEqual(StepStatusPending))
	assert.False(t, state.StatusEqual(StepStatusRunning))
}

// Integration tests combining StepsEngine with StateKeepers
func TestStepsEngine_WithMockStateKeeper(t *testing.T) {
	ctx := context.Background()

	// Setup observability
	_, shutdown := SetupLogging(ctx)
	defer shutdown(ctx)

	callCount := 0
	testStep := &SimpleStep{
		Name:  "test-step",
		State: &State{Name: "test-step"},
		Run: func(ctx context.Context) error {
			callCount++
			return nil
		},
	}

	stateKeeper := NewMockStateKeeper()
	engine := &StepsEngine{
		StateKeeper: stateKeeper,
		Steps:       []Step{testStep},
	}

	// Run the engine
	err := engine.Run(ctx)
	require.NoError(t, err)

	// Verify step was called
	assert.Equal(t, 1, callCount)

	// Verify state was updated
	states := stateKeeper.GetStoredStates()
	assert.Contains(t, states, "test-step")
	assert.Equal(t, StepStatusSucceeded, states["test-step"].Status)
}

func TestStepsEngine_SkipRunningStateForK8s(t *testing.T) {
	ctx := context.Background()

	// Setup observability
	_, shutdown := SetupLogging(ctx)
	defer shutdown(ctx)

	obj := &MockObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MockObject",
			APIVersion: "test/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-object",
			Namespace: "test-namespace",
		},
	}

	stateKeeper := &K8sObject{
		Client:     nil, // No client needed for this test
		Object:     obj,
		Conditions: &obj.Status.Conditions,
	}

	// Test that K8sObject doesn't support running state
	assert.False(t, stateKeeper.SupportsRunningState(), "K8sObject should not support running states")

	// Test that setting running state is a no-op
	runningState := &StepState{
		Name:   "TestStep",
		Status: StepStatusRunning,
		Reason: "InProgress",
	}

	err := stateKeeper.SetStepState(ctx, runningState)
	assert.NoError(t, err, "Setting running state should not error (it should be skipped)")

	// Verify no conditions were added (running states are skipped)
	assert.Empty(t, obj.Status.Conditions, "No conditions should be set for running states")
}

func TestStepsEngine_SetRunningStateForFSDB(t *testing.T) {
	ctx := context.Background()

	// Setup observability
	_, shutdown := SetupLogging(ctx)
	defer shutdown(ctx)

	stateRecorder := NewStateTransitionRecorder() // Supports running state by default

	testStep := &SimpleStep{
		Name:  "test-step",
		State: &State{Name: "test-step"},
		Run: func(ctx context.Context) error {
			// Just a simple step that succeeds
			return nil
		},
	}

	engine := &StepsEngine{
		StateKeeper: stateRecorder,
		Steps:       []Step{testStep},
	}

	// Run the engine
	err := engine.Run(ctx)
	require.NoError(t, err)

	// Verify final state is succeeded
	finalState, err := stateRecorder.GetStepState(ctx, "test-step")
	require.NoError(t, err)
	assert.Equal(t, StepStatusSucceeded, finalState.Status)

	// Verify state transitions: should have running -> succeeded
	transitions := stateRecorder.GetTransitions()
	require.Len(t, transitions, 2, "Expected running and succeeded state transitions")

	// First transition should be running state using step name
	assert.Equal(t, StepStatusRunning, transitions[0].Status)
	assert.Equal(t, "test-step", transitions[0].Name)

	// Second transition should be succeeded state using step name
	assert.Equal(t, StepStatusSucceeded, transitions[1].Status)
	assert.Equal(t, "test-step", transitions[1].Name)
}

// Benchmark tests
func BenchmarkMockStateKeeper_SetState(b *testing.B) {
	ctx := context.Background()
	keeper := NewMockStateKeeper()

	state := &StepState{
		Name:    "benchmark-step",
		Status:  StepStatusSucceeded,
		Reason:  "Benchmark",
		Message: "Benchmarking state updates",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = keeper.SetStepState(ctx, state)
	}
}

func BenchmarkMockStateKeeper_GetState(b *testing.B) {
	ctx := context.Background()
	keeper := NewMockStateKeeper()

	// Set up initial state
	state := &StepState{
		Name:   "benchmark-step",
		Status: StepStatusSucceeded,
	}
	_ = keeper.SetStepState(ctx, state)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = keeper.GetStepState(ctx, "benchmark-step")
	}
}

// Test concurrent access
func TestMockStateKeeper_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	keeper := NewMockStateKeeper()

	// Test concurrent writes to different steps
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(stepID int) {
			state := &StepState{
				Name:    fmt.Sprintf("step-%d", stepID),
				Status:  StepStatusSucceeded,
				Reason:  "ConcurrentTest",
				Message: fmt.Sprintf("Concurrent test %d", stepID),
			}
			err := keeper.SetStepState(ctx, state)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all states were set correctly
	for i := 0; i < 10; i++ {
		state, err := keeper.GetStepState(ctx, fmt.Sprintf("step-%d", i))
		require.NoError(t, err)
		assert.Equal(t, StepStatusSucceeded, state.Status)
		assert.Equal(t, "ConcurrentTest", state.Reason)
	}
}
