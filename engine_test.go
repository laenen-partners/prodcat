package prodcat_test

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

const (
	malSeedFile  = "seed/2026031801_mal_subscription.yaml"
	casaSeedFile = "seed/2026031802_casa_current_account_uae.yaml"
)

var _sharedConnStr string

func sharedTestEngine(t *testing.T) (*prodcat.Engine, prodcat.SeedTracker) {
	t.Helper()
	ctx := context.Background()

	if _sharedConnStr == "" {
		pg, err := postgres.Run(ctx,
			"pgvector/pgvector:pg17",
			postgres.WithDatabase("prodcat_test"),
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

func seedMalSubscription(t *testing.T, engine *prodcat.Engine, tracker prodcat.SeedTracker) {
	t.Helper()
	data, err := os.ReadFile(malSeedFile)
	require.NoError(t, err)
	err = engine.ApplySeed(context.Background(), malSeedFile, data, tracker)
	require.NoError(t, err)
}

func seedCASA(t *testing.T, engine *prodcat.Engine, tracker prodcat.SeedTracker) {
	t.Helper()
	seedMalSubscription(t, engine, tracker)
	data, err := os.ReadFile(casaSeedFile)
	require.NoError(t, err)
	err = engine.ApplySeed(context.Background(), casaSeedFile, data, tracker)
	require.NoError(t, err)
}

func requirementsByName(result prodcat.EvaluationResult) map[string]prodcat.RequirementResult {
	m := make(map[string]prodcat.RequirementResult, len(result.Requirements))
	for _, r := range result.Requirements {
		m[r.Name] = r
	}
	return m
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
	return prodcat.ContactInput{Type: 1, Value: "user@example.com", Primary: true, Verified: verified}
}

func phone(verified bool) prodcat.ContactInput {
	return prodcat.ContactInput{Type: 2, Value: "+971501234567", Primary: true, Verified: verified}
}

func passport(verified bool) prodcat.IDDocumentInput {
	return prodcat.IDDocumentInput{DocumentType: "passport", IssuingCountry: "AE", Verified: verified}
}

func uaePass(verified bool) prodcat.IDDocumentInput {
	return prodcat.IDDocumentInput{DocumentType: "uae_pass", IssuingCountry: "AE", Verified: verified}
}

func agreements(types ...string) []prodcat.AgreementInput {
	var out []prodcat.AgreementInput
	for _, t := range types {
		out = append(out, prodcat.AgreementInput{Type: t, Accepted: true})
	}
	return out
}

// ─── Mal Subscription Tests ───

func TestEvaluate_MalSubscription_AllPassed(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedMalSubscription(t, engine, tracker)

	result, err := engine.Evaluate(context.Background(), "mal-subscription", prodcat.EvaluationInput{
		Parties:    []prodcat.EvalPartyInput{primaryHolder(withContacts(email(true), phone(true)))},
		Agreements: agreements("general_terms_and_conditions"),
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, result.Verdict)
	for _, r := range result.Requirements {
		assert.Equal(t, prodcat.RequirementStatusPassed, r.Status, "requirement %s should pass", r.Name)
	}
}

func TestEvaluate_MalSubscription_Incomplete(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedMalSubscription(t, engine, tracker)

	result, err := engine.Evaluate(context.Background(), "mal-subscription", prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{primaryHolder(withContacts(email(true)))},
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, result.Verdict)
	byName := requirementsByName(result)
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["email_verified"].Status)
	assert.Equal(t, prodcat.RequirementStatusPending, byName["phone_verified"].Status)
	assert.Equal(t, prodcat.RequirementStatusPending, byName["general_tc_accepted"].Status)
}

func TestEvaluate_MalSubscription_ActionableFailure(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedMalSubscription(t, engine, tracker)

	result, err := engine.Evaluate(context.Background(), "mal-subscription", prodcat.EvaluationInput{
		Parties:    []prodcat.EvalPartyInput{primaryHolder(withContacts(email(false), phone(true)))},
		Agreements: agreements("general_terms_and_conditions"),
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, result.Verdict)
	byName := requirementsByName(result)
	assert.Equal(t, prodcat.RequirementStatusPending, byName["email_verified"].Status)
}

func TestEvaluate_EmptyInput(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedMalSubscription(t, engine, tracker)

	result, err := engine.Evaluate(context.Background(), "mal-subscription", prodcat.EvaluationInput{})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, result.Verdict)
	for _, r := range result.Requirements {
		assert.Equal(t, prodcat.RequirementStatusPending, r.Status)
	}
}

// ─── Failure Mode Tests ───

func TestEvaluate_DefinitiveFailure(t *testing.T) {
	engine, _ := sharedTestEngine(t)
	ctx := context.Background()

	rs, err := engine.CreateRuleset(ctx, prodcat.BaseRuleset{
		Name: "age-gate", Version: "1.0.0",
		Content: []byte(`evaluations:
  - name: age_gate
    expression: "input.age >= 18"
    writes: age_eligible
    severity: blocking
    category: eligibility
    failure_mode: definitive
`),
	})
	require.NoError(t, err)

	err = engine.RegisterProduct(ctx, prodcat.ProductEligibility{
		ProductID: "investment-fund", Name: "Investment Fund",
		Status: prodcat.ProductStatusActive, BaseRulesetIDs: []string{rs.ID},
	})
	require.NoError(t, err)

	result, err := engine.Evaluate(ctx, "investment-fund", prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{primaryHolder(withAge(16))},
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictNotEligible, result.Verdict)

	result, err = engine.Evaluate(ctx, "investment-fund", prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{primaryHolder(withAge(21))},
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, result.Verdict)
}

func TestEvaluate_FailureModes(t *testing.T) {
	engine, _ := sharedTestEngine(t)
	ctx := context.Background()

	rs, err := engine.CreateRuleset(ctx, prodcat.BaseRuleset{
		Name: "all-modes", Version: "1.0.0",
		Content: []byte(`evaluations:
  - name: email_verified
    writes: email_verified
    severity: blocking
    category: contact
    failure_mode: actionable
  - name: id_document_uploaded
    writes: id_uploaded
    severity: blocking
    category: identity
    failure_mode: input_required
  - name: peps_cleared
    writes: peps_cleared
    severity: blocking
    category: kyc
    failure_mode: manual_review
  - name: age_gate
    writes: age_eligible
    severity: blocking
    category: eligibility
    failure_mode: definitive
`),
	})
	require.NoError(t, err)

	err = engine.RegisterProduct(ctx, prodcat.ProductEligibility{
		ProductID: "test-all-modes", Name: "Test All Modes",
		Status: prodcat.ProductStatusActive, BaseRulesetIDs: []string{rs.ID},
	})
	require.NoError(t, err)

	result, err := engine.Evaluate(ctx, "test-all-modes", prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{primaryHolder(withAge(16))},
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictNotEligible, result.Verdict)

	byName := requirementsByName(result)
	assert.Equal(t, prodcat.FailureModeActionable, byName["email_verified"].FailureMode)
	assert.Equal(t, prodcat.FailureModeInputRequired, byName["id_document_uploaded"].FailureMode)
	assert.Equal(t, prodcat.FailureModeManualReview, byName["peps_cleared"].FailureMode)
	assert.Equal(t, prodcat.FailureModeDefinitive, byName["age_gate"].FailureMode)
	assert.Equal(t, prodcat.RequirementStatusFailed, byName["age_gate"].Status)
}

// ─── Subscription Lifecycle ───

func TestSubscriptionLifecycle(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedMalSubscription(t, engine, tracker)
	ctx := context.Background()

	sub, err := engine.Subscribe(ctx, prodcat.SubscribeRequest{
		EntityID: "customer-123", EntityType: prodcat.EntityTypeIndividual,
		ProductID: "mal-subscription",
		Parties:   []prodcat.SubscribePartyInput{{CustomerID: "customer-123", Role: prodcat.PartyRolePrimaryHolder}},
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.SubscriptionStatusIncomplete, sub.Status)
	assert.Len(t, sub.Parties, 1)

	sub, err = engine.Activate(ctx, sub.ID, "ext-ref-001", []prodcat.CapabilityType{prodcat.CapabilityTypeView})
	require.NoError(t, err)
	assert.Equal(t, prodcat.SubscriptionStatusActive, sub.Status)
	assert.NotNil(t, sub.ActivatedAt)

	sub, err = engine.Disable(ctx, sub.ID, prodcat.DisabledReasonRegulatoryHold, "AML review")
	require.NoError(t, err)
	require.NotNil(t, sub.Disabled)
	assert.True(t, sub.Disabled.Disabled)

	sub, err = engine.Enable(ctx, sub.ID)
	require.NoError(t, err)
	assert.Nil(t, sub.Disabled)

	sub, err = engine.Cancel(ctx, sub.ID, "customer requested")
	require.NoError(t, err)
	assert.Equal(t, prodcat.SubscriptionStatusCanceled, sub.Status)
}

// ─── Product Lookup ───

func TestProductRegistrationAndLookup(t *testing.T) {
	engine, _ := sharedTestEngine(t)
	ctx := context.Background()

	err := engine.RegisterProduct(ctx, prodcat.ProductEligibility{
		ProductID: "savings-aed", Name: "AED Savings Account",
		Tags: []string{"family:casa", "market:uae", "sharia:true"},
		Status: prodcat.ProductStatusActive, ShariaCompliant: true, CurrencyCode: "AED",
		Availability: prodcat.GeoAvailability{
			Mode: prodcat.AvailabilityModeSpecificCountries, CountryCodes: []string{"AE"},
		},
	})
	require.NoError(t, err)

	p, err := engine.GetProduct(ctx, "savings-aed")
	require.NoError(t, err)
	assert.Equal(t, "AED Savings Account", p.Name)
	assert.True(t, p.ShariaCompliant)
}

// ─── CheckEligibility ───

func TestCheckEligibility(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedCASA(t, engine, tracker)

	report, err := engine.CheckEligibility(context.Background(),
		[]string{"mal-subscription", "casa-current-account-uae"},
		prodcat.EvaluationInput{
			Parties: []prodcat.EvalPartyInput{primaryHolder(
				withContacts(email(true), phone(true)),
				withAge(25), withResidence("AE"),
				withDocs(passport(true), uaePass(true)),
			)},
			Agreements: agreements("general_terms_and_conditions", "casa_terms_and_conditions"),
		},
	)
	require.NoError(t, err)
	assert.Len(t, report.Results, 2)

	byProduct := make(map[string]prodcat.EvaluationResult)
	for _, r := range report.Results {
		byProduct[r.ProductID] = r
	}
	assert.Equal(t, prodcat.EligibilityVerdictEligible, byProduct["mal-subscription"].Verdict)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, byProduct["casa-current-account-uae"].Verdict)
}

func TestCheckEligibility_MixedVerdicts(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedCASA(t, engine, tracker)

	report, err := engine.CheckEligibility(context.Background(),
		[]string{"mal-subscription", "casa-current-account-uae"},
		prodcat.EvaluationInput{
			Parties:    []prodcat.EvalPartyInput{primaryHolder(withContacts(email(true), phone(true)), withAge(16), withResidence("AE"))},
			Agreements: agreements("general_terms_and_conditions"),
		},
	)
	require.NoError(t, err)

	byProduct := make(map[string]prodcat.EvaluationResult)
	for _, r := range report.Results {
		byProduct[r.ProductID] = r
	}
	assert.Equal(t, prodcat.EligibilityVerdictEligible, byProduct["mal-subscription"].Verdict)
	assert.Equal(t, prodcat.EligibilityVerdictNotEligible, byProduct["casa-current-account-uae"].Verdict)
}

func TestCheckEligibility_UnknownProduct(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedMalSubscription(t, engine, tracker)

	report, err := engine.CheckEligibility(context.Background(),
		[]string{"mal-subscription", "nonexistent-product"},
		prodcat.EvaluationInput{
			Parties:    []prodcat.EvalPartyInput{primaryHolder(withContacts(email(true), phone(true)))},
			Agreements: agreements("general_terms_and_conditions"),
		},
	)
	require.NoError(t, err)

	byProduct := make(map[string]prodcat.EvaluationResult)
	for _, r := range report.Results {
		byProduct[r.ProductID] = r
	}
	assert.Equal(t, prodcat.EligibilityVerdictEligible, byProduct["mal-subscription"].Verdict)
	assert.Equal(t, prodcat.EligibilityVerdictNotEligible, byProduct["nonexistent-product"].Verdict)
}

// ─── Resolve Ruleset ───

func TestResolveRuleset(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedMalSubscription(t, engine, tracker)

	resolved, err := engine.ResolveRuleset(context.Background(), "mal-subscription")
	require.NoError(t, err)
	assert.Equal(t, "mal-subscription", resolved.ProductID)
	assert.Len(t, resolved.Layers, 1)
	assert.Equal(t, "base", resolved.Layers[0].Source)
	assert.Equal(t, "base-platform-access", resolved.Layers[0].SourceID)
}

// ─── CASA Current Account UAE ───

func TestCASA_FullyEligible(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedCASA(t, engine, tracker)

	result, err := engine.Evaluate(context.Background(), "casa-current-account-uae", prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{primaryHolder(
			withContacts(email(true), phone(true)),
			withAge(25), withResidence("AE"),
			withDocs(passport(true), uaePass(true)),
		)},
		Agreements: agreements("general_terms_and_conditions", "casa_terms_and_conditions"),
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, result.Verdict)
}

func TestCASA_Underage_Definitive(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedCASA(t, engine, tracker)

	result, err := engine.Evaluate(context.Background(), "casa-current-account-uae", prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{primaryHolder(
			withContacts(email(true), phone(true)),
			withAge(16), withResidence("AE"),
			withDocs(passport(true), uaePass(true)),
		)},
		Agreements: agreements("general_terms_and_conditions", "casa_terms_and_conditions"),
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictNotEligible, result.Verdict)
	byName := requirementsByName(result)
	assert.Equal(t, prodcat.RequirementStatusFailed, byName["age_18_plus"].Status)
	assert.Equal(t, prodcat.FailureModeDefinitive, byName["age_18_plus"].FailureMode)
}

func TestCASA_NonResident_Definitive(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedCASA(t, engine, tracker)

	result, err := engine.Evaluate(context.Background(), "casa-current-account-uae", prodcat.EvaluationInput{
		Parties:    []prodcat.EvalPartyInput{primaryHolder(withContacts(email(true), phone(true)), withAge(30), withResidence("GB"))},
		Agreements: agreements("general_terms_and_conditions"),
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictNotEligible, result.Verdict)
	byName := requirementsByName(result)
	assert.Equal(t, prodcat.RequirementStatusFailed, byName["uae_resident"].Status)
}

func TestCASA_MissingPassport_InputRequired(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedCASA(t, engine, tracker)

	result, err := engine.Evaluate(context.Background(), "casa-current-account-uae", prodcat.EvaluationInput{
		Parties:    []prodcat.EvalPartyInput{primaryHolder(withContacts(email(true), phone(true)), withAge(25), withResidence("AE"))},
		Agreements: agreements("general_terms_and_conditions", "casa_terms_and_conditions"),
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, result.Verdict)
	byName := requirementsByName(result)
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["age_18_plus"].Status)
	assert.Equal(t, prodcat.RequirementStatusPending, byName["valid_passport"].Status)
	assert.Equal(t, prodcat.FailureModeInputRequired, byName["valid_passport"].FailureMode)
}

func TestCASA_ProgressiveOnboarding(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedCASA(t, engine, tracker)
	ctx := context.Background()

	// Step 1: empty.
	r1, err := engine.Evaluate(ctx, "casa-current-account-uae", prodcat.EvaluationInput{})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, r1.Verdict)

	// Step 2: platform + demographics.
	r2, err := engine.Evaluate(ctx, "casa-current-account-uae", prodcat.EvaluationInput{
		Parties:    []prodcat.EvalPartyInput{primaryHolder(withContacts(email(true), phone(true)), withAge(25), withResidence("AE"))},
		Agreements: agreements("general_terms_and_conditions"),
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, r2.Verdict)
	byName := requirementsByName(r2)
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["email_verified"].Status)
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["age_18_plus"].Status)
	assert.Equal(t, prodcat.RequirementStatusPending, byName["valid_passport"].Status)

	// Step 3: everything.
	r3, err := engine.Evaluate(ctx, "casa-current-account-uae", prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{primaryHolder(
			withContacts(email(true), phone(true)),
			withAge(25), withResidence("AE"),
			withDocs(passport(true), uaePass(true)),
		)},
		Agreements: agreements("general_terms_and_conditions", "casa_terms_and_conditions"),
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, r3.Verdict)
}

// ─── KYC Checks ───

func TestEvaluate_KycChecks(t *testing.T) {
	engine, _ := sharedTestEngine(t)
	ctx := context.Background()

	rs, err := engine.CreateRuleset(ctx, prodcat.BaseRuleset{
		Name: "full-kyc", Version: "1.0.0",
		Content: []byte(`evaluations:
  - name: liveness_check
    writes: liveness_passed
    severity: blocking
    category: kyc
    failure_mode: actionable
    resolution: "Please complete the liveness check"
  - name: pep_screening
    writes: pep_screening_clear
    severity: blocking
    category: kyc
    failure_mode: manual_review
    resolution: "Pending PEPs screening"
  - name: address_verified
    writes: residential_address_validated
    severity: blocking
    category: kyc
    failure_mode: input_required
    resolution: "Please provide proof of address"
`),
	})
	require.NoError(t, err)

	err = engine.RegisterProduct(ctx, prodcat.ProductEligibility{
		ProductID: "kyc-test-product", Name: "KYC Test",
		Status: prodcat.ProductStatusActive, BaseRulesetIDs: []string{rs.ID},
	})
	require.NoError(t, err)

	// All KYC checks passed.
	result, err := engine.Evaluate(ctx, "kyc-test-product", prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{primaryHolder(withKYC(prodcat.KycChecks{
			LivenessPassed:              true,
			PEPScreeningClear:           true,
			ResidentialAddressValidated: true,
		}))},
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, result.Verdict)

	// PEPs not cleared — manual review pending.
	result, err = engine.Evaluate(ctx, "kyc-test-product", prodcat.EvaluationInput{
		Parties: []prodcat.EvalPartyInput{primaryHolder(withKYC(prodcat.KycChecks{
			LivenessPassed:              true,
			PEPScreeningClear:           false,
			ResidentialAddressValidated: true,
		}))},
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, result.Verdict)
	byName := requirementsByName(result)
	assert.Equal(t, prodcat.RequirementStatusPending, byName["pep_screening"].Status)
	assert.Equal(t, prodcat.FailureModeManualReview, byName["pep_screening"].FailureMode)
}

// ─── Seed Tracker ───

func TestSeedTracker_Idempotent(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	ctx := context.Background()

	data, err := os.ReadFile(malSeedFile)
	require.NoError(t, err)

	err = engine.ApplySeed(ctx, malSeedFile, data, tracker)
	require.NoError(t, err)
	err = engine.ApplySeed(ctx, malSeedFile, data, tracker)
	require.NoError(t, err)

	applied, err := tracker.HasApplied(ctx, malSeedFile)
	require.NoError(t, err)
	assert.True(t, applied)
}
