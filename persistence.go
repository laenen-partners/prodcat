package prodcat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/laenen-partners/entitystore/matching"
	"github.com/laenen-partners/entitystore/store"
	"github.com/laenen-partners/tags"
	"google.golang.org/protobuf/proto"

	prodcatv1 "github.com/laenen-partners/prodcat/gen/prodcat/v1"
)

// matchRegistry holds the generated match configs for all entity types.
var matchRegistry *matching.MatchConfigRegistry

func init() {
	matchRegistry = matching.NewMatchConfigRegistry()
	matchRegistry.Register(prodcatv1.ProductMatchConfig())
	matchRegistry.Register(prodcatv1.RulesetMatchConfig())
}

// ─── Tag Helpers ───

func productTags(p Product) []string {
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

func rulesetTags(r Ruleset) []string {
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
		case *ProductCreatedEvent:
			result = append(result, &prodcatv1.ProductCreated{
				ProductId: ev.ProductID, Actor: ev.Actor, Name: ev.Name,
				Description: ev.Description, Tags: ev.Tags,
				CurrencyCode: ev.CurrencyCode, BaseRulesetIds: ev.BaseRulesetIDs,
			})
		case *ProductUpdatedEvent:
			result = append(result, &prodcatv1.ProductUpdated{
				ProductId: ev.ProductID, Actor: ev.Actor, Name: ev.Name,
				Description: ev.Description, Tags: ev.Tags,
				CurrencyCode: ev.CurrencyCode, BaseRulesetIds: ev.BaseRulesetIDs,
			})
		case *ProductDisabledEvent:
			result = append(result, &prodcatv1.ProductDisabled{
				ProductId: ev.ProductID, Actor: ev.Actor, Reason: ev.Reason, Name: ev.Name,
			})
		case *ProductEnabledEvent:
			result = append(result, &prodcatv1.ProductEnabled{
				ProductId: ev.ProductID, Actor: ev.Actor, Name: ev.Name,
			})
		case *ProductDeletedEvent:
			result = append(result, &prodcatv1.ProductDeleted{
				ProductId: ev.ProductID, Actor: ev.Actor, Name: ev.Name,
			})
		case *RulesetCreatedEvent:
			result = append(result, &prodcatv1.RulesetCreated{
				RulesetId: ev.RulesetID, Actor: ev.Actor, Name: ev.Name,
				Description: ev.Description, Version: ev.Version,
			})
		case *RulesetUpdatedEvent:
			result = append(result, &prodcatv1.RulesetUpdated{
				RulesetId: ev.RulesetID, Actor: ev.Actor, Name: ev.Name,
				Description: ev.Description, Version: ev.Version,
			})
		case *RulesetDisabledEvent:
			result = append(result, &prodcatv1.RulesetDisabled{
				RulesetId: ev.RulesetID, Actor: ev.Actor, Reason: ev.Reason, Name: ev.Name,
			})
		case *RulesetEnabledEvent:
			result = append(result, &prodcatv1.RulesetEnabled{
				RulesetId: ev.RulesetID, Actor: ev.Actor, Name: ev.Name,
			})
		case *RulesetDeletedEvent:
			result = append(result, &prodcatv1.RulesetDeleted{
				RulesetId: ev.RulesetID, Actor: ev.Actor, Name: ev.Name,
			})
		case *RulesetLinkedToProductEvent:
			result = append(result, &prodcatv1.RulesetLinkedToProduct{
				ProductId: ev.ProductID, RulesetId: ev.RulesetID, Actor: ev.Actor,
				ProductName: ev.ProductName, RulesetName: ev.RulesetName,
			})
		case *RulesetUnlinkedFromProductEvent:
			result = append(result, &prodcatv1.RulesetUnlinkedFromProduct{
				ProductId: ev.ProductID, RulesetId: ev.RulesetID, Actor: ev.Actor,
				ProductName: ev.ProductName, RulesetName: ev.RulesetName,
			})
		case *CatalogImportedEvent:
			result = append(result, &prodcatv1.CatalogImported{
				Filename: ev.Filename, Actor: ev.Actor, FileHash: ev.FileHash,
				RulesetCount: int32(ev.RulesetCount), ProductCount: int32(ev.ProductCount),
				ProductIds: ev.ProductIDs, RulesetIds: ev.RulesetIDs,
			})
		}
	}
	return result
}

