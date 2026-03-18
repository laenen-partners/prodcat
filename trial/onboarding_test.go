// Package trial demonstrates the Modular Onboarding Framework (spec 002)
// using the prodcat eligibility engine.
//
// Each test maps to a worked example from spec Section 7, showing how
// the engine evaluates eligibility across the product catalog.
package trial

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/laenen-partners/entitystore"
	"github.com/laenen-partners/prodcat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Seed file paths (relative to trial/ directory).
const (
	// Existing base rulesets from the main module.
	malSeedFile       = "../seed/2026031801_mal_subscription.yaml"
	retailKycSeedFile = "../seed/2026031803_base_retail_kyc.yaml"
	casaSeedFile      = "../seed/2026031802_casa_current_account_uae.yaml"

	// Trial-specific seeds matching the onboarding spec products.
	nonKycProductsSeedFile = "seed/001_non_kyc_products.yaml"
	aedCaSeedFile          = "seed/002_aed_current_account.yaml"
	aedSavingsSeedFile     = "seed/003_aed_savings.yaml"
	usdCaSeedFile          = "seed/004_usd_current_account.yaml"
)

var _sharedConnStr string

func setupEngine(t *testing.T) (*prodcat.Engine, prodcat.SeedTracker) {
	t.Helper()
	ctx := context.Background()

	if _sharedConnStr == "" {
		pg, err := postgres.Run(ctx,
			"pgvector/pgvector:pg17",
			postgres.WithDatabase("trial_test"),
			postgres.WithUsername("test"),
			postgres.WithPassword("test"),
			postgres.BasicWaitStrategies(),
		)
		if err != nil {
			t.Fatalf("start postgres container: %v", err)
		}

		connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatalf("get connection string: %v", err)
		}

		migrationPool, err := pgxpool.New(ctx, connStr)
		if err != nil {
			t.Fatalf("create migration pool: %v", err)
		}
		if err := entitystore.Migrate(ctx, migrationPool); err != nil {
			migrationPool.Close()
			t.Fatalf("migrate: %v", err)
		}
		migrationPool.Close()
		_sharedConnStr = connStr
	}

	pool, err := pgxpool.New(ctx, _sharedConnStr)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	es, err := entitystore.New(entitystore.WithPgStore(pool))
	if err != nil {
		t.Fatalf("create entitystore: %v", err)
	}
	t.Cleanup(es.Close)

	store := prodcat.NewESStore(es)
	engine := prodcat.NewEngine(store)
	tracker := prodcat.NewESSeedTracker(es)

	return engine, tracker
}

func applySeed(t *testing.T, engine *prodcat.Engine, tracker prodcat.SeedTracker, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	err = engine.ApplySeed(context.Background(), path, data, tracker)
	require.NoError(t, err)
}

// seedAllProducts loads base rulesets (from main module) and all trial products.
func seedAllProducts(t *testing.T, engine *prodcat.Engine, tracker prodcat.SeedTracker) {
	t.Helper()

	// Base rulesets from the main module.
	applySeed(t, engine, tracker, malSeedFile)       // base-platform-access
	applySeed(t, engine, tracker, retailKycSeedFile)  // base-retail-idv, screening, address, tax, full-kyc
	applySeed(t, engine, tracker, casaSeedFile)       // base-age-gate-18, base-uae-residence

	// Trial products from the onboarding spec.
	applySeed(t, engine, tracker, nonKycProductsSeedFile) // PFM, Travel Agent + base-privacy-policy
	applySeed(t, engine, tracker, aedCaSeedFile)          // AED Current Account + aed-ca-legal
	applySeed(t, engine, tracker, aedSavingsSeedFile)     // AED Savings + aed-savings-legal
	applySeed(t, engine, tracker, usdCaSeedFile)          // USD Current Account + usd-ca-legal
}

// ─── Test input builders ───

func primaryHolder(opts ...func(*prodcat.EvalPartyInput)) prodcat.EvalPartyInput {
	p := prodcat.EvalPartyInput{
		PartyID: "party-1",
		Type:    prodcat.PartyTypeIndividual,
		Role:    prodcat.PartyRolePrimaryHolder,
	}
	for _, o := range opts {
		o(&p)
	}
	return p
}

func withContacts(contacts ...prodcat.ContactInput) func(*prodcat.EvalPartyInput) {
	return func(p *prodcat.EvalPartyInput) { p.Contacts = contacts }
}

func withAge(age int) func(*prodcat.EvalPartyInput) {
	return func(p *prodcat.EvalPartyInput) { p.Age = age }
}

