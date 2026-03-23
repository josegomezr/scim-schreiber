package main

// TODO: DRY into /internal

import (
	"fmt"
	"log/slog"
)

type ScimLogger struct {
}

func (c *ScimLogger) Error(args ...any) {
	slog.Error(fmt.Sprintln(args...))
}
