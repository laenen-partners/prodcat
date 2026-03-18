package prodcat

import "context"

// EligibilityService manages product eligibility configuration and evaluation.
type EligibilityService interface {
	// Products
	RegisterProduct(ctx context.Context, p ProductEligibility) error
	GetProduct(ctx context.Context, productID string) (ProductEligibility, error)
	ListProducts(ctx context.Context, filter TagFilter) ([]ProductEligibility, error)
	UpdateProduct(ctx context.Context, p ProductEligibility) error

	// Base rulesets
	CreateRuleset(ctx context.Context, r BaseRuleset) (BaseRuleset, error)
	GetRuleset(ctx context.Context, id string) (BaseRuleset, error)
	ListRulesets(ctx context.Context) ([]BaseRuleset, error)

	// Evaluation
	Evaluate(ctx context.Context, productID string, input EvaluationInput) (EvaluationResult, error)
	CheckEligibility(ctx context.Context, productIDs []string, input EvaluationInput) (EligibilityReport, error)
	ResolveRuleset(ctx context.Context, productID string) (ResolvedRuleset, error)
}

// SubscriptionService manages customer subscriptions and their lifecycle.
type SubscriptionService interface {
	Subscribe(ctx context.Context, req SubscribeRequest) (Subscription, error)
	GetSubscription(ctx context.Context, id string) (Subscription, error)
	ListSubscriptions(ctx context.Context, filter SubscriptionFilter) ([]Subscription, error)
	Activate(ctx context.Context, id string, externalRef string, capabilities []CapabilityType) (Subscription, error)
	Cancel(ctx context.Context, id string, reason string) (Subscription, error)

	Disable(ctx context.Context, id string, reason DisabledReason, message string) (Subscription, error)
	Enable(ctx context.Context, id string) (Subscription, error)
	DisableCapability(ctx context.Context, subID string, capID string, reason DisabledReason, message string) (Subscription, error)
	EnableCapability(ctx context.Context, subID string, capID string) (Subscription, error)

	AddParty(ctx context.Context, subID string, customerID string, role PartyRole) (Subscription, error)
	RemoveParty(ctx context.Context, subID string, partyID string) (Subscription, error)
}

// ─── Request / Filter Types ───

// TagFilter filters products by tags.
type TagFilter struct {
	Tags        []string // all must match
	CountryCode string
	Status      ProductStatus
}

// EvaluationInput carries the data needed for eligibility evaluation.
type EvaluationInput struct {
	Contacts    []ContactInput    `json:"contacts"`
	Agreements  []AgreementInput  `json:"agreements"`
	IDDocuments []IDDocumentInput `json:"id_documents"`

	MPINCreated      bool     `json:"mpin_created"`
	LivenessPassed   bool     `json:"liveness_passed"`
	Age              int      `json:"age"`
	Nationalities    []string `json:"nationalities"`
	CountryOfResidence string `json:"country_of_residence"`

	Entity *BusinessEntityInput `json:"entity,omitempty"`
}

// ContactInput represents a contact method.
type ContactInput struct {
	Type     int    `json:"type"` // 1=email, 2=phone
	Value    string `json:"value"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// AgreementInput represents a legal agreement acceptance.
type AgreementInput struct {
	Type     string `json:"type"`
	Accepted bool   `json:"accepted"`
	Version  string `json:"version"`
}

// IDDocumentInput represents an identity document.
type IDDocumentInput struct {
	DocumentType   string `json:"document_type"`
	IssuingCountry string `json:"issuing_country"`
	Verified       bool   `json:"verified"`
	Expired        bool   `json:"expired"`
}

// BusinessEntityInput represents a business entity.
type BusinessEntityInput struct {
	EntityType           string   `json:"entity_type"`
	Jurisdiction         string   `json:"jurisdiction"`
	TradeLicenseValid    bool     `json:"trade_license_valid"`
	AuthorizedActivities []string `json:"authorized_activities"`
}

// SubscribeRequest creates a new subscription.
type SubscribeRequest struct {
	EntityID   string     `json:"entity_id"`
	EntityType EntityType `json:"entity_type"`
	ProductID  string     `json:"product_id"`
	Parties    []PartyInput `json:"parties"`
}

// PartyInput is a party to add to a subscription.
type PartyInput struct {
	CustomerID string    `json:"customer_id"`
	Role       PartyRole `json:"role"`
}

// SubscriptionFilter filters subscriptions.
type SubscriptionFilter struct {
	EntityID string
	Status   SubscriptionStatus
	Limit    int
	Offset   int
}
