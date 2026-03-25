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
// Accepts both *entitystore.EntityStore and *entitystore.ScopedStore.
// When backed by a ScopedStore, all reads are filtered and all creates
// are auto-tagged by the scope configuration (e.g. tenant, workspace).
type Store struct {
	es es.EntityStorer
}

// NewStore creates a new entitystore-backed Store.
// Pass *entitystore.EntityStore for unscoped access, or
// *entitystore.ScopedStore for tenant/workspace-scoped access.
func NewStore(e es.EntityStorer) *Store {
	return &Store{es: e}
}

// ─── Tag Helpers ───

// productTags builds entitystore tags for a product by merging user-defined tags
// with well-known system tags (status, entity type).
func productTags(p prodcat.Product) []string {
	set := tags.FromStrings(p.Tags)

	// Entity type tag.
	set, _ = set.With(tags.PrefixEntity, "product")

	// Status tag.
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

// rulesetTags builds entitystore tags for a ruleset using the tags lifecycle.
func rulesetTags(r prodcat.Ruleset) []string {
	set := tags.Set{}

	// Entity type tag.
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

// statusDisabled returns the "status:disabled" tag string for use in preconditions.
func statusDisabled() string {
	s, _ := tags.Build(tags.PrefixStatus, "disabled")
	return s
}

// ─── Products ───

// CreateProduct creates a new product, failing with ErrAlreadyExists if the ProductID is taken.
// Uses entitystore MustNotExist precondition for transactional uniqueness.
func (s *Store) CreateProduct(ctx context.Context, p prodcat.Product, prov prodcat.Provenance) error {
	pb := productToProto(p)
	cfg := prodcatv1.ProductMatchConfig()

	writeOp := prodcatv1.ProductWriteOp(pb, store.WriteActionCreate,
		store.WithTags(productTags(p)...),
	)

	// Build ruleset preconditions — each referenced ruleset must exist and not be disabled.
	rulesetPCs := rulesetPreConditions(p.BaseRulesetIDs)

	// Product must not already exist.
	pcs := append([]store.PreCondition{
		{
			EntityType:   cfg.EntityType,
			Anchors:      []matching.AnchorQuery{{Field: "product_id", Value: p.ProductID}},
			MustNotExist: true,
		},
	}, rulesetPCs...)

	_, err := s.es.BatchWrite(ctx, []store.BatchWriteOp{
		{
			WriteEntity:   writeOp,
			PreConditions: pcs,
		},
	})
	return mapPreConditionError(err)
}

// PutProduct upserts a product. When the product references rulesets, the store verifies
// each one exists and is not disabled via entitystore preconditions.
func (s *Store) PutProduct(ctx context.Context, p prodcat.Product, prov prodcat.Provenance) error {
	pb := productToProto(p)
	cfg := prodcatv1.ProductMatchConfig()
	rulesetPCs := rulesetPreConditions(p.BaseRulesetIDs)

	anchors := prodcatv1.ProductWriteOp(pb, store.WriteActionCreate).Anchors

	existing, err := s.es.FindByAnchors(ctx, cfg.EntityType, anchors, nil)
	if err != nil {
		return fmt.Errorf("find entity: %w", err)
	}

	allTags := productTags(p)

	var writeOp *store.WriteEntityOp
	if len(existing) > 0 {
		writeOp = prodcatv1.ProductWriteOp(pb, store.WriteActionUpdate,
			store.WithMatchedEntityID(existing[0].ID),
			store.WithTags(allTags...),
		)
	} else {
		writeOp = prodcatv1.ProductWriteOp(pb, store.WriteActionCreate,
			store.WithTags(allTags...),
		)
	}

	_, err = s.es.BatchWrite(ctx, []store.BatchWriteOp{
		{
			WriteEntity:   writeOp,
			PreConditions: rulesetPCs,
		},
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

// ─── Rulesets ───

// CreateRuleset creates a new ruleset, failing with ErrAlreadyExists if the ID is taken.
// Uses entitystore MustNotExist precondition for transactional uniqueness.
func (s *Store) CreateRuleset(ctx context.Context, r prodcat.Ruleset, prov prodcat.Provenance) error {
	pb := rulesetToProto(r)
	cfg := prodcatv1.RulesetMatchConfig()

	writeOp := prodcatv1.RulesetWriteOp(pb, store.WriteActionCreate,
		store.WithTags(rulesetTags(r)...),
	)

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

// PutRuleset upserts a ruleset. Uses tags lifecycle for status management:
// disabled rulesets get "status:disabled" + "disabled-reason:<reason>" tags.
func (s *Store) PutRuleset(ctx context.Context, r prodcat.Ruleset, prov prodcat.Provenance) error {
	pb := rulesetToProto(r)
	cfg := prodcatv1.RulesetMatchConfig()
	allTags := rulesetTags(r)

	anchors := prodcatv1.RulesetWriteOp(pb, store.WriteActionCreate).Anchors

	existing, err := s.es.FindByAnchors(ctx, cfg.EntityType, anchors, nil)
	if err != nil {
		return fmt.Errorf("find entity: %w", err)
	}

	var writeOp *store.WriteEntityOp
	if len(existing) > 0 {
		writeOp = prodcatv1.RulesetWriteOp(pb, store.WriteActionUpdate,
			store.WithMatchedEntityID(existing[0].ID),
			store.WithTags(allTags...),
		)
	} else {
		writeOp = prodcatv1.RulesetWriteOp(pb, store.WriteActionCreate,
			store.WithTags(allTags...),
		)
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

// rulesetPreConditions builds preconditions that verify each referenced ruleset
// exists and is not disabled. Uses the well-known "status:disabled" tag.
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

// mapPreConditionError translates entitystore PreConditionError into prodcat sentinel errors.
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

	// Store disabled state in proto status field for query support.
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
	// Separate user tags from system tags.
	set := tags.FromStrings(storedTags)
	userSet := set.Without(tags.PrefixStatus, "status_reason", tags.PrefixEntity)

	// Derive disabled state from tags.
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
