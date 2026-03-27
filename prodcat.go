// Package prodcat is a product catalogue and ruleset store.
// Rule evaluation lives in the onboarding package.
package prodcat

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ContentHashOf returns the SHA-256 hex digest of the given content.
func ContentHashOf(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}

// AvailabilityMode defines how geographic availability is interpreted.
type AvailabilityMode string

const (
	AvailabilityModeSpecificCountries AvailabilityMode = "prod:geo_specific_countries"
	AvailabilityModeGlobal            AvailabilityMode = "prod:geo_global"
	AvailabilityModeGlobalExcept      AvailabilityMode = "prod:global_except"
)

// DisabledReason explains why a product or ruleset is disabled.
// Follows the Stripe pattern — always include a machine-readable reason.
type DisabledReason string

const (
	DisabledReasonRegulatoryHold DisabledReason = "regulatory_hold"
	DisabledReasonOperations     DisabledReason = "operations"
	DisabledReasonDeleted        DisabledReason = "deleted"
	DisabledReasonSuperseded     DisabledReason = "superseded"
)

// ─── Domain Types ───

// Product is a product in the catalogue.
// Status is managed via the tags lifecycle (Disabled/DisabledReason),
// not a separate status field.
type Product struct {
	ProductID       string          `json:"product_id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Tags            []string        `json:"tags"`
	Disabled        bool            `json:"disabled"`
	DisabledReason  DisabledReason  `json:"disabled_reason,omitempty"`
	CurrencyCode    string          `json:"currency_code,omitempty"`
	ParentProductID string          `json:"parent_product_id,omitempty"`
	Availability    GeoAvailability `json:"availability"`
	BaseRulesetIDs  []string        `json:"base_ruleset_ids,omitempty"`
	Ruleset         []byte          `json:"ruleset,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// GeoAvailability defines geographic availability.
type GeoAvailability struct {
	Mode         AvailabilityMode `json:"mode"`
	CountryCodes []string         `json:"country_codes,omitempty"`
}

// Ruleset is a reusable eval engine ruleset.
type Ruleset struct {
	ID             string         `json:"ruleset_id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Content        []byte         `json:"content"`
	ContentHash    string         `json:"content_hash"`
	Version        string         `json:"version"`
	Disabled       bool           `json:"disabled"`
	DisabledReason DisabledReason `json:"disabled_reason,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// ─── Filter ───

// ListFilter filters products by tags. All tags must match (AND).
type ListFilter struct {
	Tags        []string
	CountryCode string
}

// ─── Provenance ───

// Provenance tracks who/what made a change and why.
type Provenance struct {
	SourceURN string // who/what made the change (e.g. "user:admin-123", "import:20260318", "api:onboarding")
	Reason    string // optional human-readable reason
}

// ─── Ruleset Resolution ───

// ResolvedRuleset is the merged ruleset for a product.
type ResolvedRuleset struct {
	ProductID string         `json:"product_id"`
	Merged    []byte         `json:"merged"`
	Layers    []RulesetLayer `json:"layers"`
}

// RulesetLayer identifies one component of a merged ruleset.
type RulesetLayer struct {
	Source   string `json:"source"` // "base" or "product"
	SourceID string `json:"source_id"`
}