func withResidence(country string) func(*prodcat.EvalPartyInput) {
	return func(p *prodcat.EvalPartyInput) { p.CountryOfResidence = country }
}

func withDocs(docs ...prodcat.IDDocumentInput) func(*prodcat.EvalPartyInput) {
	return func(p *prodcat.EvalPartyInput) { p.IDDocuments = docs }
}

func withKYC(kyc prodcat.KycChecks) func(*prodcat.EvalPartyInput) {
	return func(p *prodcat.EvalPartyInput) { p.KYC = kyc }
}

func email(verified bool) prodcat.ContactInput {
	return prodcat.ContactInput{Type: 1, Value: "user@mal.app", Primary: true, Verified: verified}
}

func phone(verified bool) prodcat.ContactInput {
	return prodcat.ContactInput{Type: 2, Value: "+971501234567", Primary: true, Verified: verified}
}

func emiratesID(verified bool) prodcat.IDDocumentInput {
	return prodcat.IDDocumentInput{
		DocumentType:   "emirates_id",
		IssuingCountry: "AE",
		Verified:       verified,
	}
}

func usPassport(verified bool) prodcat.IDDocumentInput {
	return prodcat.IDDocumentInput{
		DocumentType:   "passport",
		IssuingCountry: "US",
		Verified:       verified,
	}
}

func fullRetailKYC() prodcat.KycChecks {
	return prodcat.KycChecks{
		LivenessPassed:              true,
		FacialMatchPassed:           true,
		ResidentialAddressValidated: true,
		PEPScreeningClear:           true,
		SanctionsScreeningClear:     true,
		AdverseMediaClear:           true,
		TaxResidencyDeclared:        true,
		FATCADeclared:               true,
		CRSDeclared:                 true,
	}
}

func agreements(types ...string) []prodcat.AgreementInput {
	var out []prodcat.AgreementInput
	for _, t := range types {
		out = append(out, prodcat.AgreementInput{Type: t, Accepted: true})
	}
	return out
}

func requirementsByName(result prodcat.EvaluationResult) map[string]prodcat.RequirementResult {
	m := make(map[string]prodcat.RequirementResult, len(result.Requirements))
	for _, r := range result.Requirements {
		m[r.Name] = r
	}
	return m
}

func pendingRequirements(result prodcat.EvaluationResult) []string {
	var pending []string
	for _, r := range result.Requirements {
		if r.Status == prodcat.RequirementStatusPending {
			pending = append(pending, r.Name)
		}
	}
	return pending
}

// ─── Worked Examples from Onboarding Spec Section 7 ───

