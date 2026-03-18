package prodcat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/laenen-partners/entitystore"
	"github.com/laenen-partners/entitystore/matching"
)

// Entity type constants for entitystore.
const (
	EntityTypeProduct      = "eligibility.v1.Product"
	EntityTypeRuleset      = "eligibility.v1.Ruleset"
	EntityTypeSubscription = "eligibility.v1.Subscription"
)

// ESStore implements Store backed by entitystore.
type ESStore struct {
	es *entitystore.EntityStore
}

// NewESStore creates a new entitystore-backed Store.
func NewESStore(es *entitystore.EntityStore) *ESStore {
	return &ESStore{es: es}
}

// ─── Products ───

func (s *ESStore) PutProduct(ctx context.Context, p ProductEligibility) error {
	data, err := entitystore.MarshalEntityData(p)
	if err != nil {
		return fmt.Errorf("marshal product: %w", err)
	}

	// Try to find existing entity by anchor.
	existing, err := s.es.FindByAnchors(ctx, EntityTypeProduct, []matching.AnchorQuery{
		{Field: "product_id", Value: p.ProductID},
	}, nil)
	if err != nil {
		return fmt.Errorf("find product: %w", err)
	}

	var op entitystore.BatchWriteOp
	if len(existing) > 0 {
		op = entitystore.BatchWriteOp{
			WriteEntity: &entitystore.WriteEntityOp{
				Action:          entitystore.WriteActionUpdate,
				MatchedEntityID: existing[0].ID,
				EntityType:      EntityTypeProduct,
				Data:            data,
				Confidence:      1.0,
				Tags:            p.Tags,
				Anchors: []matching.AnchorQuery{
					{Field: "product_id", Value: p.ProductID},
				},
			},
		}
	} else {
		op = entitystore.BatchWriteOp{
			WriteEntity: &entitystore.WriteEntityOp{
				Action:     entitystore.WriteActionCreate,
				EntityType: EntityTypeProduct,
				Data:       data,
				Confidence: 1.0,
				Tags:       p.Tags,
				Anchors: []matching.AnchorQuery{
					{Field: "product_id", Value: p.ProductID},
				},
			},
		}
	}

	_, err = s.es.BatchWrite(ctx, []entitystore.BatchWriteOp{op})
	return err
}

func (s *ESStore) GetProduct(ctx context.Context, productID string) (ProductEligibility, error) {
	entities, err := s.es.FindByAnchors(ctx, EntityTypeProduct, []matching.AnchorQuery{
		{Field: "product_id", Value: productID},
	}, nil)
	if err != nil {
		return ProductEligibility{}, fmt.Errorf("find product: %w", err)
	}
	if len(entities) == 0 {
		return ProductEligibility{}, fmt.Errorf("product %q not found", productID)
	}

	var p ProductEligibility
	if err := json.Unmarshal(entities[0].Data, &p); err != nil {
		return ProductEligibility{}, fmt.Errorf("unmarshal product: %w", err)
	}
	return p, nil
}

func (s *ESStore) ListProducts(ctx context.Context, filter TagFilter) ([]ProductEligibility, error) {
	var qf *matching.QueryFilter
	if len(filter.Tags) > 0 {
		qf = &matching.QueryFilter{Tags: filter.Tags}
	}

	entities, err := s.es.GetEntitiesByType(ctx, EntityTypeProduct, 1000, nil)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}

	var result []ProductEligibility
	for _, e := range entities {
		if qf != nil && !hasAllTags(e.Tags, qf.Tags) {
			continue
		}
		var p ProductEligibility
		if err := json.Unmarshal(e.Data, &p); err != nil {
			continue
		}
		if filter.Status != "" && p.Status != filter.Status {
			continue
		}
		result = append(result, p)
	}
	return result, nil
}

// ─── Rulesets ───

func (s *ESStore) PutRuleset(ctx context.Context, r BaseRuleset) error {
	data, err := entitystore.MarshalEntityData(r)
	if err != nil {
		return fmt.Errorf("marshal ruleset: %w", err)
	}

	existing, err := s.es.FindByAnchors(ctx, EntityTypeRuleset, []matching.AnchorQuery{
		{Field: "ruleset_id", Value: r.ID},
	}, nil)
	if err != nil {
		return fmt.Errorf("find ruleset: %w", err)
	}

	var op entitystore.BatchWriteOp
	if len(existing) > 0 {
		op = entitystore.BatchWriteOp{
			WriteEntity: &entitystore.WriteEntityOp{
				Action:          entitystore.WriteActionUpdate,
				MatchedEntityID: existing[0].ID,
				EntityType:      EntityTypeRuleset,
				Data:            data,
				Confidence:      1.0,
				Anchors: []matching.AnchorQuery{
					{Field: "ruleset_id", Value: r.ID},
				},
			},
		}
	} else {
		op = entitystore.BatchWriteOp{
			WriteEntity: &entitystore.WriteEntityOp{
				Action:     entitystore.WriteActionCreate,
				EntityType: EntityTypeRuleset,
				Data:       data,
				Confidence: 1.0,
				Anchors: []matching.AnchorQuery{
					{Field: "ruleset_id", Value: r.ID},
				},
			},
		}
	}

	_, err = s.es.BatchWrite(ctx, []entitystore.BatchWriteOp{op})
	return err
}

