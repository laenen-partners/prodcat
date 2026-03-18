package prodcat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/laenen-partners/entitystore"
	"github.com/laenen-partners/entitystore/matching"
)

const EntityTypeSeedRecord = "eligibility.v1.SeedRecord"

// ESSeedTracker implements SeedTracker backed by entitystore.
type ESSeedTracker struct {
	es *entitystore.EntityStore
}

// NewESSeedTracker creates a new entitystore-backed seed tracker.
func NewESSeedTracker(es *entitystore.EntityStore) *ESSeedTracker {
	return &ESSeedTracker{es: es}
}

func (t *ESSeedTracker) HasApplied(ctx context.Context, filename string) (bool, error) {
	entities, err := t.es.FindByAnchors(ctx, EntityTypeSeedRecord, []matching.AnchorQuery{
		{Field: "filename", Value: filename},
	}, nil)
	if err != nil {
		return false, fmt.Errorf("find seed record: %w", err)
	}
	return len(entities) > 0, nil
}

func (t *ESSeedTracker) RecordApplied(ctx context.Context, record SeedRecord) error {
	data, err := entitystore.MarshalEntityData(record)
	if err != nil {
		return fmt.Errorf("marshal seed record: %w", err)
	}

	_, err = t.es.BatchWrite(ctx, []entitystore.BatchWriteOp{
		{WriteEntity: &entitystore.WriteEntityOp{
			Action:     entitystore.WriteActionCreate,
			EntityType: EntityTypeSeedRecord,
			Data:       data,
			Confidence: 1.0,
			Anchors: []matching.AnchorQuery{
				{Field: "filename", Value: record.Filename},
			},
		}},
	})
	return err
}

func (t *ESSeedTracker) ListApplied(ctx context.Context) ([]SeedRecord, error) {
	entities, err := t.es.GetEntitiesByType(ctx, EntityTypeSeedRecord, 1000, nil)
	if err != nil {
		return nil, fmt.Errorf("list seed records: %w", err)
	}

	result := make([]SeedRecord, 0, len(entities))
	for _, e := range entities {
		var r SeedRecord
		if err := json.Unmarshal(e.Data, &r); err != nil {
			continue
		}
		result = append(result, r)
	}
	return result, nil
}
