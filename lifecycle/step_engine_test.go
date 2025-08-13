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

	"github.com/weka/go-weka-observability/instrumentation"
)

func SetupLogging(ctx context.Context) (logger logr.Logger, shutdown func(context.Context) error) {
	writer := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly}
	zeroLogger := zerolog.New(writer).Level(zerolog.DebugLevel).With().Timestamp().Logger()
	logger = zerologr.New(&zeroLogger)

	shutdown, err := instrumentation.SetupOTelSDK(ctx, "weka-k8s-testing-unittests", "", logger)
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
			&SingleStep{
				Name: "test",
				Run:  mockSuccess.Run,
			},
			&ParallelSteps{
				Name: "test-parallel",
				Steps: []SimpleStep{
					{
						Name: "test-parallel-step1",
						Run:  mockSuccess.Run,
					},
					{
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
			&SingleStep{
				Name: "test1",
				Run:  mockFail.Run,
			},
			&SingleStep{
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
