package prodcat

import "context"

// CatalogService manages the product hierarchy and eval engine rulesets.
type CatalogService interface {
	// Families
	CreateFamily(ctx context.Context, f FamilyDefinition) (FamilyDefinition, error)
	GetFamily(ctx context.Context, id string) (FamilyDefinition, error)
	ListFamilies(ctx context.Context) ([]FamilyDefinition, error)
	UpdateFamily(ctx context.Context, f FamilyDefinition) (FamilyDefinition, error)

	// Archetypes
	CreateArchetype(ctx context.Context, a Archetype) (Archetype, error)
	GetArchetype(ctx context.Context, id string) (Archetype, error)
	ListArchetypes(ctx context.Context, familyID string) ([]Archetype, error)
	UpdateArchetype(ctx context.Context, a Archetype) (Archetype, error)

	// Products
	CreateProduct(ctx context.Context, p Product) (Product, error)
	GetProduct(ctx context.Context, id string) (Product, error)
	ListProducts(ctx context.Context, filter ProductFilter) ([]Product, error)
	UpdateProduct(ctx context.Context, p Product) (Product, error)
	TransitionProductStatus(ctx context.Context, id string, target ProductStatus, reason string) (Product, error)

	// Base rulesets
	CreateBaseRuleset(ctx context.Context, r BaseRuleset) (BaseRuleset, RulesetValidation, error)
	GetBaseRuleset(ctx context.Context, id string) (BaseRuleset, error)
	ListBaseRulesets(ctx context.Context) ([]BaseRuleset, error)
	UpdateBaseRuleset(ctx context.Context, r BaseRuleset) (BaseRuleset, RulesetValidation, error)

	// Product discovery
	ListAvailableProducts(ctx context.Context, filter AvailableProductFilter) ([]Product, error)

	// Ruleset resolution
	ResolveProductRuleset(ctx context.Context, productID string) (ResolvedRuleset, error)
}

// SubscriptionService manages customer subscriptions and their lifecycle.
type SubscriptionService interface {
	// Subscription lifecycle
	Subscribe(ctx context.Context, req SubscribeRequest) (Subscription, error)
	GetSubscription(ctx context.Context, id string) (Subscription, error)
	ListSubscriptions(ctx context.Context, filter SubscriptionFilter) ([]Subscription, error)
	Activate(ctx context.Context, id string, externalRef string, capabilities []CapabilityType) (Subscription, error)
	Cancel(ctx context.Context, id string, reason string) (Subscription, error)

	// Disable / Enable
	Disable(ctx context.Context, id string, reason DisabledReason, message string) (Subscription, error)
	Enable(ctx context.Context, id string) (Subscription, error)
	DisableCapability(ctx context.Context, subscriptionID string, capabilityID string, reason DisabledReason, message string) (Subscription, error)
	EnableCapability(ctx context.Context, subscriptionID string, capabilityID string) (Subscription, error)

	// Evaluation
	Evaluate(ctx context.Context, subscriptionID string) (Subscription, []EvalDelta, error)
	EvaluateParty(ctx context.Context, subscriptionID string, partyID string) (Party, []EvalDelta, error)
	CheckAccess(ctx context.Context, entityID string) ([]SubscriptionAccess, error)

	// Party management
	AddParty(ctx context.Context, subscriptionID string, customerID string, role PartyRole) (Subscription, error)
	RemoveParty(ctx context.Context, subscriptionID string, partyID string, reason string) (Subscription, error)

	// Signing authority
	UpdateSigningAuthority(ctx context.Context, subscriptionID string, authority SigningAuthority) (Subscription, error)
}

// ─── Request / Response Types ───

type ProductFilter struct {
	ArchetypeID  string
	FamilyID     string
	Status       ProductStatus
	CurrencyCode string
	CountryCode  string
	Limit        int
	Offset       int
}

type AvailableProductFilter struct {
	CountryCode  string
	CustomerType CustomerType
	Family       ProductFamily
}

type SubscribeRequest struct {
	EntityID         string
	EntityType       EntityType
	ProductID        string
	InitialParties   []PartyInput
	SigningAuthority SigningAuthority
}

type PartyInput struct {
	CustomerID string
	Role       PartyRole
}

type SubscriptionFilter struct {
	EntityID   string
	CustomerID string
	Status     SubscriptionStatus
	Limit      int
	Offset     int
}

type EvalDelta struct {
	EvaluationName string
	PreviouslyPassed bool
	NowPassed        bool
}

type SubscriptionAccess struct {
	SubscriptionID string
	ProductID      string
	Status         SubscriptionStatus
	Disabled       *DisabledState
	Capabilities   []Capability
	PartyStatuses  []PartyAccessStatus
}

type PartyAccessStatus struct {
	PartyID        string
	CustomerID     string
	Role           PartyRole
	RequirementsMet bool
	Disabled       *DisabledState
}

type ResolvedRuleset struct {
	ProductID  string
	Merged     []byte
	Validation RulesetValidation
	Layers     []RulesetLayer
}

type RulesetLayer struct {
	Source          string // "base", "family", "archetype", "product"
	SourceID        string
	EvaluationCount int
}
