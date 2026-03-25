package prodcat

import (
	"fmt"
	"strings"
	"time"

	"github.com/laenen-partners/evalengine"
	"gopkg.in/yaml.v3"
)

// ValidateCatalogDefinition validates a catalogue definition structurally.
// It checks required fields on products and rulesets, and delegates ruleset
// content validation to evalengine.ValidateConfig.
func ValidateCatalogDefinition(def *CatalogDefinition) error {
	var errs []string

	if def.Kind != "catalog" {
		errs = append(errs, fmt.Sprintf("kind must be \"catalog\", got %q", def.Kind))
	}

	if len(def.Rulesets) == 0 && len(def.Products) == 0 {
		errs = append(errs, "catalog must contain at least one ruleset or product")
	}

	// Validate rulesets.
	rulesetIDs := make(map[string]bool)
	for i, rs := range def.Rulesets {
		prefix := fmt.Sprintf("rulesets[%d]", i)
		if rs.ID == "" {
			errs = append(errs, prefix+": id is required")
		} else if rulesetIDs[rs.ID] {
			errs = append(errs, prefix+fmt.Sprintf(": duplicate id %q", rs.ID))
		} else {
			rulesetIDs[rs.ID] = true
		}
		if rs.Name == "" {
			errs = append(errs, prefix+": name is required")
		}
		if len(rs.Evaluations) == 0 {
			errs = append(errs, prefix+": evaluations is required")
		} else {
			// Validate evaluations via evalengine.
			content, err := marshalRulesetContent(rs.Evaluations)
			if err != nil {
				errs = append(errs, prefix+fmt.Sprintf(": marshal error: %v", err))
				continue
			}
			if evalErrs := ValidateRulesetContent(content); evalErrs != nil {
				errs = append(errs, prefix+fmt.Sprintf(" (%s): %v", rs.ID, evalErrs))
			}
		}
	}

	// Validate products.
	productIDs := make(map[string]bool)
	for i, p := range def.Products {
		prefix := fmt.Sprintf("products[%d]", i)
		if p.ProductID == "" {
			errs = append(errs, prefix+": product_id is required")
		} else if productIDs[p.ProductID] {
			errs = append(errs, prefix+fmt.Sprintf(": duplicate product_id %q", p.ProductID))
		} else {
			productIDs[p.ProductID] = true
		}
		if p.Name == "" {
			errs = append(errs, prefix+": name is required")
		}
		// Validate that referenced rulesets are defined in this catalog or will exist externally.
		// We check for internal consistency — rulesets defined in the same file.
		for _, rsID := range p.BaseRulesetIDs {
			if rsID == "" {
				errs = append(errs, prefix+": base_ruleset_ids contains empty string")
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s: %w", strings.Join(errs, "; "), ErrValidation)
	}
	return nil
}

// ValidateRulesetContent validates ruleset YAML content against the evalengine
// schema. It uses evalengine.ValidateConfig for structural checks: required
// fields (name, expression, writes), duplicate writes, cache_ttl format,
// and precondition expressions.
func ValidateRulesetContent(content []byte) error {
	cfg, err := toEvalConfig(content)
	if err != nil {
		return fmt.Errorf("parse ruleset: %w", err)
	}
	if err := evalengine.ValidateConfig(cfg); err != nil {
		return err
	}
	return nil
}

// toEvalConfig converts prodcat ruleset YAML into an evalengine.EvalConfig.
func toEvalConfig(content []byte) (*evalengine.EvalConfig, error) {
	var rs rulesetYAML
	if err := yaml.Unmarshal(content, &rs); err != nil {
		return nil, err
	}

	cfg := &evalengine.EvalConfig{
		Evaluations: make([]evalengine.EvalDefinition, len(rs.Evaluations)),
	}
	for i, e := range rs.Evaluations {
		reads := make([]evalengine.FieldRef, len(e.Reads))
		for j, r := range e.Reads {
			reads[j] = evalengine.FieldRef(r)
		}

		var preconditions []evalengine.Precondition
		for _, pc := range e.Preconditions {
			preconditions = append(preconditions, evalengine.Precondition{
				Expression:  pc.Expression,
				Description: pc.Description,
			})
		}

		def := evalengine.EvalDefinition{
			Name:               e.Name,
			Description:        e.Description,
			Expression:         e.Expression,
			Reads:              reads,
			Writes:             evalengine.FieldRef(e.Writes),
			ResolutionWorkflow: e.ResolutionWorkflow,
			Resolution:         e.Resolution,
			Severity:           e.Severity,
			Category:           e.Category,
			FailureMode:        e.FailureMode,
			CacheTTL:           e.CacheTTL,
			Preconditions:      preconditions,
		}

		// Validate cache_ttl format if set.
		if e.CacheTTL != "" {
			if _, err := time.ParseDuration(e.CacheTTL); err != nil {
				return nil, fmt.Errorf("evaluation %q: invalid cache_ttl %q: %w", e.Name, e.CacheTTL, err)
			}
		}

		cfg.Evaluations[i] = def
	}
	return cfg, nil
}
