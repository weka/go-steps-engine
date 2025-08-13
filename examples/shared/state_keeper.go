// Package shared provides shared utilities for all examples
package shared

import (
	"context"
	"fmt"
	"sync"

	"github.com/weka/go-steps-engine/lifecycle"
)

// InMemoryStateKeeper provides a thread-safe in-memory state keeper implementation
// for use in examples and testing. It supports running state tracking and provides
// detailed logging of state transitions.
type InMemoryStateKeeper struct {
	states map[string]*lifecycle.StepState
	mutex  sync.RWMutex
	name   string // workflow name for identification
}

// NewInMemoryStateKeeper creates a new in-memory state keeper for the given workflow name
func NewInMemoryStateKeeper(workflowName string) *InMemoryStateKeeper {
	return &InMemoryStateKeeper{
		states: make(map[string]*lifecycle.StepState),
		name:   workflowName,
	}
}

// GetSummaryAttributes returns attributes that identify this state keeper
func (s *InMemoryStateKeeper) GetSummaryAttributes() map[string]string {
	return map[string]string{
		"type":     "in-memory",
		"workflow": s.name,
	}
}

// GetSummary returns a string summary of this state keeper
func (s *InMemoryStateKeeper) GetSummary() string {
	return fmt.Sprintf("InMemoryStateKeeper:%s", s.name)
}

// GetStepState retrieves the current state of a step by name
func (s *InMemoryStateKeeper) GetStepState(ctx context.Context, stepName string) (*lifecycle.StepState, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if state, exists := s.states[stepName]; exists {
		// Return a copy to avoid external modifications
		return &lifecycle.StepState{
			Name:    state.Name,
			Status:  state.Status,
			Reason:  state.Reason,
			Message: state.Message,
		}, nil
	}

	// Return default pending state if not found
	return &lifecycle.StepState{
		Name:   stepName,
		Status: lifecycle.StepStatusPending,
	}, nil
}

// SetStepState persists the state of a step with logging
func (s *InMemoryStateKeeper) SetStepState(ctx context.Context, state *lifecycle.StepState) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Store a copy to avoid external modifications
	s.states[state.Name] = &lifecycle.StepState{
		Name:    state.Name,
		Status:  state.Status,
		Reason:  state.Reason,
		Message: state.Message,
	}

	// Log the state transition with appropriate icon
	statusIcon := "✅"
	switch state.Status {
	case lifecycle.StepStatusFailed:
		statusIcon = "❌"
	case lifecycle.StepStatusRunning:
		statusIcon = "🔄"
	case lifecycle.StepStatusPending:
		statusIcon = "⏸️"
	}

	message := state.Message
	if message == "" {
		message = fmt.Sprintf("Step %s", state.Status)
	}

	fmt.Printf("%s %s: %s - %s\n", statusIcon, state.Name, state.Status, message)
	return nil
}

// SupportsRunningState indicates this state keeper can track running states
func (s *InMemoryStateKeeper) SupportsRunningState() bool {
	return true
}

// GetAllStates returns all stored states (useful for debugging and final status display)
func (s *InMemoryStateKeeper) GetAllStates() map[string]*lifecycle.StepState {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Return copies to avoid external modifications
	result := make(map[string]*lifecycle.StepState)
	for name, state := range s.states {
		result[name] = &lifecycle.StepState{
			Name:    state.Name,
			Status:  state.Status,
			Reason:  state.Reason,
			Message: state.Message,
		}
	}
	return result
}

// PrintFinalStates prints a formatted summary of all step states
func (s *InMemoryStateKeeper) PrintFinalStates() {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	fmt.Println("\nFinal step states:")
	fmt.Println("------------------")

	for stepName, state := range s.states {
		statusIcon := "✅"
		switch state.Status {
		case lifecycle.StepStatusFailed:
			statusIcon = "❌"
		case lifecycle.StepStatusPending:
			statusIcon = "⏸️"
		case lifecycle.StepStatusRunning:
			statusIcon = "🔄"
		}

		fmt.Printf("%s %-25s: %s\n", statusIcon, stepName, state.Status)
		if state.Message != "" && state.Message != "Completed successfully" {
			fmt.Printf("   └─ %s\n", state.Message)
		}
	}
}
