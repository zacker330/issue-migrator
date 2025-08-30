package logger

import (
	"log/slog"
	"os"
)

// InitLogger initializes slog for both local and Docker environments
func InitLogger() {
	// Check if running in Docker
	_, inDocker := os.LookupEnv("DOCKER_CONTAINER")
	
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
		AddSource: true,
	}
	
	if inDocker {
		// Use JSON handler for Docker (better for log aggregation)
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		// Use text handler for local development
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	
	logger := slog.New(handler)
	slog.SetDefault(logger)
	
	// Force flush
	os.Stderr.Sync()
	os.Stdout.Sync()
}

// Debug wrapper that also uses fmt for immediate output
func Debug(msg string, args ...any) {
	// Print to stderr immediately (works in Docker)
	os.Stderr.WriteString("[DEBUG] " + msg + "\n")
	// Also use slog
	slog.Debug(msg, args...)
}

// Info wrapper that also uses fmt for immediate output  
func Info(msg string, args ...any) {
	// Print to stderr immediately (works in Docker)
	os.Stderr.WriteString("[INFO] " + msg + "\n")
	// Also use slog
	slog.Info(msg, args...)
}