// TestPFMOnly — Spec Section 7.4: "User only wants PFM"
//
// Profile Creation → done. Full PFM access. No further onboarding.
// PFM requires no KYC — only platform access (email + phone + general T&C)
// plus privacy policy.
func TestPFMOnly(t *testing.T) {
	engine, tracker := setupEngine(t)
	seedAllProducts(t, engine, tracker)
	ctx := context.Background()

	result, err := engine.Evaluate(ctx, "mal-pfm", prodcat.EvaluationInput{
		Parties:    []prodcat.EvalPartyInput{primaryHolder(withContacts(email(true), phone(true)))},
		Agreements: agreements("general_terms_and_conditions", "privacy_policy"),
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, result.Verdict)

	for _, r := range result.Requirements {
		assert.Equal(t, prodcat.RequirementStatusPassed, r.Status, "requirement %s should pass", r.Name)
	}
	t.Logf("PFM: %s — no KYC required, immediate access after profile creation", result.Verdict)
}

// TestNewUserWantsAEDCurrentAccount — Spec Section 7.1
//
// Progressive onboarding through modules:
//
//	Profile Creation → [taps "Open AED Account"] → evaluateEligibility()
//	→ Plan: [Module 3: Emirates ID + liveness, Module 4: KYC, Module 5: Legal]
//	→ User completes each module → Account activated
func TestNewUserWantsAEDCurrentAccount(t *testing.T) {
	engine, tracker := setupEngine(t)
	seedAllProducts(t, engine, tracker)
	ctx := context.Background()

	// Step 1: Brand new user, no data at all.
	r1, err := engine.Evaluate(ctx, "aed-current-account", prodcat.EvaluationInput{})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, r1.Verdict)
	t.Logf("Step 1 (empty): %s — %d pending", r1.Verdict, len(pendingRequirements(r1)))

	// Step 2: Module 1 complete — profile created + demographics.
	// Platform access gates pass, eligibility gates pass, but KYC and legal pending.
	r2, err := engine.Evaluate(ctx, "aed-current-account", prodcat.EvaluationInput{
		Parties:    []prodcat.EvalPartyInput{primaryHolder(withContacts(email(true), phone(true)), withAge(25), withResidence("AE"))},
		Agreements: agreements("general_terms_and_conditions"),
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, r2.Verdict)
	byName := requirementsByName(r2)
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["email_verified"].Status)
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["phone_verified"].Status)
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["age_18_plus"].Status)
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["uae_resident"].Status)
	t.Logf("Step 2 (profile + demographics): %s — eligibility gates passed, ID/KYC/legal pending", r2.Verdict)

	// Step 3: Module 3 (ID&V) + Module 4 (KYC) complete — only legal pending.
	r3, err := engine.Evaluate(ctx, "aed-current-account", prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{primaryHolder(
			withContacts(email(true), phone(true)),
			withAge(25), withResidence("AE"),
			withDocs(emiratesID(true)),
			withKYC(fullRetailKYC()),
		)},
		Agreements: agreements("general_terms_and_conditions"),
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, r3.Verdict)
	pending := pendingRequirements(r3)
	t.Logf("Step 3 (profile + ID + KYC): %s — pending: %v", r3.Verdict, pending)
	// Only legal agreements should be pending now.
	for _, name := range pending {
		r := requirementsByName(r3)[name]
		assert.Contains(t, []string{"legal", "readiness"}, r.Category,
			"only legal/readiness requirements should be pending, got %s for %s", r.Category, name)
	}

	// Step 4: Module 5 (Legal) — all agreements accepted → eligible.
	r4, err := engine.Evaluate(ctx, "aed-current-account", prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{primaryHolder(
			withContacts(email(true), phone(true)),
			withAge(25), withResidence("AE"),
			withDocs(emiratesID(true)),
			withKYC(fullRetailKYC()),
		)},
		Agreements: agreements("general_terms_and_conditions", "privacy_policy", "aed_ca_tc", "key_facts"),
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, r4.Verdict)
	t.Logf("Step 4 (all complete): %s — account can be activated", r4.Verdict)
}

