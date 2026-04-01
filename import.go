package prodcat

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// OnConflict controls how Import handles pre-existing rulesets and products.
type OnConflict int

const (
	// OnConflictError returns ErrAlreadyExists when a ruleset or product already exists.
	OnConflictError OnConflict = iota
	// OnConflictUpdate upserts rulesets and products (default for backwards compatibility).
	OnConflictUpdate
)

// ImportOption configures an Import call.
type ImportOption func(*importConfig)

type importConfig struct {
	onConflict OnConflict
}

// WithOnConflict sets the conflict strategy for Import.
func WithOnConflict(oc OnConflict) ImportOption {
	return func(cfg *importConfig) { cfg.onConflict = oc }
}

// CatalogDefinition is the top-level structure of a catalogue definition YAML file.
type CatalogDefinition struct {
	Kind     string           `yaml:"kind"`
	Version  string           `yaml:"version"`
	Rulesets []CatalogRuleset `yaml:"rulesets"`
	Products []CatalogProduct `yaml:"products"`
}

// CatalogRuleset is a ruleset definition in a catalogue file.
type CatalogRuleset struct {
	ID          string              `yaml:"id"`
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Version     string              `yaml:"version"`
	Evaluations []CatalogEvaluation `yaml:"evaluations"`
}

// CatalogProduct is a product definition in a catalogue file.
type CatalogProduct struct {
	ProductID         string            `yaml:"product_id"`
	Name              string            `yaml:"name"`
	Description       string            `yaml:"description"`
	Tags              []string          `yaml:"tags"`
	CurrencyCode      string            `yaml:"currency_code,omitempty"`
	Availability      CatalogGeo        `yaml:"availability"`
	BaseRulesetIDs    []string          `yaml:"base_ruleset_ids"`
	Routing           map[string]string `yaml:"routing,omitempty"`
	MultiSubscription bool              `yaml:"multi_subscription,omitempty"`
}

// CatalogEvaluation is a single evaluation rule in a catalogue ruleset.
type CatalogEvaluation = evaluationYAML

// CatalogGeo is geographic availability in a catalogue file.
type CatalogGeo struct {
	Mode         string   `yaml:"mode"`
	CountryCodes []string `yaml:"country_codes,omitempty"`
}

// Import parses and applies a catalogue definition file atomically.
// All rulesets and products are written in a single transaction.
// By default (OnConflictUpdate), rulesets and products are upserted.
// Use WithOnConflict(OnConflictError) to fail on duplicates instead.
func (c *Client) Import(ctx context.Context, filename string, data []byte, opts ...ImportOption) error {
	cfg := importConfig{onConflict: OnConflictUpdate}
	for _, o := range opts {
		o(&cfg)
	}

	var def CatalogDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return fmt.Errorf("parse catalogue definition %s: %w", filename, err)
	}

	if err := ValidateCatalogDefinition(&def); err != nil {
		return fmt.Errorf("validate %s: %w", filename, err)
	}

	fileHash := fmt.Sprintf("%x", sha256.Sum256(data))
	now := time.Now().UTC()

	rulesets := make([]Ruleset, 0, len(def.Rulesets))
	for _, rs := range def.Rulesets {
		content, err := marshalRulesetContent(rs.Evaluations)
		if err != nil {
			return fmt.Errorf("marshal ruleset %s: %w", rs.ID, err)
		}
		rulesets = append(rulesets, Ruleset{
			ID:          rs.ID,
			Name:        rs.Name,
			Description: rs.Description,
			Content:     content,
			ContentHash: ContentHashOf(content),
			Version:     rs.Version,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	products := make([]Product, 0, len(def.Products))
	for _, p := range def.Products {
		products = append(products, Product{
			ProductID:    p.ProductID,
			Name:         p.Name,
			Description:  p.Description,
			Tags:         p.Tags,
			CurrencyCode: p.CurrencyCode,
			Availability: GeoAvailability{
				Mode:         AvailabilityMode(p.Availability.Mode),
				CountryCodes: p.Availability.CountryCodes,
			},
			BaseRulesetIDs:    p.BaseRulesetIDs,
			Routing:           p.Routing,
			MultiSubscription: p.MultiSubscription,
			CreatedAt:         now,
			UpdatedAt:         now,
		})
	}

	actor := actorFromContext(ctx)
	rulesetIDs := make([]string, len(rulesets))
	for i, r := range rulesets {
		rulesetIDs[i] = r.ID
	}
	productIDs := make([]string, len(products))
	for i, p := range products {
		productIDs[i] = p.ProductID
	}

	importEvent := &CatalogImportedEvent{
		Filename:     filename,
		Actor:        actor,
		FileHash:     fileHash,
		RulesetCount: len(rulesets),
		ProductCount: len(products),
		RulesetIDs:   rulesetIDs,
		ProductIDs:   productIDs,
	}

	if err := c.importCatalogBatch(ctx, rulesets, products, cfg.onConflict, importEvent, actor); err != nil {
		return err
	}

	c.logger.InfoContext(ctx, "catalog imported",
		"filename", filename, "file_hash", fileHash,
		"rulesets", len(rulesets), "products", len(products))

	return nil
}

func marshalRulesetContent(evals []evaluationYAML) ([]byte, error) {
	return yaml.Marshal(rulesetYAML{Evaluations: evals})
}
