package prodcat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// Engine implements EligibilityService and SubscriptionService
// on top of a Store (which will be backed by entitystore).
type Engine struct {
	store Store
}

// Verify interface compliance.
var (
	_ EligibilityService  = (*Engine)(nil)
	_ SubscriptionService = (*Engine)(nil)
)

// NewEngine creates a new eligibility engine backed by the given store.
func NewEngine(s Store) *Engine {
	return &Engine{store: s}
}

// ─── EligibilityService ───

func (e *Engine) RegisterProduct(ctx context.Context, p ProductEligibility) error {
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Status == "" {
		p.Status = ProductStatusDraft
	}
	return e.store.PutProduct(ctx, p)
}

func (e *Engine) GetProduct(ctx context.Context, productID string) (ProductEligibility, error) {
	return e.store.GetProduct(ctx, productID)
}

func (e *Engine) ListProducts(ctx context.Context, filter TagFilter) ([]ProductEligibility, error) {
	return e.store.ListProducts(ctx, filter)
}

func (e *Engine) UpdateProduct(ctx context.Context, p ProductEligibility) error {
	p.UpdatedAt = time.Now().UTC()
	return e.store.PutProduct(ctx, p)
}

func (e *Engine) CreateRuleset(ctx context.Context, r BaseRuleset) (BaseRuleset, error) {
	now := time.Now().UTC()
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	r.CreatedAt = now
	r.UpdatedAt = now
	if err := e.store.PutRuleset(ctx, r); err != nil {
		return BaseRuleset{}, err
	}
	return r, nil
}

func (e *Engine) GetRuleset(ctx context.Context, id string) (BaseRuleset, error) {
	return e.store.GetRuleset(ctx, id)
}

func (e *Engine) ListRulesets(ctx context.Context) ([]BaseRuleset, error) {
	return e.store.ListRulesets(ctx)
}

// Evaluate runs the eligibility rules for a product against the given input.
// It resolves the product's ruleset (base + product-specific), then evaluates
// each rule. The result distinguishes between:
//   - eligible: all blocking requirements pass
//   - not_eligible: at least one blocking requirement definitively fails
//   - incomplete: no definitive failures, but some rules can't be evaluated
//     because the required input data is missing
func (e *Engine) Evaluate(ctx context.Context, productID string, input EvaluationInput) (EvaluationResult, error) {
	resolved, err := e.ResolveRuleset(ctx, productID)
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("resolve ruleset: %w", err)
	}

	var ruleset rulesetYAML
	if err := yaml.Unmarshal(resolved.Merged, &ruleset); err != nil {
		return EvaluationResult{}, fmt.Errorf("parse ruleset: %w", err)
	}

	results := make([]RequirementResult, 0, len(ruleset.Evaluations))
	state := make(map[string]ruleState)

	for _, eval := range ruleset.Evaluations {
		status, passed := evaluateRule(eval, input, state)
		state[eval.Writes] = ruleState{passed: passed, pending: status == RequirementStatusPending}

		results = append(results, RequirementResult{
			Name:        eval.Name,
			Status:      status,
			FailureMode: resolveFailureMode(eval.FailureMode),
			Category:    eval.Category,
			Severity:    eval.Severity,
			Resolution:  eval.Resolution,
		})
	}

	verdict := deriveVerdict(results)

	return EvaluationResult{
		ProductID:    productID,
		Verdict:      verdict,
		Requirements: results,
		ResolvedAt:   time.Now().UTC(),
	}, nil
}

// CheckEligibility evaluates eligibility across multiple products for the same input.
// Products that fail to resolve (e.g. not found) are skipped with a not_eligible verdict.
func (e *Engine) CheckEligibility(ctx context.Context, productIDs []string, input EvaluationInput) (EligibilityReport, error) {
	results := make([]EvaluationResult, 0, len(productIDs))

	for _, pid := range productIDs {
		result, err := e.Evaluate(ctx, pid, input)
		if err != nil {
			results = append(results, EvaluationResult{
				ProductID:  pid,
				Verdict:    EligibilityVerdictNotEligible,
				ResolvedAt: time.Now().UTC(),
			})
			continue
		}
		results = append(results, result)
	}

	return EligibilityReport{
		Results:    results,
		ResolvedAt: time.Now().UTC(),
	}, nil
}