func (s *ESStore) GetRuleset(ctx context.Context, id string) (BaseRuleset, error) {
	entities, err := s.es.FindByAnchors(ctx, EntityTypeRuleset, []matching.AnchorQuery{
		{Field: "ruleset_id", Value: id},
	}, nil)
	if err != nil {
		return BaseRuleset{}, fmt.Errorf("find ruleset: %w", err)
	}
	if len(entities) == 0 {
		return BaseRuleset{}, fmt.Errorf("ruleset %q not found", id)
	}

	var r BaseRuleset
	if err := json.Unmarshal(entities[0].Data, &r); err != nil {
		return BaseRuleset{}, fmt.Errorf("unmarshal ruleset: %w", err)
	}
	return r, nil
}

func (s *ESStore) ListRulesets(ctx context.Context) ([]BaseRuleset, error) {
	entities, err := s.es.GetEntitiesByType(ctx, EntityTypeRuleset, 1000, nil)
	if err != nil {
		return nil, fmt.Errorf("list rulesets: %w", err)
	}

	result := make([]BaseRuleset, 0, len(entities))
	for _, e := range entities {
		var r BaseRuleset
		if err := json.Unmarshal(e.Data, &r); err != nil {
			continue
		}
		result = append(result, r)
	}
	return result, nil
}

// ─── Subscriptions ───

func (s *ESStore) PutSubscription(ctx context.Context, sub Subscription) error {
	data, err := entitystore.MarshalEntityData(sub)
	if err != nil {
		return fmt.Errorf("marshal subscription: %w", err)
	}

	existing, err := s.es.FindByAnchors(ctx, EntityTypeSubscription, []matching.AnchorQuery{
		{Field: "subscription_id", Value: sub.ID},
	}, nil)
	if err != nil {
		return fmt.Errorf("find subscription: %w", err)
	}

	var op entitystore.BatchWriteOp
	if len(existing) > 0 {
		op = entitystore.BatchWriteOp{
			WriteEntity: &entitystore.WriteEntityOp{
				Action:          entitystore.WriteActionUpdate,
				MatchedEntityID: existing[0].ID,
				EntityType:      EntityTypeSubscription,
				Data:            data,
				Confidence:      1.0,
				Anchors: []matching.AnchorQuery{
					{Field: "subscription_id", Value: sub.ID},
				},
			},
		}
	} else {
		op = entitystore.BatchWriteOp{
			WriteEntity: &entitystore.WriteEntityOp{
				Action:     entitystore.WriteActionCreate,
				EntityType: EntityTypeSubscription,
				Data:       data,
				Confidence: 1.0,
				Anchors: []matching.AnchorQuery{
					{Field: "subscription_id", Value: sub.ID},
				},
			},
		}
	}

	_, err = s.es.BatchWrite(ctx, []entitystore.BatchWriteOp{op})
	return err
}

func (s *ESStore) GetSubscription(ctx context.Context, id string) (Subscription, error) {
	entities, err := s.es.FindByAnchors(ctx, EntityTypeSubscription, []matching.AnchorQuery{
		{Field: "subscription_id", Value: id},
	}, nil)
	if err != nil {
		return Subscription{}, fmt.Errorf("find subscription: %w", err)
	}
	if len(entities) == 0 {
		return Subscription{}, fmt.Errorf("subscription %q not found", id)
	}

	var sub Subscription
	if err := json.Unmarshal(entities[0].Data, &sub); err != nil {
		return Subscription{}, fmt.Errorf("unmarshal subscription: %w", err)
	}
	return sub, nil
}

func (s *ESStore) ListSubscriptions(ctx context.Context, filter SubscriptionFilter) ([]Subscription, error) {
	entities, err := s.es.GetEntitiesByType(ctx, EntityTypeSubscription, 1000, nil)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}

	var result []Subscription
	for _, e := range entities {
		var sub Subscription
		if err := json.Unmarshal(e.Data, &sub); err != nil {
			continue
		}
		if filter.EntityID != "" && sub.EntityID != filter.EntityID {
			continue
		}
		if filter.Status != "" && sub.Status != filter.Status {
			continue
		}
		result = append(result, sub)
	}
	return result, nil
}
