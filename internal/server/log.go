package server

import (
	"log/slog"
	"os"
)

func GetLogger() *slog.Logger {
	logLevel := new(slog.LevelVar)

	if logLevelStr := os.Getenv("LOG_LEVEL"); logLevelStr != "" {
		if err := logLevel.UnmarshalText([]byte(logLevelStr)); err != nil {
			slog.Info("Falling back to LOG_LEVEL Info")
		}
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
}