// TestExistingAEDCAUserWantsSavings — Spec Section 7.2
//
// Existing AED Current Account user wants AED Savings Account.
// All KYC data is already collected. The only delta is:
//   - Savings T&C (new product-specific agreement)
//   - Shariah Commodity Purchase Agreement (savings-only)
//
// Key facts and privacy policy are already accepted → not re-collected.
func TestExistingAEDCAUserWantsSavings(t *testing.T) {
	engine, tracker := setupEngine(t)
	seedAllProducts(t, engine, tracker)
	ctx := context.Background()

	// Full user profile — everything needed for AED CA.
	fullUserParty := primaryHolder(
		withContacts(email(true), phone(true)),
		withAge(25), withResidence("AE"),
		withDocs(emiratesID(true)),
		withKYC(fullRetailKYC()),
	)
	aedCAAgreements := agreements("general_terms_and_conditions", "privacy_policy", "aed_ca_tc", "key_facts")

	// Confirm: user IS eligible for AED CA.
	caResult, err := engine.Evaluate(ctx, "aed-current-account", prodcat.EvaluationInput{
		Parties:    []prodcat.EvalPartyInput{fullUserParty},
		Agreements: aedCAAgreements,
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, caResult.Verdict)
	t.Logf("AED CA: %s", caResult.Verdict)

	// Evaluate AED Savings with the same data — no savings-specific agreements yet.
	savingsResult, err := engine.Evaluate(ctx, "aed-savings", prodcat.EvaluationInput{
		Parties:    []prodcat.EvalPartyInput{fullUserParty},
		Agreements: aedCAAgreements,
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, savingsResult.Verdict)

	byName := requirementsByName(savingsResult)
	// Key facts already accepted (shared) — should pass.
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["key_facts_savings"].Status)
	// Savings-specific agreements are missing.
	assert.Equal(t, prodcat.RequirementStatusPending, byName["savings_tc"].Status)
	assert.Equal(t, prodcat.RequirementStatusPending, byName["shariah_commodity"].Status)
	pending := pendingRequirements(savingsResult)
	t.Logf("AED Savings with AED CA data: %s — pending: %v", savingsResult.Verdict, pending)

	// User accepts savings-specific agreements → eligible.
	savingsResult2, err := engine.Evaluate(ctx, "aed-savings", prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{fullUserParty},
		Agreements: agreements(
			"general_terms_and_conditions", "privacy_policy", "key_facts",
			"savings_tc", "shariah_commodity",
		),
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, savingsResult2.Verdict)
	t.Logf("AED Savings with agreements: %s — only needed savings T&C + Shariah commodity", savingsResult2.Verdict)
}

// TestExistingAEDCAUserWantsUSD — Spec Section 7.3
//
// Existing AED CA user wants USD Account.
// KYC data is already collected. USD CA needs:
//   - USD CA T&C (new product-specific agreement)
//   - No UAE residency gate (different geography)
func TestExistingAEDCAUserWantsUSD(t *testing.T) {
	engine, tracker := setupEngine(t)
	seedAllProducts(t, engine, tracker)
	ctx := context.Background()

	fullUserParty := primaryHolder(
		withContacts(email(true), phone(true)),
		withAge(25), withResidence("AE"),
		withDocs(emiratesID(true)),
		withKYC(fullRetailKYC()),
	)

	// Evaluate USD CA — user already has full KYC from AED CA.
	// Missing: USD CA T&C + key facts for USD.
	usdResult, err := engine.Evaluate(ctx, "usd-current-account", prodcat.EvaluationInput{
		Parties:    []prodcat.EvalPartyInput{fullUserParty},
		Agreements: agreements("general_terms_and_conditions", "privacy_policy"),
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, usdResult.Verdict)
	pending := pendingRequirements(usdResult)
	t.Logf("USD CA with existing KYC: %s — pending: %v", usdResult.Verdict, pending)

	// Accept USD-specific agreements → eligible.
	usdResult2, err := engine.Evaluate(ctx, "usd-current-account", prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{fullUserParty},
		Agreements: agreements(
			"general_terms_and_conditions", "privacy_policy",
			"usd_ca_tc", "key_facts",
		),
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, usdResult2.Verdict)
	t.Logf("USD CA with agreements: %s", usdResult2.Verdict)
}

// TestUnder18Blocked — Spec Section 7.7: Deferred eligibility rule
//
// User requests AED CA → dob reveals age 16 → journey stops:
// "You must be 18+ to open a current account"
// User retains PFM/Travel Agent access.
func TestUnder18Blocked(t *testing.T) {
	engine, tracker := setupEngine(t)
	seedAllProducts(t, engine, tracker)
	ctx := context.Background()

	under18Input := prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{primaryHolder(
			withContacts(email(true), phone(true)),
			withAge(16), withResidence("AE"),
			withDocs(emiratesID(true)),
			withKYC(fullRetailKYC()),
		)},
		Agreements: agreements("general_terms_and_conditions", "privacy_policy", "aed_ca_tc", "key_facts"),
	}

	// AED CA: definitively blocked.
	result, err := engine.Evaluate(ctx, "aed-current-account", under18Input)
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictNotEligible, result.Verdict)
	byName := requirementsByName(result)
	assert.Equal(t, prodcat.RequirementStatusFailed, byName["age_18_plus"].Status)
	assert.Equal(t, prodcat.FailureModeDefinitive, byName["age_18_plus"].FailureMode)
	t.Logf("AED CA under-18: %s — %s", result.Verdict, byName["age_18_plus"].Resolution)

	// PFM: still accessible (no age requirement).
	pfmResult, err := engine.Evaluate(ctx, "mal-pfm", prodcat.EvaluationInput{
		Parties:    []prodcat.EvalPartyInput{primaryHolder(withContacts(email(true), phone(true)))},
		Agreements: agreements("general_terms_and_conditions", "privacy_policy"),
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, pfmResult.Verdict)
	t.Logf("PFM for under-18: %s — non-KYC products unaffected", pfmResult.Verdict)
}

// TestNonResidentBlocked — Spec Section 7 (geo-based blocking)
//
// Non-UAE resident cannot open AED Current Account.
// Definitive failure — no action can resolve this.
func TestNonResidentBlocked(t *testing.T) {
	engine, tracker := setupEngine(t)
	seedAllProducts(t, engine, tracker)
	ctx := context.Background()

	result, err := engine.Evaluate(ctx, "aed-current-account", prodcat.EvaluationInput{
		Parties:    []prodcat.EvalPartyInput{primaryHolder(withContacts(email(true), phone(true)), withAge(30), withResidence("GB"))},
		Agreements: agreements("general_terms_and_conditions"),
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictNotEligible, result.Verdict)
	byName := requirementsByName(result)
	assert.Equal(t, prodcat.RequirementStatusFailed, byName["uae_resident"].Status)
	assert.Equal(t, prodcat.FailureModeDefinitive, byName["uae_resident"].FailureMode)
	t.Logf("Non-UAE resident: %s — %s", result.Verdict, byName["uae_resident"].Resolution)
}

// TestProgressiveUnlock — Spec Core Principle #2
//
// Multi-product eligibility check showing progressive unlock:
//   - Platform access only → PFM + Travel Agent accessible
//   - KYC products remain incomplete until ID&V and KYC modules complete
func TestProgressiveUnlock(t *testing.T) {
	engine, tracker := setupEngine(t)
	seedAllProducts(t, engine, tracker)
	ctx := context.Background()

	// User has completed Module 1 only (profile creation).
	platformOnlyInput := prodcat.EvaluationInput{
		Parties:    []prodcat.EvalPartyInput{primaryHolder(withContacts(email(true), phone(true)), withAge(25), withResidence("AE"))},
		Agreements: agreements("general_terms_and_conditions", "privacy_policy"),
	}

	report, err := engine.CheckEligibility(ctx,
		[]string{"mal-pfm", "mal-travel-agent", "aed-current-account", "aed-savings", "usd-current-account"},
		platformOnlyInput,
	)
	require.NoError(t, err)

	byProduct := make(map[string]prodcat.EvaluationResult)
	for _, r := range report.Results {
		byProduct[r.ProductID] = r
	}

	// Non-KYC products: eligible immediately after profile creation.
	assert.Equal(t, prodcat.EligibilityVerdictEligible, byProduct["mal-pfm"].Verdict)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, byProduct["mal-travel-agent"].Verdict)

	// KYC products: incomplete — need ID, KYC, and legal agreements.
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, byProduct["aed-current-account"].Verdict)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, byProduct["aed-savings"].Verdict)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, byProduct["usd-current-account"].Verdict)

	t.Log("Progressive unlock (platform access only):")
	for _, pid := range []string{"mal-pfm", "mal-travel-agent", "aed-current-account", "aed-savings", "usd-current-account"} {
		r := byProduct[pid]
		t.Logf("  %-25s → %-12s (pending: %d)", pid, r.Verdict, len(pendingRequirements(r)))
	}
}