// ResolveRuleset merges base rulesets + product-specific ruleset.
func (e *Engine) ResolveRuleset(ctx context.Context, productID string) (ResolvedRuleset, error) {
	product, err := e.store.GetProduct(ctx, productID)
	if err != nil {
		return ResolvedRuleset{}, fmt.Errorf("get product: %w", err)
	}

	var merged rulesetYAML
	var layers []RulesetLayer

	for _, baseID := range product.BaseRulesetIDs {
		base, err := e.store.GetRuleset(ctx, baseID)
		if err != nil {
			return ResolvedRuleset{}, fmt.Errorf("get base ruleset %s: %w", baseID, err)
		}
		var baseRuleset rulesetYAML
		if err := yaml.Unmarshal(base.Content, &baseRuleset); err != nil {
			return ResolvedRuleset{}, fmt.Errorf("parse base ruleset %s: %w", baseID, err)
		}
		merged.Evaluations = append(merged.Evaluations, baseRuleset.Evaluations...)
		layers = append(layers, RulesetLayer{Source: "base", SourceID: baseID})
	}

	if len(product.Ruleset) > 0 {
		var productRuleset rulesetYAML
		if err := yaml.Unmarshal(product.Ruleset, &productRuleset); err != nil {
			return ResolvedRuleset{}, fmt.Errorf("parse product ruleset: %w", err)
		}
		merged.Evaluations = append(merged.Evaluations, productRuleset.Evaluations...)
		layers = append(layers, RulesetLayer{Source: "product", SourceID: productID})
	}

	mergedBytes, err := yaml.Marshal(&merged)
	if err != nil {
		return ResolvedRuleset{}, fmt.Errorf("marshal merged ruleset: %w", err)
	}

	return ResolvedRuleset{
		ProductID: productID,
		Merged:    mergedBytes,
		Layers:    layers,
	}, nil
}

// ─── SubscriptionService ───

func (e *Engine) Subscribe(ctx context.Context, req SubscribeRequest) (Subscription, error) {
	now := time.Now().UTC()
	sub := Subscription{
		ID:            uuid.NewString(),
		ProductID:     req.ProductID,
		EntityID:      req.EntityID,
		EntityType:    req.EntityType,
		Status:        SubscriptionStatusIncomplete,
		SigningRule:   SigningRuleAnyOne,
		RequiredCount: 1,
		CreatedAt:     now,
	}

	for _, p := range req.Parties {
		sub.Parties = append(sub.Parties, Party{
			ID:         uuid.NewString(),
			CustomerID: p.CustomerID,
			Role:       p.Role,
		})
	}

	if err := e.store.PutSubscription(ctx, sub); err != nil {
		return Subscription{}, err
	}
	return sub, nil
}

func (e *Engine) GetSubscription(ctx context.Context, id string) (Subscription, error) {
	return e.store.GetSubscription(ctx, id)
}

func (e *Engine) ListSubscriptions(ctx context.Context, filter SubscriptionFilter) ([]Subscription, error) {
	return e.store.ListSubscriptions(ctx, filter)
}

func (e *Engine) Activate(ctx context.Context, id string, externalRef string, capabilities []CapabilityType) (Subscription, error) {
	sub, err := e.store.GetSubscription(ctx, id)
	if err != nil {
		return Subscription{}, err
	}

	now := time.Now().UTC()
	sub.Status = SubscriptionStatusActive
	sub.ExternalRef = externalRef
	sub.ActivatedAt = &now

	for _, ct := range capabilities {
		sub.Capabilities = append(sub.Capabilities, Capability{
			ID:             uuid.NewString(),
			CapabilityType: ct,
			Status:         CapabilityStatusActive,
		})
	}

	if err := e.store.PutSubscription(ctx, sub); err != nil {
		return Subscription{}, err
	}
	return sub, nil
}

