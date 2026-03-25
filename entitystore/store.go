// Package entitystore provides an entitystore-backed implementation of prodcat.Store.
package entitystore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	es "github.com/laenen-partners/entitystore"
	"github.com/laenen-partners/entitystore/matching"
	"github.com/laenen-partners/entitystore/store"
	"github.com/laenen-partners/tags"
	"google.golang.org/protobuf/proto"

	"github.com/laenen-partners/prodcat"
	prodcatv1 "github.com/laenen-partners/prodcat/gen/prodcat/v1"
)

// matchRegistry holds the generated match configs for all entity types.
var matchRegistry *matching.MatchConfigRegistry

func init() {
	matchRegistry = matching.NewMatchConfigRegistry()
	matchRegistry.Register(prodcatv1.ProductMatchConfig())
	matchRegistry.Register(prodcatv1.RulesetMatchConfig())
	matchRegistry.Register(prodcatv1.ImportRecordMatchConfig())
}

// Store implements prodcat.Store backed by entitystore.
type Store struct {
	es es.EntityStorer
}

// NewStore creates a new entitystore-backed Store.
func NewStore(e es.EntityStorer) *Store {
	return &Store{es: e}
}

// ─── Tag Helpers ───

func productTags(p prodcat.Product) []string {
	set := tags.FromStrings(p.Tags)
	set, _ = set.With(tags.PrefixEntity, "product")
	if p.Disabled {
		set, _ = set.With(tags.PrefixStatus, "disabled")
		if p.DisabledReason != "" {
			set, _ = set.With("status_reason", string(p.DisabledReason))
		}
	} else {
		set, _ = set.With(tags.PrefixStatus, "active")
	}
	return set.Strings()
}

func rulesetTags(r prodcat.Ruleset) []string {
	set := tags.Set{}
	set, _ = set.With(tags.PrefixEntity, "ruleset")
	if r.Disabled {
		set, _ = set.With(tags.PrefixStatus, "disabled")
		if r.DisabledReason != "" {
			set, _ = set.With("status_reason", string(r.DisabledReason))
		}
	} else {
		set, _ = set.With(tags.PrefixStatus, "active")
	}
	return set.Strings()
}

func statusDisabled() string {
	s, _ := tags.Build(tags.PrefixStatus, "disabled")
	return s
}

// ─── Event Conversion ───

func eventsToProto(events []any) []proto.Message {
	var result []proto.Message
	for _, e := range events {
		switch ev := e.(type) {
		case *prodcat.ProductCreatedEvent:
			result = append(result, &prodcatv1.ProductCreated{
				ProductId: ev.ProductID, Actor: ev.Actor, Name: ev.Name,
				Description: ev.Description, Tags: ev.Tags,
				CurrencyCode: ev.CurrencyCode, BaseRulesetIds: ev.BaseRulesetIDs,
			})
		case *prodcat.ProductUpdatedEvent:
			result = append(result, &prodcatv1.ProductUpdated{
				ProductId: ev.ProductID, Actor: ev.Actor, Name: ev.Name,
				Description: ev.Description, Tags: ev.Tags,
				CurrencyCode: ev.CurrencyCode, BaseRulesetIds: ev.BaseRulesetIDs,
			})
		case *prodcat.ProductDisabledEvent:
			result = append(result, &prodcatv1.ProductDisabled{
				ProductId: ev.ProductID, Actor: ev.Actor, Reason: ev.Reason, Name: ev.Name,
			})
		case *prodcat.ProductEnabledEvent:
			result = append(result, &prodcatv1.ProductEnabled{
				ProductId: ev.ProductID, Actor: ev.Actor, Name: ev.Name,
			})
		case *prodcat.ProductDeletedEvent:
			result = append(result, &prodcatv1.ProductDeleted{
				ProductId: ev.ProductID, Actor: ev.Actor, Name: ev.Name,
			})
		case *prodcat.RulesetCreatedEvent:
			result = append(result, &prodcatv1.RulesetCreated{
				RulesetId: ev.RulesetID, Actor: ev.Actor, Name: ev.Name,
				Description: ev.Description, Version: ev.Version,
			})
		case *prodcat.RulesetDisabledEvent:
			result = append(result, &prodcatv1.RulesetDisabled{
				RulesetId: ev.RulesetID, Actor: ev.Actor, Reason: ev.Reason, Name: ev.Name,
			})
		case *prodcat.RulesetEnabledEvent:
			result = append(result, &prodcatv1.RulesetEnabled{
				RulesetId: ev.RulesetID, Actor: ev.Actor, Name: ev.Name,
			})
		case *prodcat.RulesetDeletedEvent:
			result = append(result, &prodcatv1.RulesetDeleted{
				RulesetId: ev.RulesetID, Actor: ev.Actor, Name: ev.Name,
			})
		case *prodcat.RulesetLinkedToProductEvent:
			result = append(result, &prodcatv1.RulesetLinkedToProduct{
				ProductId: ev.ProductID, RulesetId: ev.RulesetID, Actor: ev.Actor,
				ProductName: ev.ProductName, RulesetName: ev.RulesetName,
			})
		case *prodcat.RulesetUnlinkedFromProductEvent:
			result = append(result, &prodcatv1.RulesetUnlinkedFromProduct{
				ProductId: ev.ProductID, RulesetId: ev.RulesetID, Actor: ev.Actor,
				ProductName: ev.ProductName, RulesetName: ev.RulesetName,
			})
		case *prodcat.CatalogImportedEvent:
			result = append(result, &prodcatv1.CatalogImported{
				Filename: ev.Filename, Actor: ev.Actor, FileHash: ev.FileHash,
				RulesetCount: int32(ev.RulesetCount), ProductCount: int32(ev.ProductCount),
				ProductIds: ev.ProductIDs, RulesetIds: ev.RulesetIDs,
			})
		}
	}
	return result
}

