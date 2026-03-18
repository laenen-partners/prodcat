// Package prodcat is a lightweight product catalogue for digital banking.
// It defines products, eligibility requirements (via eval engine rulesets),
// and customer subscriptions with Stripe-style capability control.
package prodcat

import "time"

// ─── Product Hierarchy Enums ───

// ProductFamily is the top-level product grouping.
type ProductFamily string

const (
	ProductFamilyCASA       ProductFamily = "casa"
	ProductFamilyLending    ProductFamily = "lending"
	ProductFamilyCards      ProductFamily = "cards"
	ProductFamilyPayments   ProductFamily = "payments"
	ProductFamilyInvestment ProductFamily = "investments"
	ProductFamilyInsurance  ProductFamily = "insurance"
	ProductFamilyPFM        ProductFamily = "pfm"
	ProductFamilyValueAdded ProductFamily = "value_added"
)

// ProductStatus tracks the product lifecycle.
type ProductStatus string

const (
	ProductStatusDraft      ProductStatus = "draft"
	ProductStatusActive     ProductStatus = "active"
	ProductStatusSuspended  ProductStatus = "suspended"
	ProductStatusDeprecated ProductStatus = "deprecated"
	ProductStatusRetired    ProductStatus = "retired"
)

// ProductType distinguishes standalone and linked products.
type ProductType string

const (
	ProductTypePrimary       ProductType = "primary"
	ProductTypeSupplementary ProductType = "supplementary"
)

// ─── Eligibility Enums ───

// AvailabilityMode defines how geographic availability is interpreted.
type AvailabilityMode string

const (
	AvailabilityModeSpecificCountries AvailabilityMode = "specific_countries"
	AvailabilityModeGlobal            AvailabilityMode = "global"
	AvailabilityModeGlobalExcept      AvailabilityMode = "global_except"
)

// CustomerType restricts products by customer segment.
type CustomerType string

const (
	CustomerTypeIndividual     CustomerType = "individual"
	CustomerTypeSoleProprietor CustomerType = "sole_proprietor"
	CustomerTypeSME            CustomerType = "sme"
	CustomerTypeCorporate      CustomerType = "corporate"
	CustomerTypeMinor          CustomerType = "minor"
	CustomerTypeNonResident    CustomerType = "non_resident"
)

// AgreementType categorises legal agreements.
type AgreementType string

const (
	AgreementTypeTermsAndConditions      AgreementType = "terms_and_conditions"
	AgreementTypePrivacyPolicy           AgreementType = "privacy_policy"
	AgreementTypeKeyFactsStatement       AgreementType = "key_facts_statement"
	AgreementTypeShariah                 AgreementType = "shariah_commodity_purchase"
	AgreementTypeDataProcessing          AgreementType = "data_processing"
	AgreementTypeCustom                  AgreementType = "custom"
)

// ─── Subscription Enums ───

// EntityType distinguishes individual and business subscription owners.
type EntityType string

const (
	EntityTypeIndividual EntityType = "individual"
	EntityTypeBusiness   EntityType = "business"
)

// SubscriptionStatus tracks the subscription lifecycle.
type SubscriptionStatus string

const (
	SubscriptionStatusIncomplete SubscriptionStatus = "incomplete"
	SubscriptionStatusActive     SubscriptionStatus = "active"
	SubscriptionStatusPastDue    SubscriptionStatus = "past_due"
	SubscriptionStatusCanceled   SubscriptionStatus = "canceled"
)

// PartyRole defines a person's role on a subscription.
type PartyRole string

const (
	PartyRolePrimaryHolder       PartyRole = "primary_holder"
	PartyRoleJointHolder         PartyRole = "joint_holder"
	PartyRoleAuthorizedSignatory PartyRole = "authorized_signatory"
	PartyRoleDirector            PartyRole = "director"
	PartyRoleUBO                 PartyRole = "ubo"
	PartyRoleSecretary           PartyRole = "secretary"
	PartyRolePOA                 PartyRole = "poa"
	PartyRoleGuardian            PartyRole = "guardian"
)

