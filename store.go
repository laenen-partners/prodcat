package prodcat

import "context"

// Store is the persistence interface for the product catalogue.
type Store interface {
	// CreateProduct creates a new product, failing with ErrAlreadyExists if the ProductID is taken.
	CreateProduct(ctx context.Context, p Product, prov Provenance) error
	// PutProduct upserts a product. When the product references rulesets via BaseRulesetIDs,
	// the store verifies each ruleset exists and is not disabled.
	PutProduct(ctx context.Context, p Product, prov Provenance) error
	GetProduct(ctx context.Context, productID string) (*Product, error)
	ListProducts(ctx context.Context, filter ListFilter) ([]Product, error)

	// CreateRuleset creates a new ruleset, failing with ErrAlreadyExists if the ID is taken.
	CreateRuleset(ctx context.Context, r Ruleset, prov Provenance) error
	// PutRuleset upserts a ruleset. When Disabled is true, the store tags the entity
	// with "status:disabled" for precondition checks by other operations.
	PutRuleset(ctx context.Context, r Ruleset, prov Provenance) error
	GetRuleset(ctx context.Context, id string) (*Ruleset, error)
	ListRulesets(ctx context.Context) ([]Ruleset, error)
}
