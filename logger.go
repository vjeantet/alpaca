// Copyright 2025 The Alpaca Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// LevelTrace is a custom log level more verbose than DEBUG.
const LevelTrace = slog.Level(-8)

const contextKeyLogger = contextKey("logger")

// parseLogLevel parses a level string, extending slog.Level.UnmarshalText
// with support for the custom "trace" level.
func parseLogLevel(s string) (slog.Level, error) {
	if strings.EqualFold(s, "trace") {
		return LevelTrace, nil
	}
	var level slog.Level
	err := level.UnmarshalText([]byte(s))
	return level, err
}

func setupLogger(level slog.Level, jsonFormat bool) {
	replaceAttr := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.LevelKey {
			if lvl, ok := a.Value.Any().(slog.Level); ok && lvl == LevelTrace {
				a.Value = slog.StringValue("TRACE")
			}
		}
		return a
	}
	opts := &slog.HandlerOptions{Level: level, ReplaceAttr: replaceAttr}
	var handler slog.Handler
	if jsonFormat {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))
}

func loggerFromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKeyLogger).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

func withLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKeyLogger, logger)
}