// SigningRule defines how many parties must authorize an action.
type SigningRule string

const (
	SigningRuleAnyOne SigningRule = "any_one"
	SigningRuleAnyN   SigningRule = "any_n"
	SigningRuleAll    SigningRule = "all"
)

// CapabilityType defines product features that can be independently controlled.
type CapabilityType string

const (
	CapabilityTypeView                   CapabilityType = "view"
	CapabilityTypeDomesticTransfers      CapabilityType = "domestic_transfers"
	CapabilityTypeInternationalTransfers CapabilityType = "international_transfers"
	CapabilityTypeCardPayments           CapabilityType = "card_payments"
	CapabilityTypeATM                    CapabilityType = "atm"
	CapabilityTypeReceive                CapabilityType = "receive"
	CapabilityTypeBillPayments           CapabilityType = "bill_payments"
	CapabilityTypeFX                     CapabilityType = "fx"
	CapabilityTypeStandingOrders         CapabilityType = "standing_orders"
	CapabilityTypeCustom                 CapabilityType = "custom"
)

// CapabilityStatus tracks whether a capability is usable.
type CapabilityStatus string

const (
	CapabilityStatusActive   CapabilityStatus = "active"
	CapabilityStatusDisabled CapabilityStatus = "disabled"
	CapabilityStatusPending  CapabilityStatus = "pending"
)

// DisabledReason explains why something is disabled.
type DisabledReason string

const (
	DisabledReasonRequirementsNotMet DisabledReason = "requirements_not_met"
	DisabledReasonExpiredData        DisabledReason = "expired_data"
	DisabledReasonFailedEvaluation   DisabledReason = "failed_evaluation"
	DisabledReasonRegulatoryHold     DisabledReason = "regulatory_hold"
	DisabledReasonFraudSuspicion     DisabledReason = "fraud_suspicion"
	DisabledReasonCustomerRequested  DisabledReason = "customer_requested"
	DisabledReasonOperations         DisabledReason = "operations"
	DisabledReasonParentDisabled     DisabledReason = "parent_disabled"
	DisabledReasonPartyIncomplete    DisabledReason = "party_incomplete"
	DisabledReasonPartyRemoved       DisabledReason = "party_removed"
)

// EvalStatus is the overall status of an eval engine run.
type EvalStatus string

const (
	EvalStatusAllPassed      EvalStatus = "all_passed"
	EvalStatusWorkflowActive EvalStatus = "workflow_active"
	EvalStatusActionRequired EvalStatus = "action_required"
	EvalStatusBlocked        EvalStatus = "blocked"
)

// ─── Product Hierarchy Types ───

// FamilyDefinition is the top level of the product hierarchy.
type FamilyDefinition struct {
	ID          string            `json:"id" db:"id"`
	Family      ProductFamily     `json:"family" db:"family"`
	Name        map[string]string `json:"name" db:"name"`
	Description map[string]string `json:"description" db:"description"`
	Ruleset     []byte            `json:"ruleset,omitempty" db:"ruleset"`
	BaseRulesetIDs []string       `json:"base_ruleset_ids,omitempty" db:"base_ruleset_ids"`
	CreatedAt   time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" db:"updated_at"`
}

// Archetype groups related products within a family.
type Archetype struct {
	ID          string            `json:"id" db:"id"`
	FamilyID    string            `json:"family_id" db:"family_id"`
	Name        map[string]string `json:"name" db:"name"`
	Description map[string]string `json:"description" db:"description"`
	Ruleset     []byte            `json:"ruleset,omitempty" db:"ruleset"`
	BaseRulesetIDs []string       `json:"base_ruleset_ids,omitempty" db:"base_ruleset_ids"`
	CreatedAt   time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" db:"updated_at"`
}

