package singtun

import (
	"context"

	"github.com/sagernet/sing/common/logger"
)

type nopLogger struct{}

func (nopLogger) Trace(args ...any)                             {}
func (nopLogger) Debug(args ...any)                             {}
func (nopLogger) Info(args ...any)                              {}
func (nopLogger) Warn(args ...any)                              {}
func (nopLogger) Error(args ...any)                             {}
func (nopLogger) Fatal(args ...any)                             {}
func (nopLogger) Panic(args ...any)                             {}
func (nopLogger) TraceContext(ctx context.Context, args ...any) {}
func (nopLogger) DebugContext(ctx context.Context, args ...any) {}
func (nopLogger) InfoContext(ctx context.Context, args ...any)  {}
func (nopLogger) WarnContext(ctx context.Context, args ...any)  {}
func (nopLogger) ErrorContext(ctx context.Context, args ...any) {}
func (nopLogger) FatalContext(ctx context.Context, args ...any) {}
func (nopLogger) PanicContext(ctx context.Context, args ...any) {}

var _ logger.ContextLogger = nopLogger{}
