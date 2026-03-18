package prodcat

import (
	"context"
	"time"
)

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

// ─── Evaluation Input (party-centric) ───

// EvaluationInput carries all data needed for eligibility evaluation.
// It is party-centric: each party carries their own data.
// Agreements and business entity details are at the subscription level.
type EvaluationInput struct {
	Parties        []EvalPartyInput     `json:"parties"`
	Agreements     []AgreementInput     `json:"agreements"`
	BusinessEntity *BusinessEntityInput `json:"business_entity,omitempty"`
}

// PartyType distinguishes individuals from organisations.
type PartyType string

const (
	PartyTypeIndividual   PartyType = "individual"
	PartyTypeOrganisation PartyType = "organisation"
)

// EvalPartyInput carries all evaluation data for a single party.
type EvalPartyInput struct {
	PartyID string    `json:"party_id"`
	Type    PartyType `json:"type"`
	Role    PartyRole `json:"role"`

	// Contact information.
	Contacts []ContactInput `json:"contacts"`

	// Identity documents.
	IDDocuments []IDDocumentInput `json:"id_documents"`

	// Demographics.
	Age                int      `json:"age"`
	Nationalities      []string `json:"nationalities"`
	CountryOfResidence string   `json:"country_of_residence"`

	// KYC checks.
	KYC KycChecks `json:"kyc"`

	// Security.
	MPINCreated bool `json:"mpin_created"`
}

// ContactInput represents a contact method.
type ContactInput struct {
	Type       int        `json:"type"` // 1=email, 2=phone
	Value      string     `json:"value"`
	Primary    bool       `json:"primary"`
	Verified   bool       `json:"verified"`
	VerifiedOn *time.Time `json:"verified_on,omitempty"`
}

// IDDocumentInput represents an identity document.
type IDDocumentInput struct {
	DocumentType   string     `json:"document_type"`
	IssuingCountry string     `json:"issuing_country"`
	Verified       bool       `json:"verified"`
	Expired        bool       `json:"expired"`
	VerifiedOn     *time.Time `json:"verified_on,omitempty"`
	ExpiryDate     *time.Time `json:"expiry_date,omitempty"`
	DocumentNumber string     `json:"document_number,omitempty"`
}

// KycChecks captures the state of all KYC verification steps for a party.
type KycChecks struct {
	// Identity verification (IDV).
	LivenessPassed      bool       `json:"liveness_passed"`
	LivenessPassedOn    *time.Time `json:"liveness_passed_on,omitempty"`
	FacialMatchPassed   bool       `json:"facial_match_passed"`
	FacialMatchPassedOn *time.Time `json:"facial_match_passed_on,omitempty"`

	// Address verification.
	ResidentialAddressValidated   bool       `json:"residential_address_validated"`
	ResidentialAddressValidatedOn *time.Time `json:"residential_address_validated_on,omitempty"`

	// Screening.
	PEPScreeningClear         bool       `json:"pep_screening_clear"`
	PEPScreeningClearOn       *time.Time `json:"pep_screening_clear_on,omitempty"`
	SanctionsScreeningClear   bool       `json:"sanctions_screening_clear"`
	SanctionsScreeningClearOn *time.Time `json:"sanctions_screening_clear_on,omitempty"`
	AdverseMediaClear         bool       `json:"adverse_media_clear"`
	AdverseMediaClearOn       *time.Time `json:"adverse_media_clear_on,omitempty"`

	// Source of funds / wealth.
	SourceOfFundsVerified    bool       `json:"source_of_funds_verified"`
	SourceOfFundsVerifiedOn  *time.Time `json:"source_of_funds_verified_on,omitempty"`
	SourceOfWealthVerified   bool       `json:"source_of_wealth_verified"`
	SourceOfWealthVerifiedOn *time.Time `json:"source_of_wealth_verified_on,omitempty"`

	// Tax / regulatory.
	TaxResidencyDeclared   bool       `json:"tax_residency_declared"`
	TaxResidencyDeclaredOn *time.Time `json:"tax_residency_declared_on,omitempty"`
	FATCADeclared          bool       `json:"fatca_declared"`
	FATCADeclaredOn        *time.Time `json:"fatca_declared_on,omitempty"`
	CRSDeclared            bool       `json:"crs_declared"`
	CRSDeclaredOn          *time.Time `json:"crs_declared_on,omitempty"`

	// Employment.
	EmploymentVerified   bool       `json:"employment_verified"`
	EmploymentVerifiedOn *time.Time `json:"employment_verified_on,omitempty"`

	// Risk assessment.
	RiskRating   string     `json:"risk_rating,omitempty"` // low, medium, high
	RiskRatingOn *time.Time `json:"risk_rating_on,omitempty"`
}

// AgreementInput represents a legal agreement acceptance.
type AgreementInput struct {
	Type       string     `json:"type"`
	Accepted   bool       `json:"accepted"`
	Version    string     `json:"version"`
	AcceptedOn *time.Time `json:"accepted_on,omitempty"`
}

// BusinessEntityInput represents a business entity.
type BusinessEntityInput struct {
	EntityType              string     `json:"entity_type"`
	Jurisdiction            string     `json:"jurisdiction"`
	TradeLicenseValid       bool       `json:"trade_license_valid"`
	TradeLicenseExpiry      *time.Time `json:"trade_license_expiry,omitempty"`
	AuthorizedActivities    []string   `json:"authorized_activities"`
	IncorporationVerified   bool       `json:"incorporation_verified"`
	IncorporationVerifiedOn *time.Time `json:"incorporation_verified_on,omitempty"`
}

// ─── Subscription Requests ───

// SubscribeRequest creates a new subscription.
type SubscribeRequest struct {
	EntityID   string                `json:"entity_id"`
	EntityType EntityType            `json:"entity_type"`
	ProductID  string                `json:"product_id"`
	Parties    []SubscribePartyInput `json:"parties"`
}

// SubscribePartyInput is a party to add to a subscription.
type SubscribePartyInput struct {
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