// Product is a subscribable offering in the catalogue.
type Product struct {
	ID              string            `json:"id" db:"id"`
	ArchetypeID     string            `json:"archetype_id" db:"archetype_id"`
	Name            map[string]string `json:"name" db:"name"`
	Description     map[string]string `json:"description" db:"description"`
	Tagline         map[string]string `json:"tagline,omitempty" db:"tagline"`
	Status          ProductStatus     `json:"status" db:"status"`
	ProductType     ProductType       `json:"product_type" db:"product_type"`
	CurrencyCode    string            `json:"currency_code" db:"currency_code"`
	ParentProductID *string           `json:"parent_product_id,omitempty" db:"parent_product_id"`
	Provider        RegulatoryProvider `json:"provider" db:"provider"`
	EffectiveFrom   *time.Time        `json:"effective_from,omitempty" db:"effective_from"`
	EffectiveTo     *time.Time        `json:"effective_to,omitempty" db:"effective_to"`
	Compliance      ComplianceConfig   `json:"compliance" db:"compliance"`
	Eligibility     EligibilityConfig  `json:"eligibility" db:"eligibility"`
	CreatedAt       time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at" db:"updated_at"`
	CreatedBy       string             `json:"created_by" db:"created_by"`
}

// ─── Geography & Compliance ───

type RegulatoryProvider struct {
	ProviderID        string `json:"provider_id" db:"provider_id"`
	Name              string `json:"name" db:"name"`
	Regulator         string `json:"regulator" db:"regulator"`
	LicenseNumber     string `json:"license_number" db:"license_number"`
	RegulatoryCountry string `json:"regulatory_country" db:"regulatory_country"`
}

type GeographicAvailability struct {
	Mode         AvailabilityMode `json:"mode" db:"mode"`
	CountryCodes []string         `json:"country_codes,omitempty" db:"country_codes"`
}

type ComplianceConfig struct {
	ShariaCompliant bool             `json:"sharia_compliant" db:"sharia_compliant"`
	Agreements      []LegalAgreement `json:"agreements,omitempty"`
}

type LegalAgreement struct {
	ID            string        `json:"id" db:"id"`
	ProductID     string        `json:"product_id" db:"product_id"`
	AgreementType AgreementType `json:"agreement_type" db:"agreement_type"`
	Title         map[string]string `json:"title" db:"title"`
	Version       string        `json:"version" db:"version"`
	DocumentRef   string        `json:"document_ref" db:"document_ref"`
	Shared        bool          `json:"shared" db:"shared"`
}

// ─── Eligibility ───

type EligibilityConfig struct {
	Geographic     GeographicAvailability `json:"geographic" db:"geographic"`
	Ruleset        []byte                 `json:"ruleset,omitempty" db:"ruleset"`
	BaseRulesetIDs []string               `json:"base_ruleset_ids,omitempty" db:"base_ruleset_ids"`
	AcceptableIDs  []AcceptableIDConfig   `json:"acceptable_ids,omitempty"`
	Segments       []CustomerSegment      `json:"segments,omitempty"`
}

type AcceptableIDConfig struct {
	IDTypeID          string   `json:"id_type_id" db:"id_type_id"`
	IsCategoryWildcard bool    `json:"is_category_wildcard" db:"is_category_wildcard"`
	IssuingGeoFilter  []string `json:"issuing_geo_filter,omitempty" db:"issuing_geo_filter"`
}

type CustomerSegment struct {
	SegmentID    string       `json:"segment_id" db:"segment_id"`
	CustomerType CustomerType `json:"customer_type" db:"customer_type"`
}