// eventOpts converts domain events to a WriteOpOption slice.
func eventOpts(events []any) []store.WriteOpOption {
	protoEvents := eventsToProto(events)
	if len(protoEvents) > 0 {
		return []store.WriteOpOption{store.WithEvents(protoEvents...)}
	}
	return nil
}

// ─── Products ───

func (s *Store) CreateProduct(ctx context.Context, p prodcat.Product, prov prodcat.Provenance, events ...any) error {
	pb := productToProto(p)
	cfg := prodcatv1.ProductMatchConfig()

	opts := append([]store.WriteOpOption{store.WithTags(productTags(p)...)}, eventOpts(events)...)
	writeOp := prodcatv1.ProductWriteOp(pb, store.WriteActionCreate, opts...)

	rulesetPCs := rulesetPreConditions(p.BaseRulesetIDs)
	pcs := append([]store.PreCondition{
		{
			EntityType:   cfg.EntityType,
			Anchors:      []matching.AnchorQuery{{Field: "product_id", Value: p.ProductID}},
			MustNotExist: true,
		},
	}, rulesetPCs...)

	_, err := s.es.BatchWrite(ctx, []store.BatchWriteOp{
		{WriteEntity: writeOp, PreConditions: pcs},
	})
	return mapPreConditionError(err)
}

func (s *Store) PutProduct(ctx context.Context, p prodcat.Product, prov prodcat.Provenance, events ...any) error {
	pb := productToProto(p)
	cfg := prodcatv1.ProductMatchConfig()
	rulesetPCs := rulesetPreConditions(p.BaseRulesetIDs)

	anchors := prodcatv1.ProductWriteOp(pb, store.WriteActionCreate).Anchors
	existing, err := s.es.FindByAnchors(ctx, cfg.EntityType, anchors, nil)
	if err != nil {
		return fmt.Errorf("find entity: %w", err)
	}

	allTags := productTags(p)
	opts := append([]store.WriteOpOption{store.WithTags(allTags...)}, eventOpts(events)...)

	var writeOp *store.WriteEntityOp
	if len(existing) > 0 {
		opts = append(opts, store.WithMatchedEntityID(existing[0].ID))
		writeOp = prodcatv1.ProductWriteOp(pb, store.WriteActionUpdate, opts...)
	} else {
		writeOp = prodcatv1.ProductWriteOp(pb, store.WriteActionCreate, opts...)
	}

	_, err = s.es.BatchWrite(ctx, []store.BatchWriteOp{
		{WriteEntity: writeOp, PreConditions: rulesetPCs},
	})
	return mapPreConditionError(err)
}

