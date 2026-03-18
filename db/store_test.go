package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/laenen-partners/prodcat"
	"github.com/laenen-partners/prodcat/db"
)

func setupStore(t *testing.T) (*db.Store, context.Context) {
	t.Helper()
	ctx := context.Background()

	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("prodcat_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithSQLDriver("pgx"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { pg.Terminate(context.Background()) })

	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	store, err := db.New(db.WithPool(pool))
	require.NoError(t, err)

	err = store.Migrate(ctx)
	require.NoError(t, err)

	return store, ctx
}

func TestCreateAndGetFamily(t *testing.T) {
	store, ctx := setupStore(t)

	f := prodcat.FamilyDefinition{
		ID:     "casa",
		Family: prodcat.ProductFamilyCASA,
		Name:   map[string]string{"en": "CASA", "ar": "حسابات جارية"},
		Description: map[string]string{"en": "Current and Savings Accounts"},
	}

	created, err := store.CreateFamily(ctx, f)
	require.NoError(t, err)
	assert.Equal(t, "casa", created.ID)
	assert.Equal(t, prodcat.ProductFamilyCASA, created.Family)
	assert.Equal(t, "CASA", created.Name["en"])

	got, err := store.GetFamily(ctx, "casa")
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, created.Family, got.Family)
}

func TestListFamilies(t *testing.T) {
	store, ctx := setupStore(t)

	_, err := store.CreateFamily(ctx, prodcat.FamilyDefinition{
		ID: "casa", Family: prodcat.ProductFamilyCASA,
		Name: map[string]string{"en": "CASA"},
	})
	require.NoError(t, err)

	_, err = store.CreateFamily(ctx, prodcat.FamilyDefinition{
		ID: "cards", Family: prodcat.ProductFamilyCards,
		Name: map[string]string{"en": "Cards"},
	})
	require.NoError(t, err)

	families, err := store.ListFamilies(ctx)
	require.NoError(t, err)
	assert.Len(t, families, 2)
}

func TestUpdateFamily(t *testing.T) {
	store, ctx := setupStore(t)

	_, err := store.CreateFamily(ctx, prodcat.FamilyDefinition{
		ID: "casa", Family: prodcat.ProductFamilyCASA,
		Name: map[string]string{"en": "CASA"},
	})
	require.NoError(t, err)

	updated, err := store.UpdateFamily(ctx, prodcat.FamilyDefinition{
		ID:   "casa",
		Name: map[string]string{"en": "CASA Updated"},
		Description: map[string]string{"en": "Updated description"},
		Ruleset: []byte("evaluations:\n  - name: test\n"),
		BaseRulesetIDs: []string{"base-contact-verification"},
	})
	require.NoError(t, err)
	assert.Equal(t, "CASA Updated", updated.Name["en"])
	assert.Equal(t, []string{"base-contact-verification"}, updated.BaseRulesetIDs)
	assert.NotNil(t, updated.Ruleset)
}

func TestCreateArchetypeAndProduct(t *testing.T) {
	store, ctx := setupStore(t)

	_, err := store.CreateFamily(ctx, prodcat.FamilyDefinition{
		ID: "casa", Family: prodcat.ProductFamilyCASA,
		Name: map[string]string{"en": "CASA"},
	})
	require.NoError(t, err)

	arch, err := store.CreateArchetype(ctx, prodcat.Archetype{
		ID:       "current-account",
		FamilyID: "casa",
		Name:     map[string]string{"en": "Current Account"},
	})
	require.NoError(t, err)
	assert.Equal(t, "current-account", arch.ID)
	assert.Equal(t, "casa", arch.FamilyID)

	product, err := store.CreateProduct(ctx, prodcat.Product{
		ID:           "aed-ca",
		ArchetypeID:  "current-account",
		Name:         map[string]string{"en": "AED Current Account"},
		Description:  map[string]string{"en": "UAE Dirham current account"},
		Status:       prodcat.ProductStatusDraft,
		ProductType:  prodcat.ProductTypePrimary,
		CurrencyCode: "AED",
		Provider: prodcat.RegulatoryProvider{
			ProviderID:        "mal",
			Name:              "Mal",
			Regulator:         "CBUAE",
			RegulatoryCountry: "AE",
		},
		Compliance: prodcat.ComplianceConfig{ShariaCompliant: true},
		Eligibility: prodcat.EligibilityConfig{
			Geographic: prodcat.GeographicAvailability{
				Mode:         prodcat.AvailabilityModeSpecificCountries,
				CountryCodes: []string{"AE"},
			},
			Ruleset: []byte("evaluations:\n  - name: aed_ca_tc\n"),
		},
		CreatedBy: "pascal",
	})
	require.NoError(t, err)
	assert.Equal(t, "aed-ca", product.ID)
	assert.Equal(t, prodcat.ProductStatusDraft, product.Status)
	assert.True(t, product.Compliance.ShariaCompliant)
	assert.Equal(t, []string{"AE"}, product.Eligibility.Geographic.CountryCodes)
}

