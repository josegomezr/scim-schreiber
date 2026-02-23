package main

import (
	"fmt"
	"log/slog"
)

type ScimLogger struct {
}

func (c *ScimLogger) Error(args ...any) {
	slog.Info(fmt.Sprintln(args...))
}
