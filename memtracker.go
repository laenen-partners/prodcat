package prodcat

import (
	"context"
	"sync"
)

// MemTracker is an in-memory SeedTracker for testing.
type MemTracker struct {
	mu      sync.RWMutex
	records map[string]SeedRecord
}

// NewMemTracker creates a new in-memory seed tracker.
func NewMemTracker() *MemTracker {
	return &MemTracker{records: make(map[string]SeedRecord)}
}

func (m *MemTracker) HasApplied(_ context.Context, filename string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.records[filename]
	return ok, nil
}

func (m *MemTracker) RecordApplied(_ context.Context, record SeedRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[record.Filename] = record
	return nil
}

func (m *MemTracker) ListApplied(_ context.Context) ([]SeedRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]SeedRecord, 0, len(m.records))
	for _, r := range m.records {
		result = append(result, r)
	}
	return result, nil
}