func TestTransitionProductStatus(t *testing.T) {
	store, ctx := setupStore(t)
	seedProduct(t, store, ctx)

	p, err := store.TransitionProductStatus(ctx, "aed-ca", prodcat.ProductStatusActive, "ready for launch")
	require.NoError(t, err)
	assert.Equal(t, prodcat.ProductStatusActive, p.Status)
}

func TestBaseRulesetCRUD(t *testing.T) {
	store, ctx := setupStore(t)

	content := []byte(`evaluations:
  - name: email_verified
    expression: "input.contacts.exists(c, c.type == 2 && c.primary && c.verified)"
    reads: [input.contacts]
    writes: email_verified
    severity: blocking
    category: contact
`)

	created, validation, err := store.CreateBaseRuleset(ctx, prodcat.BaseRuleset{
		ID:      "base-contact-verification",
		Name:    "Contact Verification",
		Content: content,
		Version: "1",
	})
	require.NoError(t, err)
	assert.True(t, validation.Valid)
	assert.Equal(t, "base-contact-verification", created.ID)

	got, err := store.GetBaseRuleset(ctx, "base-contact-verification")
	require.NoError(t, err)
	assert.Equal(t, content, got.Content)

	all, err := store.ListBaseRulesets(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestSubscribeAndGetSubscription(t *testing.T) {
	store, ctx := setupStore(t)
	seedProduct(t, store, ctx)

	sub, err := store.Subscribe(ctx, prodcat.SubscribeRequest{
		EntityID:   "customer-1",
		EntityType: prodcat.EntityTypeIndividual,
		ProductID:  "aed-ca",
		InitialParties: []prodcat.PartyInput{
			{CustomerID: "customer-1", Role: prodcat.PartyRolePrimaryHolder},
		},
		SigningAuthority: prodcat.SigningAuthority{
			Rule:          prodcat.SigningRuleAnyOne,
			RequiredCount: 1,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.SubscriptionStatusIncomplete, sub.Status)
	assert.Equal(t, "customer-1", sub.EntityID)
	assert.Equal(t, prodcat.EntityTypeIndividual, sub.EntityType)
	assert.Len(t, sub.Parties, 1)
	assert.Equal(t, prodcat.PartyRolePrimaryHolder, sub.Parties[0].Role)

	got, err := store.GetSubscription(ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, sub.ID, got.ID)
	assert.Len(t, got.Parties, 1)
}

func TestJointAccountSubscription(t *testing.T) {
	store, ctx := setupStore(t)
	seedProduct(t, store, ctx)

	sub, err := store.Subscribe(ctx, prodcat.SubscribeRequest{
		EntityID:   "customer-1",
		EntityType: prodcat.EntityTypeIndividual,
		ProductID:  "aed-ca",
		InitialParties: []prodcat.PartyInput{
			{CustomerID: "customer-1", Role: prodcat.PartyRolePrimaryHolder},
			{CustomerID: "customer-2", Role: prodcat.PartyRoleJointHolder},
		},
		SigningAuthority: prodcat.SigningAuthority{
			Rule:          prodcat.SigningRuleAnyN,
			RequiredCount: 2,
		},
	})
	require.NoError(t, err)
	assert.Len(t, sub.Parties, 2)
	assert.Equal(t, prodcat.SigningRuleAnyN, sub.SigningAuthority.Rule)
	assert.Equal(t, 2, sub.SigningAuthority.RequiredCount)
}

func TestActivateSubscription(t *testing.T) {
	store, ctx := setupStore(t)
	seedProduct(t, store, ctx)

	sub, err := store.Subscribe(ctx, prodcat.SubscribeRequest{
		EntityID:   "customer-1",
		EntityType: prodcat.EntityTypeIndividual,
		ProductID:  "aed-ca",
		InitialParties: []prodcat.PartyInput{
			{CustomerID: "customer-1", Role: prodcat.PartyRolePrimaryHolder},
		},
		SigningAuthority: prodcat.SigningAuthority{Rule: prodcat.SigningRuleAnyOne, RequiredCount: 1},
	})
	require.NoError(t, err)

	activated, err := store.Activate(ctx, sub.ID, "saascada-acc-123", []prodcat.CapabilityType{
		prodcat.CapabilityTypeView,
		prodcat.CapabilityTypeDomesticTransfers,
		prodcat.CapabilityTypeReceive,
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.SubscriptionStatusActive, activated.Status)
	assert.NotNil(t, activated.ExternalRef)
	assert.Equal(t, "saascada-acc-123", *activated.ExternalRef)
	assert.Len(t, activated.Capabilities, 3)
}

func TestDisableEnableSubscription(t *testing.T) {
	store, ctx := setupStore(t)
	seedProduct(t, store, ctx)

	sub := createActiveSubscription(t, store, ctx)

	disabled, err := store.Disable(ctx, sub.ID, prodcat.DisabledReasonRegulatoryHold, "AML review pending")
	require.NoError(t, err)
	assert.NotNil(t, disabled.Disabled)
	assert.True(t, disabled.Disabled.Disabled)
	assert.Equal(t, prodcat.DisabledReasonRegulatoryHold, disabled.Disabled.Reason)
	assert.Equal(t, "AML review pending", disabled.Disabled.Message)

	enabled, err := store.Enable(ctx, sub.ID)
	require.NoError(t, err)
	assert.Nil(t, enabled.Disabled)
}

func TestDisableEnableCapability(t *testing.T) {
	store, ctx := setupStore(t)
	seedProduct(t, store, ctx)

	sub := createActiveSubscription(t, store, ctx)
	capID := sub.Capabilities[1].ID // domestic_transfers

	updated, err := store.DisableCapability(ctx, sub.ID, capID, prodcat.DisabledReasonExpiredData, "Emirates ID expired")
	require.NoError(t, err)

	var found bool
	for _, c := range updated.Capabilities {
		if c.ID == capID {
			found = true
			assert.Equal(t, prodcat.CapabilityStatusDisabled, c.Status)
			assert.NotNil(t, c.Disabled)
			assert.Equal(t, prodcat.DisabledReasonExpiredData, c.Disabled.Reason)
		}
	}
	assert.True(t, found)

	updated, err = store.EnableCapability(ctx, sub.ID, capID)
	require.NoError(t, err)
	for _, c := range updated.Capabilities {
		if c.ID == capID {
			assert.Equal(t, prodcat.CapabilityStatusActive, c.Status)
			assert.Nil(t, c.Disabled)
		}
	}
}

func TestAddRemoveParty(t *testing.T) {
	store, ctx := setupStore(t)
	seedProduct(t, store, ctx)

	sub, err := store.Subscribe(ctx, prodcat.SubscribeRequest{
		EntityID:   "customer-1",
		EntityType: prodcat.EntityTypeIndividual,
		ProductID:  "aed-ca",
		InitialParties: []prodcat.PartyInput{
			{CustomerID: "customer-1", Role: prodcat.PartyRolePrimaryHolder},
		},
		SigningAuthority: prodcat.SigningAuthority{Rule: prodcat.SigningRuleAnyOne, RequiredCount: 1},
	})
	require.NoError(t, err)
	assert.Len(t, sub.Parties, 1)

	updated, err := store.AddParty(ctx, sub.ID, "customer-2", prodcat.PartyRoleJointHolder)
	require.NoError(t, err)
	assert.Len(t, updated.Parties, 2)

	jointPartyID := updated.Parties[1].ID
	updated, err = store.RemoveParty(ctx, sub.ID, jointPartyID, "customer request")
	require.NoError(t, err)
	assert.Len(t, updated.Parties, 1) // removed party is filtered out (removed_at IS NULL)
}

func TestCancelSubscription(t *testing.T) {
	store, ctx := setupStore(t)
	seedProduct(t, store, ctx)

	sub := createActiveSubscription(t, store, ctx)

	canceled, err := store.Cancel(ctx, sub.ID, "customer requested closure")
	require.NoError(t, err)
	assert.Equal(t, prodcat.SubscriptionStatusCanceled, canceled.Status)
	assert.NotNil(t, canceled.CanceledAt)
}

func TestUpdateSigningAuthority(t *testing.T) {
	store, ctx := setupStore(t)
	seedProduct(t, store, ctx)

	sub, err := store.Subscribe(ctx, prodcat.SubscribeRequest{
		EntityID:   "customer-1",
		EntityType: prodcat.EntityTypeIndividual,
		ProductID:  "aed-ca",
		InitialParties: []prodcat.PartyInput{
			{CustomerID: "customer-1", Role: prodcat.PartyRolePrimaryHolder},
			{CustomerID: "customer-2", Role: prodcat.PartyRoleJointHolder},
		},
		SigningAuthority: prodcat.SigningAuthority{Rule: prodcat.SigningRuleAnyOne, RequiredCount: 1},
	})
	require.NoError(t, err)

	updated, err := store.UpdateSigningAuthority(ctx, sub.ID, prodcat.SigningAuthority{
		Rule:          prodcat.SigningRuleAll,
		RequiredCount: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, prodcat.SigningRuleAll, updated.SigningAuthority.Rule)
	assert.Equal(t, 2, updated.SigningAuthority.RequiredCount)
}

func TestNewStoreRequiresPool(t *testing.T) {
	_, err := db.New()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pool is required")
}

// ─── Helpers ───

func seedProduct(t *testing.T, store *db.Store, ctx context.Context) {
	t.Helper()

	_, err := store.CreateFamily(ctx, prodcat.FamilyDefinition{
		ID: "casa", Family: prodcat.ProductFamilyCASA,
		Name: map[string]string{"en": "CASA"},
	})
	require.NoError(t, err)

	_, err = store.CreateArchetype(ctx, prodcat.Archetype{
		ID: "current-account", FamilyID: "casa",
		Name: map[string]string{"en": "Current Account"},
	})
	require.NoError(t, err)

	_, err = store.CreateProduct(ctx, prodcat.Product{
		ID: "aed-ca", ArchetypeID: "current-account",
		Name:         map[string]string{"en": "AED Current Account"},
		Status:       prodcat.ProductStatusActive,
		ProductType:  prodcat.ProductTypePrimary,
		CurrencyCode: "AED",
		Provider: prodcat.RegulatoryProvider{
			ProviderID: "mal", Name: "Mal", Regulator: "CBUAE", RegulatoryCountry: "AE",
		},
		Compliance: prodcat.ComplianceConfig{ShariaCompliant: true},
		Eligibility: prodcat.EligibilityConfig{
			Geographic: prodcat.GeographicAvailability{
				Mode: prodcat.AvailabilityModeSpecificCountries, CountryCodes: []string{"AE"},
			},
		},
		CreatedBy: "test",
	})
	require.NoError(t, err)
}

func createActiveSubscription(t *testing.T, store *db.Store, ctx context.Context) prodcat.Subscription {
	t.Helper()

	sub, err := store.Subscribe(ctx, prodcat.SubscribeRequest{
		EntityID:   "customer-1",
		EntityType: prodcat.EntityTypeIndividual,
		ProductID:  "aed-ca",
		InitialParties: []prodcat.PartyInput{
			{CustomerID: "customer-1", Role: prodcat.PartyRolePrimaryHolder},
		},
		SigningAuthority: prodcat.SigningAuthority{Rule: prodcat.SigningRuleAnyOne, RequiredCount: 1},
	})
	require.NoError(t, err)

	activated, err := store.Activate(ctx, sub.ID, "ext-ref-123", []prodcat.CapabilityType{
		prodcat.CapabilityTypeView,
		prodcat.CapabilityTypeDomesticTransfers,
		prodcat.CapabilityTypeReceive,
	})
	require.NoError(t, err)
	return activated
}
