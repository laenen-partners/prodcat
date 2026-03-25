package prodcat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// ImportRecord tracks which catalogue definitions have been imported.
type ImportRecord struct {
	Filename  string    `json:"filename"`
	AppliedAt time.Time `json:"applied_at"`
	Checksum  string    `json:"checksum"`
}

// ImportTracker persists which catalogue definitions have been imported.
type ImportTracker interface {
	HasImported(ctx context.Context, filename string) (bool, error)
	RecordImported(ctx context.Context, record ImportRecord) error
	ListImported(ctx context.Context) ([]ImportRecord, error)
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
	ProductID      string     `yaml:"product_id"`
	Name           string     `yaml:"name"`
	Description    string     `yaml:"description"`
	Tags           []string   `yaml:"tags"`
	CurrencyCode   string     `yaml:"currency_code,omitempty"`
	Availability   CatalogGeo `yaml:"availability"`
	BaseRulesetIDs []string   `yaml:"base_ruleset_ids"`
}

// CatalogEvaluation is a single evaluation rule in a catalogue ruleset.
type CatalogEvaluation = evaluationYAML

// CatalogGeo is geographic availability in a catalogue file.
type CatalogGeo struct {
	Mode         string   `yaml:"mode"`
	CountryCodes []string `yaml:"country_codes,omitempty"`
}

// Import parses and applies a catalogue definition file. Rulesets and products
// are upserted for idempotency. When a tracker is provided, already-imported
// files are skipped.
//
// Provenance is set to "import:<filename>" so all changes are traceable to
// the specific catalogue file that created them.
func (c *Client) Import(ctx context.Context, filename string, data []byte, tracker ImportTracker) error {
	if tracker != nil {
		imported, err := tracker.HasImported(ctx, filename)
		if err != nil {
			return fmt.Errorf("check import status: %w", err)
		}
		if imported {
			return nil
		}
	}

	var def CatalogDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return fmt.Errorf("parse catalogue definition %s: %w", filename, err)
	}

	if err := ValidateCatalogDefinition(&def); err != nil {
		return fmt.Errorf("validate %s: %w", filename, err)
	}

	prov := Provenance{SourceURN: "import:" + filename}

	// Import rulesets first (products reference them).
	// Use PutRuleset (upsert) for idempotency.
	for _, rs := range def.Rulesets {
		content, err := marshalRulesetContent(rs.Evaluations)
		if err != nil {
			return fmt.Errorf("marshal ruleset %s: %w", rs.ID, err)
		}
		now := time.Now().UTC()
		if err := c.store.PutRuleset(ctx, Ruleset{
			ID:          rs.ID,
			Name:        rs.Name,
			Description: rs.Description,
			Content:     content,
			Version:     rs.Version,
			CreatedAt:   now,
			UpdatedAt:   now,
		}, prov); err != nil {
			return fmt.Errorf("put ruleset %s: %w", rs.ID, err)
		}
	}

	// Import products — upsert for idempotency.
	for _, p := range def.Products {
		if err := c.upsertProduct(ctx, Product{
			ProductID:    p.ProductID,
			Name:         p.Name,
			Description:  p.Description,
			Tags:         p.Tags,
			CurrencyCode: p.CurrencyCode,
			Availability: GeoAvailability{
				Mode:         AvailabilityMode(p.Availability.Mode),
				CountryCodes: p.Availability.CountryCodes,
			},
			BaseRulesetIDs: p.BaseRulesetIDs,
		}, prov); err != nil {
			return fmt.Errorf("upsert product %s: %w", p.ProductID, err)
		}
	}

	if tracker != nil {
		if err := tracker.RecordImported(ctx, ImportRecord{
			Filename:  filename,
			AppliedAt: time.Now().UTC(),
		}); err != nil {
			// ErrAlreadyExists means a concurrent import recorded it first — that's fine.
			if !errors.Is(err, ErrAlreadyExists) {
				return fmt.Errorf("record import: %w", err)
			}
		}
	}

	return nil
}

func marshalRulesetContent(evals []evaluationYAML) ([]byte, error) {
	return yaml.Marshal(rulesetYAML{Evaluations: evals})
}
