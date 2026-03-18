// Package prodcat is an eligibility engine for digital banking.
// It determines who can have which products, what they still need to do,
// and whether anything has changed that affects their access.
package prodcat

import "time"

// ─── Product Enums ───

// ProductStatus tracks the product lifecycle.
type ProductStatus string

const (
	ProductStatusDraft      ProductStatus = "draft"
	ProductStatusActive     ProductStatus = "active"
	ProductStatusSuspended  ProductStatus = "suspended"
	ProductStatusDeprecated ProductStatus = "deprecated"
	ProductStatusRetired    ProductStatus = "retired"
)

// AvailabilityMode defines how geographic availability is interpreted.
type AvailabilityMode string

const (
	AvailabilityModeSpecificCountries AvailabilityMode = "specific_countries"
	AvailabilityModeGlobal            AvailabilityMode = "global"
	AvailabilityModeGlobalExcept      AvailabilityMode = "global_except"
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

// ─── Evaluation Enums ───

// EligibilityVerdict is the overall outcome of an eligibility evaluation.
// It distinguishes between "definitely no", "need more data", and "yes".
type EligibilityVerdict string

const (
	// EligibilityVerdictEligible means all blocking requirements are met.
	EligibilityVerdictEligible EligibilityVerdict = "eligible"

	// EligibilityVerdictNotEligible means at least one blocking requirement
	// definitively failed — the customer cannot have this product regardless
	// of what additional data they provide.
	// Example: age < 18 for investment products, blocked nationality.
	EligibilityVerdictNotEligible EligibilityVerdict = "not_eligible"

	// EligibilityVerdictIncomplete means no blocking requirements have
	// definitively failed, but some cannot be evaluated yet because the
	// required input data is missing or unverified.
	// Example: email not yet verified, ID document not yet uploaded.
	EligibilityVerdictIncomplete EligibilityVerdict = "incomplete"
)

// RequirementStatus is the outcome of a single evaluation rule.
type RequirementStatus string

const (
	// RequirementStatusPassed means the requirement is met.
	RequirementStatusPassed RequirementStatus = "passed"

	// RequirementStatusFailed means the requirement is definitively not met.
	// The customer's data was evaluated and they don't qualify.
	// Only produced by rules with failure_mode "definitive".
	RequirementStatusFailed RequirementStatus = "failed"

	// RequirementStatusPending means the requirement is not yet met but
	// can still be resolved — by the customer, by providing data, or by
	// a human reviewer.
	RequirementStatusPending RequirementStatus = "pending"
)

// FailureMode declares what happens when a rule evaluates to false.
// It tells the caller who needs to act and how.
type FailureMode string

const (
	// FailureModeActionable means the customer can self-serve to resolve it.
	// Example: verify email, accept T&C.
	FailureModeActionable FailureMode = "actionable"

	// FailureModeInputRequired means the system needs more data from
	// the customer before the rule can be evaluated.
	// Example: upload ID document, provide nationality.
	FailureModeInputRequired FailureMode = "input_required"

	// FailureModeManualReview means a human (compliance officer, ops)
	// must make a decision. The system cannot auto-resolve.
	// Example: PEPs screening, sanctions check, source of funds review.
	FailureModeManualReview FailureMode = "manual_review"

	// FailureModeDefinitive means the customer is disqualified.
	// No action can change this outcome.
	// Example: age < 18, blocked jurisdiction.
	FailureModeDefinitive FailureMode = "definitive"
)

// ─── Domain Types ───

// ProductEligibility is a product's eligibility configuration.
// The product's operational details (fees, rates, limits) live in core banking.
type ProductEligibility struct {
	ProductID       string           `json:"product_id"`
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	Tags            []string         `json:"tags"`
	Status          ProductStatus    `json:"status"`
	CurrencyCode    string           `json:"currency_code,omitempty"`
	ParentProductID string           `json:"parent_product_id,omitempty"`
	ShariaCompliant bool             `json:"sharia_compliant"`
	Availability    GeoAvailability  `json:"availability"`
	BaseRulesetIDs  []string         `json:"base_ruleset_ids,omitempty"`
	Ruleset         []byte           `json:"ruleset,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// GeoAvailability defines geographic availability.
type GeoAvailability struct {
	Mode         AvailabilityMode `json:"mode"`
	CountryCodes []string         `json:"country_codes,omitempty"`
}

// BaseRuleset is a reusable eval engine ruleset.
type BaseRuleset struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Content     []byte    `json:"content"`
	Version     string    `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ─── Subscription Types ───

// Subscription is a customer's instance of a product.
type Subscription struct {
	ID           string             `json:"id"`
	ProductID    string             `json:"product_id"`
	EntityID     string             `json:"entity_id"`
	EntityType   EntityType         `json:"entity_type"`
	Status       SubscriptionStatus `json:"status"`
	SigningRule   SigningRule        `json:"signing_rule"`
	RequiredCount int               `json:"required_count"`
	ExternalRef  string             `json:"external_ref,omitempty"`
	Disabled     *DisabledState     `json:"disabled,omitempty"`
	Parties      []Party            `json:"parties,omitempty"`
	Capabilities []Capability       `json:"capabilities,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	ActivatedAt  *time.Time         `json:"activated_at,omitempty"`
	CanceledAt   *time.Time         `json:"canceled_at,omitempty"`
}

// Party represents a person's role on a subscription.
type Party struct {
	ID              string         `json:"id"`
	CustomerID      string         `json:"customer_id"`
	Role            PartyRole      `json:"role"`
	RequirementsMet bool           `json:"requirements_met"`
	Disabled        *DisabledState `json:"disabled,omitempty"`
}

// Capability represents a product feature that can be independently controlled.
type Capability struct {
	ID             string           `json:"id"`
	CapabilityType CapabilityType   `json:"capability_type"`
	Status         CapabilityStatus `json:"status"`
	Disabled       *DisabledState   `json:"disabled,omitempty"`
}

// DisabledState explains why something is disabled.
type DisabledState struct {
	Disabled          bool           `json:"disabled"`
	Reason            DisabledReason `json:"reason"`
	Message           string         `json:"message"`
	DisabledAt        *time.Time     `json:"disabled_at,omitempty"`
	FailedEvaluations []string       `json:"failed_evaluations,omitempty"`
}

// ─── Evaluation Types ───

// EvaluationResult is the output of an eligibility evaluation.
type EvaluationResult struct {
	ProductID    string             `json:"product_id"`
	Verdict      EligibilityVerdict `json:"verdict"`
	Requirements []RequirementResult `json:"requirements"`
	ResolvedAt   time.Time          `json:"resolved_at"`
}

// RequirementResult is the outcome of a single evaluation rule.
type RequirementResult struct {
	Name        string            `json:"name"`
	Status      RequirementStatus `json:"status"`
	FailureMode FailureMode       `json:"failure_mode"`
	Category    string            `json:"category"`
	Severity    string            `json:"severity"`
	Resolution  string            `json:"resolution,omitempty"`
}

// ResolvedRuleset is the merged ruleset for a product.
type ResolvedRuleset struct {
	ProductID  string         `json:"product_id"`
	Merged     []byte         `json:"merged"`
	Layers     []RulesetLayer `json:"layers"`
}

// RulesetLayer identifies one component of a merged ruleset.
type RulesetLayer struct {
	Source   string `json:"source"` // "base" or "product"
	SourceID string `json:"source_id"`
}
