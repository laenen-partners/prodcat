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

// sharedConnStr caches the connection string so all tests share one container.
var _sharedConnStr string

func sharedTestEngine(t *testing.T) (*prodcat.Engine, *prodcat.MemTracker) {
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

		// Run entitystore migrations.
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
	tracker := prodcat.NewMemTracker()

	return engine, tracker
}

func seedMalSubscription(t *testing.T, engine *prodcat.Engine, tracker *prodcat.MemTracker) {
	t.Helper()
	ctx := context.Background()

	data, err := os.ReadFile(malSeedFile)
	require.NoError(t, err)

	err = engine.ApplySeed(ctx, malSeedFile, data, tracker)
	require.NoError(t, err)
}

func seedCASA(t *testing.T, engine *prodcat.Engine, tracker *prodcat.MemTracker) {
	t.Helper()
	ctx := context.Background()

	seedMalSubscription(t, engine, tracker)

	data, err := os.ReadFile(casaSeedFile)
	require.NoError(t, err)
	err = engine.ApplySeed(ctx, casaSeedFile, data, tracker)
	require.NoError(t, err)
}

func requirementsByName(result prodcat.EvaluationResult) map[string]prodcat.RequirementResult {
	m := make(map[string]prodcat.RequirementResult, len(result.Requirements))
	for _, r := range result.Requirements {
		m[r.Name] = r
	}
	return m
}

// ─── Mal Subscription Tests ───

