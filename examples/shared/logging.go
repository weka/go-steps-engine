// Package shared provides shared utilities for all examples
package shared

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/zerologr"
	"github.com/rs/zerolog"
	"github.com/weka/go-weka-observability/instrumentation"
	"github.com/weka/go-weka-observability/logger"
)

// SetupLogging configures logging and OpenTelemetry for examples.
// It returns a logger and a shutdown function that should be called when the example completes.
func SetupLogging(ctx context.Context, serviceName string) (ctxNew context.Context, shutdown func(context.Context) error) {
	writer := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly}
	zeroLogger := zerolog.New(writer).Level(zerolog.DebugLevel).With().Timestamp().Logger()
	logr := zerologr.New(&zeroLogger)

	ctxNew = logger.ContextWithLogr(ctx, logr)

	shutdown, err := instrumentation.SetupOTelSDKWithOptions(ctxNew, serviceName, "", logr)
	if err != nil {
		panic(fmt.Sprintf("failed to setup OTel SDK: %v", err))
	}
	return
}