func (e *Engine) Cancel(ctx context.Context, id string, reason string) (Subscription, error) {
	sub, err := e.store.GetSubscription(ctx, id)
	if err != nil {
		return Subscription{}, err
	}

	now := time.Now().UTC()
	sub.Status = SubscriptionStatusCanceled
	sub.CanceledAt = &now

	if err := e.store.PutSubscription(ctx, sub); err != nil {
		return Subscription{}, err
	}
	return sub, nil
}

func (e *Engine) Disable(ctx context.Context, id string, reason DisabledReason, message string) (Subscription, error) {
	sub, err := e.store.GetSubscription(ctx, id)
	if err != nil {
		return Subscription{}, err
	}

	now := time.Now().UTC()
	sub.Disabled = &DisabledState{
		Disabled:   true,
		Reason:     reason,
		Message:    message,
		DisabledAt: &now,
	}

	if err := e.store.PutSubscription(ctx, sub); err != nil {
		return Subscription{}, err
	}
	return sub, nil
}

func (e *Engine) Enable(ctx context.Context, id string) (Subscription, error) {
	sub, err := e.store.GetSubscription(ctx, id)
	if err != nil {
		return Subscription{}, err
	}

	sub.Disabled = nil

	if err := e.store.PutSubscription(ctx, sub); err != nil {
		return Subscription{}, err
	}
	return sub, nil
}

func (e *Engine) DisableCapability(ctx context.Context, subID string, capID string, reason DisabledReason, message string) (Subscription, error) {
	sub, err := e.store.GetSubscription(ctx, subID)
	if err != nil {
		return Subscription{}, err
	}

	now := time.Now().UTC()
	for i := range sub.Capabilities {
		if sub.Capabilities[i].ID == capID {
			sub.Capabilities[i].Status = CapabilityStatusDisabled
			sub.Capabilities[i].Disabled = &DisabledState{
				Disabled:   true,
				Reason:     reason,
				Message:    message,
				DisabledAt: &now,
			}
			break
		}
	}

	if err := e.store.PutSubscription(ctx, sub); err != nil {
		return Subscription{}, err
	}
	return sub, nil
}

func (e *Engine) EnableCapability(ctx context.Context, subID string, capID string) (Subscription, error) {
	sub, err := e.store.GetSubscription(ctx, subID)
	if err != nil {
		return Subscription{}, err
	}

	for i := range sub.Capabilities {
		if sub.Capabilities[i].ID == capID {
			sub.Capabilities[i].Status = CapabilityStatusActive
			sub.Capabilities[i].Disabled = nil
			break
		}
	}

	if err := e.store.PutSubscription(ctx, sub); err != nil {
		return Subscription{}, err
	}
	return sub, nil
}

func (e *Engine) AddParty(ctx context.Context, subID string, customerID string, role PartyRole) (Subscription, error) {
	sub, err := e.store.GetSubscription(ctx, subID)
	if err != nil {
		return Subscription{}, err
	}

	sub.Parties = append(sub.Parties, Party{
		ID:         uuid.NewString(),
		CustomerID: customerID,
		Role:       role,
	})

	if err := e.store.PutSubscription(ctx, sub); err != nil {
		return Subscription{}, err
	}
	return sub, nil
}

func (e *Engine) RemoveParty(ctx context.Context, subID string, partyID string) (Subscription, error) {
	sub, err := e.store.GetSubscription(ctx, subID)
	if err != nil {
		return Subscription{}, err
	}

	parties := make([]Party, 0, len(sub.Parties))
	for _, p := range sub.Parties {
		if p.ID != partyID {
			parties = append(parties, p)
		}
	}
	sub.Parties = parties

	if err := e.store.PutSubscription(ctx, sub); err != nil {
		return Subscription{}, err
	}
	return sub, nil
}

// ─── Internal: Ruleset parsing and evaluation ───

type rulesetYAML struct {
	Evaluations []evaluationYAML `yaml:"evaluations"`
}

