package entitystore

import (
	"context"
	"errors"
	"fmt"

	es "github.com/laenen-partners/entitystore"
	"github.com/laenen-partners/entitystore/matching"
	"github.com/laenen-partners/entitystore/store"

	"github.com/laenen-partners/prodcat"
	prodcatv1 "github.com/laenen-partners/prodcat/gen/prodcat/v1"
)

// ImportTracker implements prodcat.ImportTracker backed by entitystore.
// Typically uses the raw (unscoped) EntityStore since imports are
// system-level operations that should not be tenant-scoped.
type ImportTracker struct {
	es  es.EntityStorer
	cfg matching.EntityMatchConfig
}

// NewImportTracker creates a new entitystore-backed import tracker.
func NewImportTracker(e es.EntityStorer) *ImportTracker {
	cfg := prodcatv1.ImportRecordMatchConfig()
	return &ImportTracker{es: e, cfg: cfg}
}

func (t *ImportTracker) HasImported(ctx context.Context, filename string) (bool, error) {
	value := filename
	if t.cfg.Normalizers != nil {
		if fn, ok := t.cfg.Normalizers["filename"]; ok && fn != nil {
			value = fn(value)
		}
	}

	entities, err := t.es.FindByAnchors(ctx, t.cfg.EntityType, []matching.AnchorQuery{
		{Field: "filename", Value: value},
	}, nil)
	if err != nil {
		return false, fmt.Errorf("find import record: %w", err)
	}
	return len(entities) > 0, nil
}

// RecordImported records an import. Uses a MustNotExist precondition to
// atomically skip duplicates — if the filename already exists, it returns
// ErrAlreadyExists instead of creating a duplicate record.
func (t *ImportTracker) RecordImported(ctx context.Context, record prodcat.ImportRecord) error {
	pb := &prodcatv1.ImportRecord{
		Filename:  record.Filename,
		Checksum:  record.Checksum,
		AppliedAt: record.AppliedAt.Format("2006-01-02T15:04:05Z"),
	}

	writeOp := prodcatv1.ImportRecordWriteOp(pb, store.WriteActionCreate)

	_, err := t.es.BatchWrite(ctx, []store.BatchWriteOp{
		{
			WriteEntity: writeOp,
			PreConditions: []store.PreCondition{
				{
					EntityType:   t.cfg.EntityType,
					Anchors:      []matching.AnchorQuery{{Field: "filename", Value: record.Filename}},
					MustNotExist: true,
				},
			},
		},
	})
	if err != nil {
		var pcErr *store.PreConditionError
		if errors.As(err, &pcErr) && pcErr.Violation == "already_exists" {
			return fmt.Errorf("import %q: %w", record.Filename, prodcat.ErrAlreadyExists)
		}
		return err
	}
	return nil
}

func (t *ImportTracker) ListImported(ctx context.Context) ([]prodcat.ImportRecord, error) {
	entities, err := t.es.GetEntitiesByType(ctx, t.cfg.EntityType, 1000, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("list import records: %w", err)
	}

	result := make([]prodcat.ImportRecord, 0, len(entities))
	for _, e := range entities {
		var pb prodcatv1.ImportRecord
		if err := e.GetData(&pb); err != nil {
			continue
		}
		result = append(result, prodcat.ImportRecord{
			Filename: pb.Filename,
			Checksum: pb.Checksum,
		})
	}
	return result, nil
}