// TestSubscriptionLifecycle — Spec Section 6 (State Machine)
//
// Demonstrates the subscription lifecycle:
//
//	No User → Profile Only → Onboarding In Progress → Product Active → (Degraded → Restored)
func TestSubscriptionLifecycle(t *testing.T) {
	engine, tracker := setupEngine(t)
	seedAllProducts(t, engine, tracker)
	ctx := context.Background()

	// Step 1: User subscribes (creates subscription in incomplete state).
	sub, err := engine.Subscribe(ctx, prodcat.SubscribeRequest{
		EntityID: "customer-001", EntityType: prodcat.EntityTypeIndividual,
		ProductID: "aed-current-account",
		Parties:   []prodcat.SubscribePartyInput{{CustomerID: "customer-001", Role: prodcat.PartyRolePrimaryHolder}},
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.SubscriptionStatusIncomplete, sub.Status)
	t.Logf("Subscription created: %s (status: %s)", sub.ID, sub.Status)

	// Step 2: User completes onboarding → activate with capabilities.
	sub, err = engine.Activate(ctx, sub.ID, "core-banking-ref-001", []prodcat.CapabilityType{
		prodcat.CapabilityTypeView,
		prodcat.CapabilityTypeDomesticTransfers,
		prodcat.CapabilityTypeReceive,
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.SubscriptionStatusActive, sub.Status)
	assert.Len(t, sub.Capabilities, 3)
	t.Logf("Subscription activated: %s (capabilities: %d)", sub.Status, len(sub.Capabilities))

	// Step 3: Emirates ID expires → disable subscription (degradation).
	sub, err = engine.Disable(ctx, sub.ID, prodcat.DisabledReasonExpiredData, "Emirates ID expired")
	require.NoError(t, err)
	require.NotNil(t, sub.Disabled)
	assert.True(t, sub.Disabled.Disabled)
	assert.Equal(t, prodcat.DisabledReasonExpiredData, sub.Disabled.Reason)
	t.Logf("Subscription disabled: %s — %s", sub.Disabled.Reason, sub.Disabled.Message)

	// Step 4: User refreshes ID → re-enable subscription.
	sub, err = engine.Enable(ctx, sub.ID)
	require.NoError(t, err)
	assert.Nil(t, sub.Disabled)
	assert.Equal(t, prodcat.SubscriptionStatusActive, sub.Status)
	t.Logf("Subscription restored: %s", sub.Status)
}