func (s *Store) GetProduct(ctx context.Context, productID string) (*prodcat.Product, error) {
	cfg := prodcatv1.ProductMatchConfig()
	entity, err := s.getEntityByAnchor(ctx, cfg.EntityType, "product_id", productID)
	if err != nil {
		return nil, err
	}
	var pb prodcatv1.Product
	if err := entity.GetData(&pb); err != nil {
		return nil, fmt.Errorf("unmarshal product: %w", err)
	}
	p := productFromProto(&pb, entity.Tags)
	return &p, nil
}

func (s *Store) ListProducts(ctx context.Context, filter prodcat.ListFilter) ([]prodcat.Product, error) {
	cfg := prodcatv1.ProductMatchConfig()
	entities, err := s.es.GetEntitiesByType(ctx, cfg.EntityType, 1000, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	var result []prodcat.Product
	for _, e := range entities {
		set := tags.FromStrings(e.Tags)
		if len(filter.Tags) > 0 {
			required := tags.FromStrings(filter.Tags)
			if !set.HasAll(required) {
				continue
			}
		}
		var pb prodcatv1.Product
		if err := e.GetData(&pb); err != nil {
			continue
		}
		result = append(result, productFromProto(&pb, e.Tags))
	}
	return result, nil
}

func (s *Store) DeleteProduct(ctx context.Context, productID string) error {
	cfg := prodcatv1.ProductMatchConfig()
	entity, err := s.getEntityByAnchor(ctx, cfg.EntityType, "product_id", productID)
	if err != nil {
		return err
	}
	return s.es.DeleteEntity(ctx, entity.ID)
}

// ─── Rulesets ───

func (s *Store) CreateRuleset(ctx context.Context, r prodcat.Ruleset, prov prodcat.Provenance, events ...any) error {
	pb := rulesetToProto(r)
	cfg := prodcatv1.RulesetMatchConfig()

	opts := append([]store.WriteOpOption{store.WithTags(rulesetTags(r)...)}, eventOpts(events)...)
	writeOp := prodcatv1.RulesetWriteOp(pb, store.WriteActionCreate, opts...)

	_, err := s.es.BatchWrite(ctx, []store.BatchWriteOp{
		{
			WriteEntity: writeOp,
			PreConditions: []store.PreCondition{
				{
					EntityType:   cfg.EntityType,
					Anchors:      []matching.AnchorQuery{{Field: "ruleset_id", Value: r.ID}},
					MustNotExist: true,
				},
			},
		},
	})
	return mapPreConditionError(err)
}

func (s *Store) PutRuleset(ctx context.Context, r prodcat.Ruleset, prov prodcat.Provenance, events ...any) error {
	pb := rulesetToProto(r)
	cfg := prodcatv1.RulesetMatchConfig()
	allTags := rulesetTags(r)

	anchors := prodcatv1.RulesetWriteOp(pb, store.WriteActionCreate).Anchors
	existing, err := s.es.FindByAnchors(ctx, cfg.EntityType, anchors, nil)
	if err != nil {
		return fmt.Errorf("find entity: %w", err)
	}

	opts := append([]store.WriteOpOption{store.WithTags(allTags...)}, eventOpts(events)...)

	var writeOp *store.WriteEntityOp
	if len(existing) > 0 {
		opts = append(opts, store.WithMatchedEntityID(existing[0].ID))
		writeOp = prodcatv1.RulesetWriteOp(pb, store.WriteActionUpdate, opts...)
	} else {
		writeOp = prodcatv1.RulesetWriteOp(pb, store.WriteActionCreate, opts...)
	}

	_, err = s.es.BatchWrite(ctx, []store.BatchWriteOp{
		{WriteEntity: writeOp},
	})
	return err
}

func (s *Store) GetRuleset(ctx context.Context, id string) (*prodcat.Ruleset, error) {
	cfg := prodcatv1.RulesetMatchConfig()
	entity, err := s.getEntityByAnchor(ctx, cfg.EntityType, "ruleset_id", id)
	if err != nil {
		return nil, err
	}
	var pb prodcatv1.Ruleset
	if err := entity.GetData(&pb); err != nil {
		return nil, fmt.Errorf("unmarshal ruleset: %w", err)
	}
	r := rulesetFromProto(&pb)
	return &r, nil
}

func (s *Store) ListRulesets(ctx context.Context) ([]prodcat.Ruleset, error) {
	cfg := prodcatv1.RulesetMatchConfig()
	entities, err := s.es.GetEntitiesByType(ctx, cfg.EntityType, 1000, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("list rulesets: %w", err)
	}
	result := make([]prodcat.Ruleset, 0, len(entities))
	for _, e := range entities {
		var pb prodcatv1.Ruleset
		if err := e.GetData(&pb); err != nil {
			continue
		}
		result = append(result, rulesetFromProto(&pb))
	}
	return result, nil
}

func (s *Store) DeleteRuleset(ctx context.Context, id string) error {
	cfg := prodcatv1.RulesetMatchConfig()
	entity, err := s.getEntityByAnchor(ctx, cfg.EntityType, "ruleset_id", id)
	if err != nil {
		return err
	}
	return s.es.DeleteEntity(ctx, entity.ID)
}

// ─── Graph Relations ───

func (s *Store) LinkRulesetToProduct(ctx context.Context, productID, rulesetID string) error {
	productEntity, err := s.getEntityByAnchor(ctx, prodcatv1.ProductMatchConfig().EntityType, "product_id", productID)
	if err != nil {
		return fmt.Errorf("find product: %w", err)
	}
	rulesetEntity, err := s.getEntityByAnchor(ctx, prodcatv1.RulesetMatchConfig().EntityType, "ruleset_id", rulesetID)
	if err != nil {
		return fmt.Errorf("find ruleset: %w", err)
	}
	_, err = s.es.BatchWrite(ctx, []store.BatchWriteOp{{
		UpsertRelation: &store.UpsertRelationOp{
			SourceID:     productEntity.ID,
			TargetID:     rulesetEntity.ID,
			RelationType: "has_ruleset",
			Confidence:   1.0,
		},
	}})
	return err
}

func (s *Store) UnlinkRulesetFromProduct(ctx context.Context, productID, rulesetID string) error {
	productEntity, err := s.getEntityByAnchor(ctx, prodcatv1.ProductMatchConfig().EntityType, "product_id", productID)
	if err != nil {
		return fmt.Errorf("find product: %w", err)
	}
	rulesetEntity, err := s.getEntityByAnchor(ctx, prodcatv1.RulesetMatchConfig().EntityType, "ruleset_id", rulesetID)
	if err != nil {
		return fmt.Errorf("find ruleset: %w", err)
	}
	return s.es.DeleteRelationByKey(ctx, productEntity.ID, rulesetEntity.ID, "has_ruleset")
}

// ─── Generic helpers ───

func (s *Store) getEntityByAnchor(ctx context.Context, entityType, field, value string) (matching.StoredEntity, error) {
	cfg, _ := matchRegistry.Get(entityType)
	if cfg.Normalizers != nil {
		if fn, ok := cfg.Normalizers[field]; ok && fn != nil {
			value = fn(value)
		}
	}
	entities, err := s.es.FindByAnchors(ctx, entityType, []matching.AnchorQuery{
		{Field: field, Value: value},
	}, nil)
	if err != nil {
		return matching.StoredEntity{}, fmt.Errorf("find %s: %w", entityType, err)
	}
	if len(entities) == 0 {
		return matching.StoredEntity{}, fmt.Errorf("%s %q: %w", field, value, prodcat.ErrNotFound)
	}
	return entities[0], nil
}

func rulesetPreConditions(rulesetIDs []string) []store.PreCondition {
	if len(rulesetIDs) == 0 {
		return nil
	}
	rulesetCfg := prodcatv1.RulesetMatchConfig()
	disabledTag := statusDisabled()
	pcs := make([]store.PreCondition, len(rulesetIDs))
	for i, id := range rulesetIDs {
		pcs[i] = store.PreCondition{
			EntityType:   rulesetCfg.EntityType,
			Anchors:      []matching.AnchorQuery{{Field: "ruleset_id", Value: id}},
			MustExist:    true,
			TagForbidden: disabledTag,
		}
	}
	return pcs
}

func mapPreConditionError(err error) error {
	if err == nil {
		return nil
	}
	var pcErr *store.PreConditionError
	if errors.As(err, &pcErr) {
		switch pcErr.Violation {
		case "not_found":
			return fmt.Errorf("%s: %w", pcErr.Condition.EntityType, prodcat.ErrNotFound)
		case "already_exists":
			return fmt.Errorf("%s: %w", pcErr.Condition.EntityType, prodcat.ErrAlreadyExists)
		case "tag_forbidden":
			return fmt.Errorf("%s: %w", pcErr.Condition.EntityType, prodcat.ErrRulesetDisabled)
		default:
			return fmt.Errorf("precondition failed: %w", err)
		}
	}
	return err
}

// ─── Domain <-> Proto conversions ───

func productToProto(p prodcat.Product) *prodcatv1.Product {
	meta := make(map[string]string)
	if p.CurrencyCode != "" {
		meta["currency_code"] = p.CurrencyCode
	}
	if p.Availability.Mode != "" {
		meta["availability_mode"] = string(p.Availability.Mode)
	}
	if len(p.Availability.CountryCodes) > 0 {
		meta["country_codes"] = strings.Join(p.Availability.CountryCodes, ",")
	}
	if len(p.Ruleset) > 0 {
		meta["ruleset"] = string(p.Ruleset)
	}
	status := "active"
	if p.Disabled {
		status = "disabled"
	}
	return &prodcatv1.Product{
		ProductId:       p.ProductID,
		Name:            p.Name,
		Description:     p.Description,
		Tags:            p.Tags,
		Status:          status,
		ParentProductId: p.ParentProductID,
		RulesetIds:      p.BaseRulesetIDs,
		Meta:            meta,
	}
}

func productFromProto(pb *prodcatv1.Product, storedTags []string) prodcat.Product {
	set := tags.FromStrings(storedTags)
	userSet := set.Without(tags.PrefixStatus, "status_reason", tags.PrefixEntity)
	statusVal, _ := set.Get(tags.PrefixStatus)
	disabled := statusVal == "disabled"
	var disabledReason prodcat.DisabledReason
	if disabled {
		if reason, ok := set.Get("status_reason"); ok {
			disabledReason = prodcat.DisabledReason(reason)
		}
	}
	p := prodcat.Product{
		ProductID:       pb.ProductId,
		Name:            pb.Name,
		Description:     pb.Description,
		Tags:            userSet.Strings(),
		Disabled:        disabled,
		DisabledReason:  disabledReason,
		ParentProductID: pb.ParentProductId,
		BaseRulesetIDs:  pb.RulesetIds,
	}
	if pb.Meta != nil {
		p.CurrencyCode = pb.Meta["currency_code"]
		if mode, ok := pb.Meta["availability_mode"]; ok {
			p.Availability.Mode = prodcat.AvailabilityMode(mode)
		}
		if codes, ok := pb.Meta["country_codes"]; ok && codes != "" {
			p.Availability.CountryCodes = strings.Split(codes, ",")
		}
		if rs, ok := pb.Meta["ruleset"]; ok {
			p.Ruleset = []byte(rs)
		}
	}
	return p
}

func rulesetToProto(r prodcat.Ruleset) *prodcatv1.Ruleset {
	return &prodcatv1.Ruleset{
		RulesetId:      r.ID,
		Name:           r.Name,
		Description:    r.Description,
		Version:        r.Version,
		Content:        string(r.Content),
		Disabled:       r.Disabled,
		DisabledReason: string(r.DisabledReason),
	}
}

func rulesetFromProto(pb *prodcatv1.Ruleset) prodcat.Ruleset {
	return prodcat.Ruleset{
		ID:             pb.RulesetId,
		Name:           pb.Name,
		Description:    pb.Description,
		Version:        pb.Version,
		Content:        []byte(pb.Content),
		Disabled:       pb.Disabled,
		DisabledReason: prodcat.DisabledReason(pb.DisabledReason),
	}
}