// BaseRuleset is a reusable eval engine ruleset.
type BaseRuleset struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Content     []byte    `json:"content" db:"content"`
	Version     string    `json:"version" db:"version"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// ─── Subscription Types ───

// Subscription is a customer's instance of a product.
type Subscription struct {
	ID                   string             `json:"id" db:"id"`
	ProductID            string             `json:"product_id" db:"product_id"`
	EntityID             string             `json:"entity_id" db:"entity_id"`
	EntityType           EntityType         `json:"entity_type" db:"entity_type"`
	Status               SubscriptionStatus `json:"status" db:"status"`
	SigningAuthority     SigningAuthority    `json:"signing_authority" db:"signing_authority"`
	ParentSubscriptionID *string            `json:"parent_subscription_id,omitempty" db:"parent_subscription_id"`
	ExternalRef          *string            `json:"external_ref,omitempty" db:"external_ref"`
	Disabled             *DisabledState     `json:"disabled,omitempty"`
	EvalState            *EvalState         `json:"eval_state,omitempty"`
	Parties              []Party            `json:"parties,omitempty"`
	Capabilities         []Capability       `json:"capabilities,omitempty"`
	CreatedAt            time.Time          `json:"created_at" db:"created_at"`
	ActivatedAt          *time.Time         `json:"activated_at,omitempty" db:"activated_at"`
	CanceledAt           *time.Time         `json:"canceled_at,omitempty" db:"canceled_at"`
}

type SigningAuthority struct {
	Rule          SigningRule                `json:"rule" db:"rule"`
	RequiredCount int                       `json:"required_count" db:"required_count"`
	Overrides     []CapabilitySigningOverride `json:"overrides,omitempty"`
}

type CapabilitySigningOverride struct {
	CapabilityType CapabilityType `json:"capability_type"`
	Rule           SigningRule    `json:"rule"`
	RequiredCount  int            `json:"required_count"`
}

type Party struct {
	ID              string         `json:"id" db:"id"`
	SubscriptionID  string         `json:"subscription_id" db:"subscription_id"`
	CustomerID      string         `json:"customer_id" db:"customer_id"`
	Role            PartyRole      `json:"role" db:"role"`
	RequirementsMet bool           `json:"requirements_met" db:"requirements_met"`
	Disabled        *DisabledState `json:"disabled,omitempty"`
	EvalState       *EvalState     `json:"eval_state,omitempty"`
	AddedAt         time.Time      `json:"added_at" db:"added_at"`
	RemovedAt       *time.Time     `json:"removed_at,omitempty" db:"removed_at"`
}

type Capability struct {
	ID             string           `json:"id" db:"id"`
	SubscriptionID string           `json:"subscription_id" db:"subscription_id"`
	CapabilityType CapabilityType   `json:"capability_type" db:"capability_type"`
	Status         CapabilityStatus `json:"status" db:"status"`
	Disabled       *DisabledState   `json:"disabled,omitempty"`
}

type DisabledState struct {
	Disabled          bool           `json:"disabled" db:"disabled"`
	Reason            DisabledReason `json:"reason" db:"reason"`
	Message           string         `json:"message" db:"message"`
	DisabledAt        *time.Time     `json:"disabled_at,omitempty" db:"disabled_at"`
	FailedEvaluations []string       `json:"failed_evaluations,omitempty" db:"failed_evaluations"`
	CausedByPartyID   *string        `json:"caused_by_party_id,omitempty" db:"caused_by_party_id"`
}

type EvalState struct {
	OverallStatus EvalStatus   `json:"overall_status" db:"overall_status"`
	Results       []EvalResult `json:"results,omitempty"`
	Deferred      []string     `json:"deferred,omitempty"`
	EvaluatedAt   *time.Time   `json:"evaluated_at,omitempty" db:"evaluated_at"`
	LayerIDs      []string     `json:"layer_ids,omitempty" db:"layer_ids"`
}

type EvalResult struct {
	Name               string `json:"name"`
	Passed             bool   `json:"passed"`
	Category           string `json:"category"`
	Severity           string `json:"severity"`
	Resolution         string `json:"resolution,omitempty"`
	ResolutionWorkflow string `json:"resolution_workflow,omitempty"`
}

// ─── Ruleset Validation ───

type RulesetValidation struct {
	Valid           bool     `json:"valid"`
	Errors          []string `json:"errors,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	EvaluationCount int      `json:"evaluation_count"`
	MaxDepth        int      `json:"max_depth"`
}