func TestEvaluate_MalSubscription_AllPassed(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedMalSubscription(t, engine, tracker)
	ctx := context.Background()

	result, err := engine.Evaluate(ctx, "mal-subscription", prodcat.EvaluationInput{
		Contacts: []prodcat.ContactInput{
			{Type: 1, Value: "user@example.com", Primary: true, Verified: true},
			{Type: 2, Value: "+971501234567", Primary: true, Verified: true},
		},
		Agreements: []prodcat.AgreementInput{
			{Type: "general_terms_and_conditions", Accepted: true, Version: "1.0"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, result.Verdict)
	assert.Len(t, result.Requirements, 4)

	for _, r := range result.Requirements {
		assert.Equal(t, prodcat.RequirementStatusPassed, r.Status, "requirement %s should pass", r.Name)
	}
}

func TestEvaluate_MalSubscription_Incomplete(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedMalSubscription(t, engine, tracker)
	ctx := context.Background()

	result, err := engine.Evaluate(ctx, "mal-subscription", prodcat.EvaluationInput{
		Contacts: []prodcat.ContactInput{
			{Type: 1, Value: "user@example.com", Primary: true, Verified: true},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, result.Verdict)

	byName := requirementsByName(result)
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["email_verified"].Status)
	assert.Equal(t, prodcat.RequirementStatusPending, byName["phone_verified"].Status)
	assert.Equal(t, prodcat.RequirementStatusPending, byName["general_tc_accepted"].Status)
	assert.Equal(t, prodcat.RequirementStatusPending, byName["platform_access_ready"].Status)
}

func TestEvaluate_MalSubscription_ActionableFailure(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedMalSubscription(t, engine, tracker)
	ctx := context.Background()

	result, err := engine.Evaluate(ctx, "mal-subscription", prodcat.EvaluationInput{
		Contacts: []prodcat.ContactInput{
			{Type: 1, Value: "user@example.com", Primary: true, Verified: false},
			{Type: 2, Value: "+971501234567", Primary: true, Verified: true},
		},
		Agreements: []prodcat.AgreementInput{
			{Type: "general_terms_and_conditions", Accepted: true, Version: "1.0"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, result.Verdict)

	byName := requirementsByName(result)
	assert.Equal(t, prodcat.RequirementStatusPending, byName["email_verified"].Status)
}

func TestEvaluate_EmptyInput(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedMalSubscription(t, engine, tracker)
	ctx := context.Background()

	result, err := engine.Evaluate(ctx, "mal-subscription", prodcat.EvaluationInput{})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, result.Verdict)

	for _, r := range result.Requirements {
		assert.Equal(t, prodcat.RequirementStatusPending, r.Status, "requirement %s should be pending", r.Name)
	}
}

// ─── Failure Mode Tests ───

func TestEvaluate_DefinitiveFailure(t *testing.T) {
	engine, _ := sharedTestEngine(t)
	ctx := context.Background()

	ageGateRuleset := []byte(`evaluations:
  - name: age_gate
    expression: "input.age >= 18"
    writes: age_eligible
    severity: blocking
    category: eligibility
    failure_mode: definitive
    resolution: "You must be at least 18 years old"
`)
	rs, err := engine.CreateRuleset(ctx, prodcat.BaseRuleset{
		Name:    "age-gate",
		Content: ageGateRuleset,
		Version: "1.0.0",
	})
	require.NoError(t, err)

	err = engine.RegisterProduct(ctx, prodcat.ProductEligibility{
		ProductID:      "investment-fund",
		Name:           "Investment Fund",
		Tags:           []string{"family:investments", "market:uae"},
		Status:         prodcat.ProductStatusActive,
		BaseRulesetIDs: []string{rs.ID},
	})
	require.NoError(t, err)

	result, err := engine.Evaluate(ctx, "investment-fund", prodcat.EvaluationInput{Age: 16})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictNotEligible, result.Verdict)
	assert.Equal(t, prodcat.RequirementStatusFailed, result.Requirements[0].Status)

	result, err = engine.Evaluate(ctx, "investment-fund", prodcat.EvaluationInput{Age: 21})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, result.Verdict)
}

func TestEvaluate_FailureModes(t *testing.T) {
	engine, _ := sharedTestEngine(t)
	ctx := context.Background()

	rulesetContent := []byte(`evaluations:
  - name: email_verified
    expression: "input.contacts.exists(c, c.type == 1 && c.primary && c.verified)"
    writes: email_verified
    severity: blocking
    category: contact
    failure_mode: actionable
  - name: id_document_uploaded
    expression: "input.id_documents.exists(d, d.verified && !d.expired)"
    writes: id_uploaded
    severity: blocking
    category: identity
    failure_mode: input_required
  - name: peps_cleared
    expression: "input.peps_cleared"
    writes: peps_cleared
    severity: blocking
    category: compliance
    failure_mode: manual_review
  - name: age_gate
    expression: "input.age >= 18"
    writes: age_eligible
    severity: blocking
    category: eligibility
    failure_mode: definitive
`)
	rs, err := engine.CreateRuleset(ctx, prodcat.BaseRuleset{
		Name: "all-modes", Content: rulesetContent, Version: "1.0.0",
	})
	require.NoError(t, err)

	err = engine.RegisterProduct(ctx, prodcat.ProductEligibility{
		ProductID: "test-all-modes", Name: "Test All Modes",
		Status: prodcat.ProductStatusActive, BaseRulesetIDs: []string{rs.ID},
	})
	require.NoError(t, err)

	result, err := engine.Evaluate(ctx, "test-all-modes", prodcat.EvaluationInput{Age: 16})
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
		Parties:   []prodcat.PartyInput{{CustomerID: "customer-123", Role: prodcat.PartyRolePrimaryHolder}},
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.SubscriptionStatusIncomplete, sub.Status)
	assert.Len(t, sub.Parties, 1)

	sub, err = engine.Activate(ctx, sub.ID, "ext-ref-001", []prodcat.CapabilityType{prodcat.CapabilityTypeView})
	require.NoError(t, err)
	assert.Equal(t, prodcat.SubscriptionStatusActive, sub.Status)
	assert.Len(t, sub.Capabilities, 1)
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
	assert.NotNil(t, sub.CanceledAt)
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

func TestResolveRuleset(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedMalSubscription(t, engine, tracker)
	ctx := context.Background()

	resolved, err := engine.ResolveRuleset(ctx, "mal-subscription")
	require.NoError(t, err)
	assert.Equal(t, "mal-subscription", resolved.ProductID)
	assert.Len(t, resolved.Layers, 1)
	assert.Equal(t, "base", resolved.Layers[0].Source)
	assert.Equal(t, "base-platform-access", resolved.Layers[0].SourceID)
	assert.NotEmpty(t, resolved.Merged)
}

// ─── CASA Current Account UAE ───

func TestCASA_FullyEligible(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedCASA(t, engine, tracker)
	ctx := context.Background()

	result, err := engine.Evaluate(ctx, "casa-current-account-uae", prodcat.EvaluationInput{
		Contacts: []prodcat.ContactInput{
			{Type: 1, Value: "user@example.com", Primary: true, Verified: true},
			{Type: 2, Value: "+971501234567", Primary: true, Verified: true},
		},
		Agreements: []prodcat.AgreementInput{
			{Type: "general_terms_and_conditions", Accepted: true},
			{Type: "casa_terms_and_conditions", Accepted: true},
		},
		Age: 25, CountryOfResidence: "AE",
		IDDocuments: []prodcat.IDDocumentInput{
			{DocumentType: "passport", IssuingCountry: "AE", Verified: true, Expired: false},
			{DocumentType: "uae_pass", IssuingCountry: "AE", Verified: true, Expired: false},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, result.Verdict)
	for _, r := range result.Requirements {
		assert.Equal(t, prodcat.RequirementStatusPassed, r.Status, "requirement %s should pass", r.Name)
	}
}

func TestCASA_Underage_Definitive(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedCASA(t, engine, tracker)
	ctx := context.Background()

	result, err := engine.Evaluate(ctx, "casa-current-account-uae", prodcat.EvaluationInput{
		Contacts: []prodcat.ContactInput{
			{Type: 1, Value: "user@example.com", Primary: true, Verified: true},
			{Type: 2, Value: "+971501234567", Primary: true, Verified: true},
		},
		Agreements: []prodcat.AgreementInput{
			{Type: "general_terms_and_conditions", Accepted: true},
			{Type: "casa_terms_and_conditions", Accepted: true},
		},
		Age: 16, CountryOfResidence: "AE",
		IDDocuments: []prodcat.IDDocumentInput{
			{DocumentType: "passport", IssuingCountry: "AE", Verified: true, Expired: false},
			{DocumentType: "uae_pass", IssuingCountry: "AE", Verified: true, Expired: false},
		},
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
	ctx := context.Background()

	result, err := engine.Evaluate(ctx, "casa-current-account-uae", prodcat.EvaluationInput{
		Contacts: []prodcat.ContactInput{
			{Type: 1, Value: "user@example.com", Primary: true, Verified: true},
			{Type: 2, Value: "+971501234567", Primary: true, Verified: true},
		},
		Agreements: []prodcat.AgreementInput{{Type: "general_terms_and_conditions", Accepted: true}},
		Age: 30, CountryOfResidence: "GB",
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictNotEligible, result.Verdict)
	byName := requirementsByName(result)
	assert.Equal(t, prodcat.RequirementStatusFailed, byName["uae_resident"].Status)
	assert.Equal(t, prodcat.FailureModeDefinitive, byName["uae_resident"].FailureMode)
}

func TestCASA_MissingPassport_InputRequired(t *testing.T) {
	engine, tracker := sharedTestEngine(t)
	seedCASA(t, engine, tracker)
	ctx := context.Background()

	result, err := engine.Evaluate(ctx, "casa-current-account-uae", prodcat.EvaluationInput{
		Contacts: []prodcat.ContactInput{
			{Type: 1, Value: "user@example.com", Primary: true, Verified: true},
			{Type: 2, Value: "+971501234567", Primary: true, Verified: true},
		},
		Agreements: []prodcat.AgreementInput{
			{Type: "general_terms_and_conditions", Accepted: true},
			{Type: "casa_terms_and_conditions", Accepted: true},
		},
		Age: 25, CountryOfResidence: "AE",
	})

	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, result.Verdict)
	byName := requirementsByName(result)
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["age_18_plus"].Status)
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["uae_resident"].Status)
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

	// Step 2: platform + age + residence.
	r2, err := engine.Evaluate(ctx, "casa-current-account-uae", prodcat.EvaluationInput{
		Contacts: []prodcat.ContactInput{
			{Type: 1, Value: "user@example.com", Primary: true, Verified: true},
			{Type: 2, Value: "+971501234567", Primary: true, Verified: true},
		},
		Agreements: []prodcat.AgreementInput{{Type: "general_terms_and_conditions", Accepted: true}},
		Age: 25, CountryOfResidence: "AE",
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictIncomplete, r2.Verdict)
	byName := requirementsByName(r2)
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["email_verified"].Status)
	assert.Equal(t, prodcat.RequirementStatusPassed, byName["age_18_plus"].Status)
	assert.Equal(t, prodcat.RequirementStatusPending, byName["valid_passport"].Status)

	// Step 3: everything.
	r3, err := engine.Evaluate(ctx, "casa-current-account-uae", prodcat.EvaluationInput{
		Contacts: []prodcat.ContactInput{
			{Type: 1, Value: "user@example.com", Primary: true, Verified: true},
			{Type: 2, Value: "+971501234567", Primary: true, Verified: true},
		},
		Agreements: []prodcat.AgreementInput{
			{Type: "general_terms_and_conditions", Accepted: true},
			{Type: "casa_terms_and_conditions", Accepted: true},
		},
		Age: 25, CountryOfResidence: "AE",
		IDDocuments: []prodcat.IDDocumentInput{
			{DocumentType: "passport", IssuingCountry: "AE", Verified: true, Expired: false},
			{DocumentType: "uae_pass", IssuingCountry: "AE", Verified: true, Expired: false},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.EligibilityVerdictEligible, r3.Verdict)
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

	applied, err := tracker.ListApplied(ctx)
	require.NoError(t, err)
	assert.Len(t, applied, 1)
}
