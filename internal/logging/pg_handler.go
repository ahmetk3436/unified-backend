package logging

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// PGHandler is an slog.Handler that batches ERROR+ logs to PostgreSQL.
type PGHandler struct {
	db      *gorm.DB
	mu      sync.Mutex
	buffer  []models.SystemLog
	ticker  *time.Ticker
	done    chan struct{}
}

func NewPGHandler(db *gorm.DB) *PGHandler {
	h := &PGHandler{
		db:     db,
		buffer: make([]models.SystemLog, 0, 50),
		ticker: time.NewTicker(5 * time.Second),
		done:   make(chan struct{}),
	}
	go h.flushLoop()
	return h
}

func (h *PGHandler) flushLoop() {
	for {
		select {
		case <-h.ticker.C:
			h.flush()
		case <-h.done:
			h.flush()
			return
		}
	}
}

func (h *PGHandler) flush() {
	h.mu.Lock()
	if len(h.buffer) == 0 {
		h.mu.Unlock()
		return
	}
	batch := h.buffer
	h.buffer = make([]models.SystemLog, 0, 50)
	h.mu.Unlock()

	if err := h.db.CreateInBatches(batch, 50).Error; err != nil {
		slog.Error("failed to flush system logs to DB", "error", err, "count", len(batch))
	}
}

func (h *PGHandler) Stop() {
	h.ticker.Stop()
	close(h.done)
}

// Enabled only handles ERROR and above.
func (h *PGHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelError
}

func (h *PGHandler) Handle(_ context.Context, record slog.Record) error {
	entry := models.SystemLog{
		ID:        uuid.New(),
		Timestamp: record.Time,
		Level:     record.Level.String(),
		Message:   record.Message,
	}

	extra := make(map[string]interface{})
	record.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "app_id":
			entry.AppID = a.Value.String()
		case "trace_id":
			entry.TraceID = a.Value.String()
		case "user_id":
			s := a.Value.String()
			entry.UserID = &s
		case "action":
			entry.Action = a.Value.String()
		case "error":
			entry.Error = a.Value.String()
		case "latency_ms":
			if f, ok := a.Value.Any().(float64); ok {
				entry.LatencyMs = int(math.Round(f))
			}
		default:
			extra[a.Key] = a.Value.Any()
		}
		return true
	})

	if len(extra) > 0 {
		if b, err := json.Marshal(extra); err == nil {
			entry.Extra = datatypes.JSON(b)
		}
	}

	h.mu.Lock()
	h.buffer = append(h.buffer, entry)
	needFlush := len(h.buffer) >= 50
	h.mu.Unlock()

	if needFlush {
		go h.flush()
	}
	return nil
}

func (h *PGHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *PGHandler) WithGroup(name string) slog.Handler {
	return h
}
