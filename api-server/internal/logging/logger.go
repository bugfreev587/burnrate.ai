package logging

import (
	"context"
	"log/slog"
	"os"
	"time"

	slogbetterstack "github.com/samber/slog-betterstack"
)

// multiHandler fans out slog records to multiple handlers.
type multiHandler struct {
	handlers []slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			_ = handler.Handle(ctx, r.Clone())
		}
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

// Init sets up the global slog logger with stdout JSON output and optional
// Better Stack forwarding. It bridges Go's standard log package through slog
// so existing log.Printf calls are also captured.
//
// Returns a shutdown function that should be deferred in main().
func Init(betterstackToken string) func() {
	handlers := []slog.Handler{
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
	}

	hasBetterStack := false
	if betterstackToken != "" {
		bsHandler := slogbetterstack.Option{
			Token: betterstackToken,
		}.NewBetterstackHandler()
		handlers = append(handlers, bsHandler)
		hasBetterStack = true
	}

	logger := slog.New(&multiHandler{handlers: handlers})
	slog.SetDefault(logger)

	// Bridge standard log package through slog so existing log.Printf calls
	// are captured by both stdout and Better Stack.
	slog.SetLogLoggerLevel(slog.LevelInfo)

	return func() {
		if hasBetterStack {
			// The Better Stack handler sends logs asynchronously via goroutines.
			// Allow a brief window for in-flight HTTP requests to complete.
			time.Sleep(500 * time.Millisecond)
		}
	}
}
