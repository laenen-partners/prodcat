package prodcat

import (
	"context"

	"github.com/laenen-partners/identity"
)

// actorFromContext extracts the principal ID from context for event attribution.
func actorFromContext(ctx context.Context) string {
	if id, ok := identity.FromContext(ctx); ok {
		return id.PrincipalID()
	}
	return "system"
}

// ─── Product Events ───

type ProductCreatedEvent struct {
	ProductID      string
	Actor          string
	Name           string
	Description    string
	Tags           []string
	CurrencyCode   string
	BaseRulesetIDs []string
}

type ProductUpdatedEvent struct {
	ProductID      string
	Actor          string
	Name           string
	Description    string
	Tags           []string
	CurrencyCode   string
	BaseRulesetIDs []string
}

type ProductDisabledEvent struct {
	ProductID string
	Actor     string
	Reason    string
	Name      string
}

type ProductEnabledEvent struct {
	ProductID string
	Actor     string
	Name      string
}

type ProductDeletedEvent struct {
	ProductID string
	Actor     string
	Name      string
}

// ─── Ruleset Events ───

type RulesetCreatedEvent struct {
	RulesetID   string
	Actor       string
	Name        string
	Description string
	Version     string
}

type RulesetDisabledEvent struct {
	RulesetID string
	Actor     string
	Reason    string
	Name      string
}

type RulesetEnabledEvent struct {
	RulesetID string
	Actor     string
	Name      string
}

type RulesetDeletedEvent struct {
	RulesetID string
	Actor     string
	Name      string
}

// ─── Linking Events ───

type RulesetLinkedToProductEvent struct {
	ProductID   string
	RulesetID   string
	Actor       string
	ProductName string
	RulesetName string
}

type RulesetUnlinkedFromProductEvent struct {
	ProductID   string
	RulesetID   string
	Actor       string
	ProductName string
	RulesetName string
}

// ─── Import Events ───

type CatalogImportedEvent struct {
	Filename     string
	Actor        string
	RulesetCount int
	ProductCount int
	ProductIDs   []string
	RulesetIDs   []string
}