type evaluationYAML struct {
	Name       string   `yaml:"name"`
	Expression string   `yaml:"expression"`
	Reads      []string `yaml:"reads,omitempty"`
	Writes     string   `yaml:"writes"`
	Severity   string   `yaml:"severity"`
	Category   string   `yaml:"category"`
	Resolution string   `yaml:"resolution,omitempty"`
	// FailureMode controls what happens when the rule evaluates to false:
	//   "actionable"     — customer self-service (verify email, accept T&C)
	//   "input_required" — customer must provide data (upload ID, enter nationality)
	//   "manual_review"  — human must decide (PEPs, sanctions, compliance)
	//   "definitive"     — hard reject, nothing can fix it (age, jurisdiction)
	// Default (empty) is treated as "actionable".
	FailureMode string `yaml:"failure_mode,omitempty"`
}

// evaluateRule runs a single evaluation rule against the input.
// It returns the requirement status and whether the rule passed.
//
// Rules that depend on other rules (via reads) check the state map.
// Rules that check input data use a simplified matcher — the full CEL
// evaluation will be delegated to the eval engine once integrated.
type ruleState struct {
	passed  bool
	pending bool
}

func evaluateRule(eval evaluationYAML, input EvaluationInput, state map[string]ruleState) (RequirementStatus, bool) {
	// If this rule depends on other rules (reads), check if all dependencies passed.
	if len(eval.Reads) > 0 {
		allPassed := true
		anyPending := false
		for _, dep := range eval.Reads {
			v, ok := state[dep]
			if !ok {
				anyPending = true
				continue
			}
			if v.pending {
				anyPending = true
				continue
			}
			if !v.passed {
				allPassed = false
			}
		}
		if anyPending {
			return RequirementStatusPending, false
		}
		if !allPassed {
			return RequirementStatusFailed, false
		}
		return RequirementStatusPassed, true
	}

	// For leaf rules, use simplified matching based on the rule name/expression.
	// This is a bridge until the full eval engine (CEL) is integrated.
	passed, dataPresent := matchRule(eval, input)
	if !dataPresent {
		return RequirementStatusPending, false
	}
	if passed {
		return RequirementStatusPassed, true
	}
	// Rule failed. Is this a definitive disqualification or an actionable step?
	// "definitive" means the customer can't fix it (e.g., age, nationality).
	// Default ("actionable" or empty) means the customer can still act.
	if eval.FailureMode == "definitive" {
		return RequirementStatusFailed, false
	}
	return RequirementStatusPending, false
}

// primaryParty returns the primary holder from the input.
// If no primary holder exists, returns the first party.
// If no parties exist, returns nil.
func primaryParty(input EvaluationInput) *EvalPartyInput {
	for i := range input.Parties {
		if input.Parties[i].Role == PartyRolePrimaryHolder {
			return &input.Parties[i]
		}
	}
	if len(input.Parties) > 0 {
		return &input.Parties[0]
	}
	return nil
}

// matchRule is a simplified rule matcher for common patterns.
// Returns (passed, dataPresent).
func matchRule(eval evaluationYAML, input EvaluationInput) (bool, bool) {
	switch eval.Category {
	case "contact":
		return matchContactRule(eval, input)
	case "legal":
		return matchLegalRule(eval, input)
	case "identity":
		return matchIdentityRule(eval, input)
	case "eligibility":
		return matchEligibilityRule(eval, input)
	case "kyc":
		return matchKycRule(eval, input)
	default:
		return false, false
	}
}

func matchContactRule(eval evaluationYAML, input EvaluationInput) (bool, bool) {
	p := primaryParty(input)
	if p == nil || len(p.Contacts) == 0 {
		return false, false
	}

	switch eval.Writes {
	case "email_verified":
		for _, c := range p.Contacts {
			if c.Type == 1 && c.Primary {
				return c.Verified, true
			}
		}
		return false, false
	case "phone_verified":
		for _, c := range p.Contacts {
			if c.Type == 2 && c.Primary {
				return c.Verified, true
			}
		}
		return false, false
	}
	return false, false
}

