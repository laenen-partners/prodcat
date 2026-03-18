package prodcat

import (
	"context"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// SeedRecord tracks which seed files have been applied.
type SeedRecord struct {
	Filename  string    `json:"filename"`
	AppliedAt time.Time `json:"applied_at"`
	Checksum  string    `json:"checksum"`
}

// SeedTracker persists which seeds have been applied.
type SeedTracker interface {
	HasApplied(ctx context.Context, filename string) (bool, error)
	RecordApplied(ctx context.Context, record SeedRecord) error
	ListApplied(ctx context.Context) ([]SeedRecord, error)
}

// SeedFile is the top-level structure of a seed YAML file.
type SeedFile struct {
	Kind     string        `yaml:"kind"`
	Version  string        `yaml:"version"`
	Rulesets []SeedRuleset `yaml:"rulesets"`
	Products []SeedProduct `yaml:"products"`
}

// SeedRuleset is a ruleset definition in a seed file.
type SeedRuleset struct {
	ID          string           `yaml:"id"`
	Name        string           `yaml:"name"`
	Description string           `yaml:"description"`
	Version     string           `yaml:"version"`
	Evaluations []evaluationYAML `yaml:"evaluations"`
}

// SeedProduct is a product definition in a seed file.
type SeedProduct struct {
	ProductID       string   `yaml:"product_id"`
	Name            string   `yaml:"name"`
	Description     string   `yaml:"description"`
	Tags            []string `yaml:"tags"`
	Status          string   `yaml:"status"`
	CurrencyCode    string   `yaml:"currency_code,omitempty"`
	ShariaCompliant bool     `yaml:"sharia_compliant"`
	Availability    SeedGeo  `yaml:"availability"`
	BaseRulesetIDs  []string `yaml:"base_ruleset_ids"`
}

// SeedGeo is geographic availability in a seed file.
type SeedGeo struct {
	Mode         string   `yaml:"mode"`
	CountryCodes []string `yaml:"country_codes,omitempty"`
}

// ApplySeed parses and applies a seed file. It registers rulesets and products
// through the engine. If a tracker is provided, it checks whether the seed
// has already been applied and records it after success.
func (e *Engine) ApplySeed(ctx context.Context, filename string, data []byte, tracker SeedTracker) error {
	if tracker != nil {
		applied, err := tracker.HasApplied(ctx, filename)
		if err != nil {
			return fmt.Errorf("check seed status: %w", err)
		}
		if applied {
			return nil // already applied
		}
	}

	var seed SeedFile
	if err := yaml.Unmarshal(data, &seed); err != nil {
		return fmt.Errorf("parse seed file %s: %w", filename, err)
	}

	if seed.Kind != "seed" {
		return fmt.Errorf("unexpected kind %q in %s", seed.Kind, filename)
	}

	// Apply rulesets first (products reference them).
	for _, rs := range seed.Rulesets {
		content, err := marshalRulesetContent(rs.Evaluations)
		if err != nil {
			return fmt.Errorf("marshal ruleset %s: %w", rs.ID, err)
		}
		if _, err := e.CreateRuleset(ctx, BaseRuleset{
			ID:          rs.ID,
			Name:        rs.Name,
			Description: rs.Description,
			Content:     content,
			Version:     rs.Version,
		}); err != nil {
			return fmt.Errorf("create ruleset %s: %w", rs.ID, err)
		}
	}

	// Apply products.
	for _, p := range seed.Products {
		if err := e.RegisterProduct(ctx, ProductEligibility{
			ProductID:       p.ProductID,
			Name:            p.Name,
			Description:     p.Description,
			Tags:            p.Tags,
			Status:          ProductStatus(p.Status),
			CurrencyCode:    p.CurrencyCode,
			ShariaCompliant: p.ShariaCompliant,
			Availability: GeoAvailability{
				Mode:         AvailabilityMode(p.Availability.Mode),
				CountryCodes: p.Availability.CountryCodes,
			},
			BaseRulesetIDs: p.BaseRulesetIDs,
		}); err != nil {
			return fmt.Errorf("register product %s: %w", p.ProductID, err)
		}
	}

	if tracker != nil {
		if err := tracker.RecordApplied(ctx, SeedRecord{
			Filename:  filename,
			AppliedAt: time.Now().UTC(),
		}); err != nil {
			return fmt.Errorf("record seed applied: %w", err)
		}
	}

	return nil
}

func marshalRulesetContent(evals []evaluationYAML) ([]byte, error) {
	return yaml.Marshal(rulesetYAML{Evaluations: evals})
}