func eventOpts(events []any) []store.WriteOpOption {
	protoEvents := eventsToProto(events)
	if len(protoEvents) > 0 {
		return []store.WriteOpOption{store.WithEvents(protoEvents...)}
	}
	return nil
}

// ─── Products (persistence) ───

func (c *Client) createProduct(ctx context.Context, p Product, prov Provenance, events ...any) error {
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

	_, err := c.es.BatchWrite(ctx, []store.BatchWriteOp{
		{WriteEntity: writeOp, PreConditions: pcs},
	})
	return mapPreConditionError(err)
}

func (c *Client) putProduct(ctx context.Context, p Product, prov Provenance, events ...any) error {
	pb := productToProto(p)
	cfg := prodcatv1.ProductMatchConfig()
	rulesetPCs := rulesetPreConditions(p.BaseRulesetIDs)

	anchors := prodcatv1.ProductWriteOp(pb, store.WriteActionCreate).Anchors
	existing, err := c.es.FindByAnchors(ctx, cfg.EntityType, anchors, nil)
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

	_, err = c.es.BatchWrite(ctx, []store.BatchWriteOp{
		{WriteEntity: writeOp, PreConditions: rulesetPCs},
	})
	return mapPreConditionError(err)
}

func (c *Client) getProduct(ctx context.Context, productID string) (*Product, error) {
	cfg := prodcatv1.ProductMatchConfig()
	entity, err := c.getEntityByAnchor(ctx, cfg.EntityType, "product_id", productID)
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

func (c *Client) listProducts(ctx context.Context, filter ListFilter) ([]Product, error) {
	cfg := prodcatv1.ProductMatchConfig()
	entities, err := c.es.GetEntitiesByType(ctx, cfg.EntityType, 1000, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	var result []Product
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

func (c *Client) deleteProduct(ctx context.Context, productID string) error {
	cfg := prodcatv1.ProductMatchConfig()
	entity, err := c.getEntityByAnchor(ctx, cfg.EntityType, "product_id", productID)
	if err != nil {
		return err
	}
	return c.es.DeleteEntity(ctx, entity.ID)
}

// ─── Rulesets (persistence) ───

func (c *Client) createRuleset(ctx context.Context, r Ruleset, prov Provenance, events ...any) error {
	pb := rulesetToProto(r)
	cfg := prodcatv1.RulesetMatchConfig()

	opts := append([]store.WriteOpOption{store.WithTags(rulesetTags(r)...)}, eventOpts(events)...)
	writeOp := prodcatv1.RulesetWriteOp(pb, store.WriteActionCreate, opts...)

	_, err := c.es.BatchWrite(ctx, []store.BatchWriteOp{
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

func (c *Client) putRuleset(ctx context.Context, r Ruleset, prov Provenance, events ...any) error {
	pb := rulesetToProto(r)
	cfg := prodcatv1.RulesetMatchConfig()
	allTags := rulesetTags(r)

	anchors := prodcatv1.RulesetWriteOp(pb, store.WriteActionCreate).Anchors
	existing, err := c.es.FindByAnchors(ctx, cfg.EntityType, anchors, nil)
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

	_, err = c.es.BatchWrite(ctx, []store.BatchWriteOp{
		{WriteEntity: writeOp},
	})
	return err
}

func (c *Client) getRuleset(ctx context.Context, id string) (*Ruleset, error) {
	cfg := prodcatv1.RulesetMatchConfig()
	entity, err := c.getEntityByAnchor(ctx, cfg.EntityType, "ruleset_id", id)
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

func (c *Client) listRulesets(ctx context.Context) ([]Ruleset, error) {
	cfg := prodcatv1.RulesetMatchConfig()
	entities, err := c.es.GetEntitiesByType(ctx, cfg.EntityType, 1000, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("list rulesets: %w", err)
	}
	result := make([]Ruleset, 0, len(entities))
	for _, e := range entities {
		var pb prodcatv1.Ruleset
		if err := e.GetData(&pb); err != nil {
			continue
		}
		result = append(result, rulesetFromProto(&pb))
	}
	return result, nil
}

func (c *Client) deleteRuleset(ctx context.Context, id string) error {
	cfg := prodcatv1.RulesetMatchConfig()
	entity, err := c.getEntityByAnchor(ctx, cfg.EntityType, "ruleset_id", id)
	if err != nil {
		return err
	}
	return c.es.DeleteEntity(ctx, entity.ID)
}

// ─── Graph Relations ───

func (c *Client) linkRulesetToProduct(ctx context.Context, productID, rulesetID string) error {
	productEntity, err := c.getEntityByAnchor(ctx, prodcatv1.ProductMatchConfig().EntityType, "product_id", productID)
	if err != nil {
		return fmt.Errorf("find product: %w", err)
	}
	rulesetEntity, err := c.getEntityByAnchor(ctx, prodcatv1.RulesetMatchConfig().EntityType, "ruleset_id", rulesetID)
	if err != nil {
		return fmt.Errorf("find ruleset: %w", err)
	}
	_, err = c.es.BatchWrite(ctx, []store.BatchWriteOp{{
		UpsertRelation: &store.UpsertRelationOp{
			SourceID:     productEntity.ID,
			TargetID:     rulesetEntity.ID,
			RelationType: "has_ruleset",
			Confidence:   1.0,
		},
	}})
	return err
}

func (c *Client) unlinkRulesetFromProduct(ctx context.Context, productID, rulesetID string) error {
	productEntity, err := c.getEntityByAnchor(ctx, prodcatv1.ProductMatchConfig().EntityType, "product_id", productID)
	if err != nil {
		return fmt.Errorf("find product: %w", err)
	}
	rulesetEntity, err := c.getEntityByAnchor(ctx, prodcatv1.RulesetMatchConfig().EntityType, "ruleset_id", rulesetID)
	if err != nil {
		return fmt.Errorf("find ruleset: %w", err)
	}
	return c.es.DeleteRelationByKey(ctx, productEntity.ID, rulesetEntity.ID, "has_ruleset")
}

// ─── Import (persistence) ───

func (c *Client) importCatalogBatch(ctx context.Context, rulesets []Ruleset, products []Product, prov Provenance, onConflict OnConflict, importEvent *CatalogImportedEvent, actor string) error {
	var ops []store.BatchWriteOp

	rulesetCfg := prodcatv1.RulesetMatchConfig()
	for _, r := range rulesets {
		pb := rulesetToProto(r)
		allTags := rulesetTags(r)

		switch onConflict {
		case OnConflictError:
			event := &prodcatv1.RulesetCreated{
				RulesetId: r.ID, Actor: actor, Name: r.Name,
				Description: r.Description, Version: r.Version,
			}
			opts := []store.WriteOpOption{
				store.WithTags(allTags...),
				store.WithEvents(event),
			}
			writeOp := prodcatv1.RulesetWriteOp(pb, store.WriteActionCreate, opts...)
			ops = append(ops, store.BatchWriteOp{
				WriteEntity: writeOp,
				PreConditions: []store.PreCondition{{
					EntityType:   rulesetCfg.EntityType,
					Anchors:      []matching.AnchorQuery{{Field: "ruleset_id", Value: r.ID}},
					MustNotExist: true,
				}},
			})
		default:
			anchors := prodcatv1.RulesetWriteOp(pb, store.WriteActionCreate).Anchors
			existing, err := c.es.FindByAnchors(ctx, rulesetCfg.EntityType, anchors, nil)
			if err != nil {
				return fmt.Errorf("find ruleset %s: %w", r.ID, err)
			}
			opts := []store.WriteOpOption{store.WithTags(allTags...)}
			var writeOp *store.WriteEntityOp
			if len(existing) > 0 {
				event := &prodcatv1.RulesetUpdated{
					RulesetId: r.ID, Actor: actor, Name: r.Name,
					Description: r.Description, Version: r.Version,
				}
				opts = append(opts, store.WithMatchedEntityID(existing[0].ID), store.WithEvents(event))
				writeOp = prodcatv1.RulesetWriteOp(pb, store.WriteActionUpdate, opts...)
			} else {
				event := &prodcatv1.RulesetCreated{
					RulesetId: r.ID, Actor: actor, Name: r.Name,
					Description: r.Description, Version: r.Version,
				}
				opts = append(opts, store.WithEvents(event))
				writeOp = prodcatv1.RulesetWriteOp(pb, store.WriteActionCreate, opts...)
			}
			ops = append(ops, store.BatchWriteOp{WriteEntity: writeOp})
		}
	}

	productCfg := prodcatv1.ProductMatchConfig()
	for _, p := range products {
		pb := productToProto(p)
		allTags := productTags(p)

		switch onConflict {
		case OnConflictError:
			event := &prodcatv1.ProductCreated{
				ProductId: p.ProductID, Actor: actor, Name: p.Name,
				Description: p.Description, Tags: p.Tags,
				CurrencyCode: p.CurrencyCode, BaseRulesetIds: p.BaseRulesetIDs,
			}
			opts := []store.WriteOpOption{
				store.WithTags(allTags...),
				store.WithEvents(event),
			}
			writeOp := prodcatv1.ProductWriteOp(pb, store.WriteActionCreate, opts...)
			ops = append(ops, store.BatchWriteOp{
				WriteEntity: writeOp,
				PreConditions: []store.PreCondition{{
					EntityType:   productCfg.EntityType,
					Anchors:      []matching.AnchorQuery{{Field: "product_id", Value: p.ProductID}},
					MustNotExist: true,
				}},
			})
		default:
			anchors := prodcatv1.ProductWriteOp(pb, store.WriteActionCreate).Anchors
			existing, err := c.es.FindByAnchors(ctx, productCfg.EntityType, anchors, nil)
			if err != nil {
				return fmt.Errorf("find product %s: %w", p.ProductID, err)
			}
			opts := []store.WriteOpOption{store.WithTags(allTags...)}
			var writeOp *store.WriteEntityOp
			if len(existing) > 0 {
				event := &prodcatv1.ProductUpdated{
					ProductId: p.ProductID, Actor: actor, Name: p.Name,
					Description: p.Description, Tags: p.Tags,
					CurrencyCode: p.CurrencyCode, BaseRulesetIds: p.BaseRulesetIDs,
				}
				opts = append(opts, store.WithMatchedEntityID(existing[0].ID), store.WithEvents(event))
				writeOp = prodcatv1.ProductWriteOp(pb, store.WriteActionUpdate, opts...)
			} else {
				event := &prodcatv1.ProductCreated{
					ProductId: p.ProductID, Actor: actor, Name: p.Name,
					Description: p.Description, Tags: p.Tags,
					CurrencyCode: p.CurrencyCode, BaseRulesetIds: p.BaseRulesetIDs,
				}
				opts = append(opts, store.WithEvents(event))
				writeOp = prodcatv1.ProductWriteOp(pb, store.WriteActionCreate, opts...)
			}
			ops = append(ops, store.BatchWriteOp{WriteEntity: writeOp})
		}
	}

	if len(ops) == 0 {
		return nil
	}

	// Emit the catalog-level import event on the first op.
	if importEvent != nil {
		importProto := eventsToProto([]any{importEvent})
		if len(importProto) > 0 {
			ops[0].WriteEntity.Events = append(ops[0].WriteEntity.Events, importProto...)
		}
	}

	results, err := c.es.BatchWrite(ctx, ops)
	if err != nil {
		return mapPreConditionError(err)
	}

	// Create graph relations for product→ruleset links.
	rulesetOffset := len(rulesets)
	for i, p := range products {
		if len(p.BaseRulesetIDs) == 0 {
			continue
		}
		productEntityID := results[rulesetOffset+i].Entity.ID
		for _, rsID := range p.BaseRulesetIDs {
			for j, r := range rulesets {
				if r.ID == rsID {
					rulesetEntityID := results[j].Entity.ID
					_, linkErr := c.es.BatchWrite(ctx, []store.BatchWriteOp{{
						UpsertRelation: &store.UpsertRelationOp{
							SourceID:     productEntityID,
							TargetID:     rulesetEntityID,
							RelationType: "has_ruleset",
							Confidence:   1.0,
						},
					}})
					if linkErr != nil {
						return fmt.Errorf("link product %s to ruleset %s: %w", p.ProductID, rsID, linkErr)
					}
					break
				}
			}
		}
	}

	return nil
}

// ─── Generic helpers ───

func (c *Client) getEntityByAnchor(ctx context.Context, entityType, field, value string) (matching.StoredEntity, error) {
	cfg, _ := matchRegistry.Get(entityType)
	if cfg.Normalizers != nil {
		if fn, ok := cfg.Normalizers[field]; ok && fn != nil {
			value = fn(value)
		}
	}
	entities, err := c.es.FindByAnchors(ctx, entityType, []matching.AnchorQuery{
		{Field: field, Value: value},
	}, nil)
	if err != nil {
		return matching.StoredEntity{}, fmt.Errorf("find %s: %w", entityType, err)
	}
	if len(entities) == 0 {
		return matching.StoredEntity{}, fmt.Errorf("%s %q: %w", field, value, ErrNotFound)
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
			return fmt.Errorf("%s: %w", pcErr.Condition.EntityType, ErrNotFound)
		case "already_exists":
			return fmt.Errorf("%s: %w", pcErr.Condition.EntityType, ErrAlreadyExists)
		case "tag_forbidden":
			return fmt.Errorf("%s: %w", pcErr.Condition.EntityType, ErrRulesetDisabled)
		default:
			return fmt.Errorf("precondition failed: %w", err)
		}
	}
	return err
}

// ─── Domain <-> Proto conversions ───

func productToProto(p Product) *prodcatv1.Product {
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

func productFromProto(pb *prodcatv1.Product, storedTags []string) Product {
	set := tags.FromStrings(storedTags)
	userSet := set.Without(tags.PrefixStatus, "status_reason", tags.PrefixEntity)
	statusVal, _ := set.Get(tags.PrefixStatus)
	disabled := statusVal == "disabled"
	var disabledReason DisabledReason
	if disabled {
		if reason, ok := set.Get("status_reason"); ok {
			disabledReason = DisabledReason(reason)
		}
	}
	p := Product{
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
			p.Availability.Mode = AvailabilityMode(mode)
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

func rulesetToProto(r Ruleset) *prodcatv1.Ruleset {
	return &prodcatv1.Ruleset{
		RulesetId:      r.ID,
		Name:           r.Name,
		Description:    r.Description,
		Version:        r.Version,
		Content:        string(r.Content),
		ContentHash:    r.ContentHash,
		Disabled:       r.Disabled,
		DisabledReason: string(r.DisabledReason),
	}
}

func rulesetFromProto(pb *prodcatv1.Ruleset) Ruleset {
	return Ruleset{
		ID:             pb.RulesetId,
		Name:           pb.Name,
		Description:    pb.Description,
		Version:        pb.Version,
		Content:        []byte(pb.Content),
		ContentHash:    pb.ContentHash,
		Disabled:       pb.Disabled,
		DisabledReason: DisabledReason(pb.DisabledReason),
	}
}