func matchLegalRule(eval evaluationYAML, input EvaluationInput) (bool, bool) {
	if len(input.Agreements) == 0 {
		return false, false
	}

	agreementTypes := map[string]string{
		"general_tc_accepted": "general_terms_and_conditions",
		"casa_tc_accepted":    "casa_terms_and_conditions",
	}

	expectedType, ok := agreementTypes[eval.Writes]
	if !ok {
		return false, false
	}

	for _, a := range input.Agreements {
		if a.Type == expectedType {
			return a.Accepted, true
		}
	}
	return false, false
}

func matchEligibilityRule(eval evaluationYAML, input EvaluationInput) (bool, bool) {
	p := primaryParty(input)

	switch eval.Writes {
	case "age_eligible":
		if p == nil || p.Age == 0 {
			return false, false
		}
		return p.Age >= 18, true
	case "uae_resident":
		if p == nil || p.CountryOfResidence == "" {
			return false, false
		}
		return p.CountryOfResidence == "AE", true
	case "nationality_eligible":
		if p == nil || len(p.Nationalities) == 0 {
			return false, false
		}
		return true, true
	}
	return false, false
}

func matchIdentityRule(eval evaluationYAML, input EvaluationInput) (bool, bool) {
	p := primaryParty(input)
	if p == nil || len(p.IDDocuments) == 0 {
		return false, false
	}

	switch eval.Writes {
	case "valid_passport":
		for _, doc := range p.IDDocuments {
			if doc.DocumentType == "passport" {
				return doc.Verified && !doc.Expired, true
			}
		}
		return false, false
	case "uae_pass_verified":
		for _, doc := range p.IDDocuments {
			if doc.DocumentType == "uae_pass" {
				return doc.Verified, true
			}
		}
		return false, false
	default:
		for _, doc := range p.IDDocuments {
			if doc.Verified && !doc.Expired {
				return true, true
			}
		}
		return false, true
	}
}

func matchKycRule(eval evaluationYAML, input EvaluationInput) (bool, bool) {
	p := primaryParty(input)
	if p == nil {
		return false, false
	}
	kyc := p.KYC

	switch eval.Writes {
	case "liveness_passed":
		return kyc.LivenessPassed, true
	case "facial_match_passed":
		return kyc.FacialMatchPassed, true
	case "residential_address_validated":
		return kyc.ResidentialAddressValidated, true
	case "pep_screening_clear":
		return kyc.PEPScreeningClear, true
	case "sanctions_screening_clear":
		return kyc.SanctionsScreeningClear, true
	case "adverse_media_clear":
		return kyc.AdverseMediaClear, true
	case "source_of_funds_verified":
		return kyc.SourceOfFundsVerified, true
	case "source_of_wealth_verified":
		return kyc.SourceOfWealthVerified, true
	case "tax_residency_declared":
		return kyc.TaxResidencyDeclared, true
	case "fatca_declared":
		return kyc.FATCADeclared, true
	case "crs_declared":
		return kyc.CRSDeclared, true
	case "employment_verified":
		return kyc.EmploymentVerified, true
	}
	return false, false
}

// resolveFailureMode converts the YAML string to the domain type.
// Empty defaults to "actionable".
func resolveFailureMode(s string) FailureMode {
	switch FailureMode(s) {
	case FailureModeActionable, FailureModeInputRequired, FailureModeManualReview, FailureModeDefinitive:
		return FailureMode(s)
	default:
		return FailureModeActionable
	}
}

// deriveVerdict determines the overall eligibility verdict from individual results.
func deriveVerdict(results []RequirementResult) EligibilityVerdict {
	hasBlockingFailed := false
	hasBlockingPending := false

	for _, r := range results {
		if r.Severity != "blocking" {
			continue
		}
		switch r.Status {
		case RequirementStatusFailed:
			hasBlockingFailed = true
		case RequirementStatusPending:
			hasBlockingPending = true
		}
	}

	if hasBlockingFailed {
		return EligibilityVerdictNotEligible
	}
	if hasBlockingPending {
		return EligibilityVerdictIncomplete
	}
	return EligibilityVerdictEligible
}
