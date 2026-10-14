package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/logger"
)

func TestContextFieldsAreLogged(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "info").With("process", "api")

	ctx := logger.WithRequestID(context.Background(), "req-1")
	ctx = logger.WithUserID(ctx, "user-1")
	ctx = logger.WithModule(ctx, "procurement")
	log.WithGroup("http").InfoContext(ctx, "request completed", "status", 200)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "request completed", entry["msg"])
	assert.Equal(t, "api", entry["process"])
	assert.Equal(t, "req-1", logger.RequestID(ctx))
	httpGroup, ok := entry["http"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "req-1", httpGroup["request_id"])
	assert.Equal(t, "user-1", httpGroup["user_id"])
	assert.Equal(t, "procurement", httpGroup["module"])
}

func TestLevelFiltering(t *testing.T) {
	tests := []struct {
		level     string
		debugSeen bool
		infoSeen  bool
		warnSeen  bool
	}{
		{"debug", true, true, true},
		{"info", false, true, true},
		{"warn", false, false, true},
		{"error", false, false, false},
		{"tidak-dikenal", false, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			var buf bytes.Buffer
			log := logger.New(&buf, tt.level)
			ctx := context.Background()

			log.DebugContext(ctx, "d")
			assert.Equal(t, tt.debugSeen, buf.Len() > 0)
			buf.Reset()
			log.InfoContext(ctx, "i")
			assert.Equal(t, tt.infoSeen, buf.Len() > 0)
			buf.Reset()
			log.WarnContext(ctx, "w")
			assert.Equal(t, tt.warnSeen, buf.Len() > 0)
		})
	}
}

func TestEmptyContextHasNoExtraFields(t *testing.T) {
	var buf bytes.Buffer
	logger.New(&buf, "info").Info("hello")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.NotContains(t, entry, "request_id")
	assert.Empty(t, logger.RequestID(context.Background()))
}
