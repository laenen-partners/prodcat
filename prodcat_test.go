package prodcat_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/laenen-partners/entitystore"
	"github.com/laenen-partners/prodcat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _sharedConnStr string

func sharedTestClient(t *testing.T) *prodcat.Client {
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

	client := prodcat.NewClient(es)

	return client
}

// validRulesetContent returns minimal valid ruleset YAML for tests.
func validRulesetContent() []byte {
	return []byte("evaluations:\n- name: test_check\n  expression: \"true\"\n  writes: test_result")
}

// ─── Product CRUD ───

func TestProductRegistrationAndLookup(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	_, err := client.RegisterProduct(ctx, prodcat.Product{
		ProductID: "savings-basic", Name: "Basic Savings Account",
		Tags:         []string{"family:casa", "type:savings"},
		CurrencyCode: "USD",
		Availability: prodcat.GeoAvailability{
			Mode: prodcat.AvailabilityModeSpecificCountries, CountryCodes: []string{"US"},
		},
	})
	require.NoError(t, err)

	p, err := client.GetProduct(ctx, "savings-basic")
	require.NoError(t, err)
	assert.Equal(t, "Basic Savings Account", p.Name)
	assert.Contains(t, p.Tags, "family:casa")
}

// ─── Product Validation ───

func TestRegisterProduct_Validation(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	t.Run("empty product_id", func(t *testing.T) {
		_, err := client.RegisterProduct(ctx, prodcat.Product{
			Name: "No ID",
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, prodcat.ErrValidation))
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := client.RegisterProduct(ctx, prodcat.Product{
			ProductID: "no-name",
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, prodcat.ErrValidation))
	})
}

func TestRegisterProduct_DuplicateReturnsAlreadyExists(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	_, err := client.RegisterProduct(ctx, prodcat.Product{
		ProductID: "dup-test", Name: "First",
	})
	require.NoError(t, err)

	_, err = client.RegisterProduct(ctx, prodcat.Product{
		ProductID: "dup-test", Name: "Second",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, prodcat.ErrAlreadyExists))
}

// ─── Ruleset Validation ───

func TestCreateRuleset_Validation(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	t.Run("empty name", func(t *testing.T) {
		_, err := client.CreateRuleset(ctx, prodcat.Ruleset{
			Content: validRulesetContent(),
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, prodcat.ErrValidation))
	})

	t.Run("empty content", func(t *testing.T) {
		_, err := client.CreateRuleset(ctx, prodcat.Ruleset{
			Name: "No Content",
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, prodcat.ErrValidation))
	})
}

func TestCreateRuleset_DuplicateReturnsAlreadyExists(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	_, err := client.CreateRuleset(ctx, prodcat.Ruleset{
		ID: "dup-rs", Name: "First", Content: validRulesetContent(),
	})
	require.NoError(t, err)

	_, err = client.CreateRuleset(ctx, prodcat.Ruleset{
		ID: "dup-rs", Name: "Second", Content: validRulesetContent(),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, prodcat.ErrAlreadyExists))
}

// ─── Not Found ───

func TestGetProduct_NotFoundReturnsErrNotFound(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()

	_, err := client.GetProduct(ctx, "nonexistent-product")
	require.Error(t, err)
	assert.True(t, errors.Is(err, prodcat.ErrNotFound))
}

func TestGetRuleset_NotFoundReturnsErrNotFound(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()

	_, err := client.GetRuleset(ctx, "nonexistent-ruleset")
	require.Error(t, err)
	assert.True(t, errors.Is(err, prodcat.ErrNotFound))
}

// ─── Disable / Enable Ruleset ───

func TestDisableEnableRuleset(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	_, err := client.CreateRuleset(ctx, prodcat.Ruleset{
		ID: "disable-test", Name: "Disable Test", Content: validRulesetContent(),
	})
	require.NoError(t, err)

	// Disable
	rs, err := client.DisableRuleset(ctx, "disable-test", prodcat.DisabledReasonDeleted)
	require.NoError(t, err)
	assert.True(t, rs.Disabled)
	assert.Equal(t, prodcat.DisabledReasonDeleted, rs.DisabledReason)

	// Verify persisted
	rs, err = client.GetRuleset(ctx, "disable-test")
	require.NoError(t, err)
	assert.True(t, rs.Disabled)

	// Enable
	rs, err = client.EnableRuleset(ctx, "disable-test")
	require.NoError(t, err)
	assert.False(t, rs.Disabled)
	assert.Empty(t, rs.DisabledReason)
}

// ─── AddRuleset: Referential Integrity ───

func TestAddRuleset_RejectsNonexistentRuleset(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	_, err := client.RegisterProduct(ctx, prodcat.Product{
		ProductID: "add-nonexistent-test", Name: "Test",
	})
	require.NoError(t, err)

	_, err = client.AddRuleset(ctx, "add-nonexistent-test", "ghost-ruleset")
	require.Error(t, err)
	assert.True(t, errors.Is(err, prodcat.ErrNotFound))
}

func TestAddRuleset_RejectsDisabledRuleset(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	_, err := client.RegisterProduct(ctx, prodcat.Product{
		ProductID: "add-disabled-test", Name: "Test",
	})
	require.NoError(t, err)

	_, err = client.CreateRuleset(ctx, prodcat.Ruleset{
		ID: "disabled-rs", Name: "Disabled", Content: validRulesetContent(),
	})
	require.NoError(t, err)

	_, err = client.DisableRuleset(ctx, "disabled-rs", prodcat.DisabledReasonDeleted)
	require.NoError(t, err)

	_, err = client.AddRuleset(ctx, "add-disabled-test", "disabled-rs")
	require.Error(t, err)
	assert.True(t, errors.Is(err, prodcat.ErrRulesetDisabled))
}

// ─── RegisterProduct: Rejects Nonexistent Rulesets ───

func TestRegisterProduct_RejectsNonexistentRuleset(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	_, err := client.RegisterProduct(ctx, prodcat.Product{
		ProductID: "reg-ghost-rs", Name: "Test",
		BaseRulesetIDs: []string{"does-not-exist"},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, prodcat.ErrNotFound))
}

func TestRegisterProduct_RejectsDisabledRuleset(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	_, err := client.CreateRuleset(ctx, prodcat.Ruleset{
		ID: "reg-disabled", Name: "Will Disable", Content: validRulesetContent(),
	})
	require.NoError(t, err)

	_, err = client.DisableRuleset(ctx, "reg-disabled", prodcat.DisabledReasonDeleted)
	require.NoError(t, err)

	_, err = client.RegisterProduct(ctx, prodcat.Product{
		ProductID: "reg-disabled-rs", Name: "Test",
		BaseRulesetIDs: []string{"reg-disabled"},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, prodcat.ErrRulesetDisabled))
}

// ─── ResolveRuleset: Skips Disabled ───

func TestResolveRuleset_SkipsDisabledBaseRuleset(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	_, err := client.CreateRuleset(ctx, prodcat.Ruleset{
		ID: "active-rs", Name: "Active",
		Content: []byte("evaluations:\n- name: check_active\n  expression: \"true\"\n  writes: active_result"),
	})
	require.NoError(t, err)

	_, err = client.CreateRuleset(ctx, prodcat.Ruleset{
		ID: "will-disable-rs", Name: "Will Disable",
		Content: []byte("evaluations:\n- name: check_disabled\n  expression: \"true\"\n  writes: disabled_result"),
	})
	require.NoError(t, err)

	_, err = client.RegisterProduct(ctx, prodcat.Product{
		ProductID: "resolve-skip-test", Name: "Test",
		BaseRulesetIDs: []string{"active-rs", "will-disable-rs"},
	})
	require.NoError(t, err)

	_, err = client.DisableRuleset(ctx, "will-disable-rs", prodcat.DisabledReasonDeleted)
	require.NoError(t, err)

	resolved, err := client.ResolveRuleset(ctx, "resolve-skip-test")
	require.NoError(t, err)
	assert.Len(t, resolved.Layers, 1)
	assert.Equal(t, "active-rs", resolved.Layers[0].SourceID)
	assert.Contains(t, string(resolved.Merged), "check_active")
	assert.NotContains(t, string(resolved.Merged), "check_disabled")
}

// ─── Ruleset Resolution ───

func TestResolveRuleset(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()

	data, err := os.ReadFile("examples/2026031801_platform_subscription.yaml")
	require.NoError(t, err)
	err = client.Import(ctx, "examples/2026031801_platform_subscription.yaml", data)
	require.NoError(t, err)

	resolved, err := client.ResolveRuleset(ctx, "platform-subscription")
	require.NoError(t, err)
	assert.Equal(t, "platform-subscription", resolved.ProductID)
	assert.Len(t, resolved.Layers, 1)
	assert.Equal(t, "base", resolved.Layers[0].Source)
	assert.Equal(t, "base-platform-access", resolved.Layers[0].SourceID)
	assert.NotEmpty(t, resolved.Merged)
	assert.Contains(t, string(resolved.Merged), "email_verified")
}

// ─── Add / Remove Ruleset ───

func TestAddRemoveRuleset(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	_, err := client.RegisterProduct(ctx, prodcat.Product{
		ProductID: "test-ruleset-mgmt", Name: "Test",
	})
	require.NoError(t, err)

	_, err = client.CreateRuleset(ctx, prodcat.Ruleset{
		ID: "rs-1", Name: "Ruleset 1", Content: validRulesetContent(),
	})
	require.NoError(t, err)

	p, err := client.AddRuleset(ctx, "test-ruleset-mgmt", "rs-1")
	require.NoError(t, err)
	assert.Contains(t, p.BaseRulesetIDs, "rs-1")

	p, err = client.AddRuleset(ctx, "test-ruleset-mgmt", "rs-1")
	require.NoError(t, err)
	assert.Len(t, p.BaseRulesetIDs, 1)

	p, err = client.RemoveRuleset(ctx, "test-ruleset-mgmt", "rs-1")
	require.NoError(t, err)
	assert.Empty(t, p.BaseRulesetIDs)
}

// ─── Import OnConflict ───

func TestImport_OnConflictUpdate(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()

	data, err := os.ReadFile("examples/2026031801_platform_subscription.yaml")
	require.NoError(t, err)

	// Default: upsert — importing twice should succeed.
	err = client.Import(ctx, "import-upsert-test.yaml", data)
	require.NoError(t, err)
	err = client.Import(ctx, "import-upsert-test.yaml", data)
	require.NoError(t, err)
}

func TestImport_OnConflictError(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()

	data := []byte(`kind: catalog
rulesets:
  - id: conflict-test-rs
    name: Conflict Test RS
    evaluations:
      - name: check
        expression: "true"
        writes: result
products:
  - product_id: conflict-test-prod
    name: Conflict Test Product
`)

	// First import with OnConflictError should succeed.
	err := client.Import(ctx, "import-error-test.yaml", data, prodcat.WithOnConflict(prodcat.OnConflictError))
	require.NoError(t, err)

	// Second import with OnConflictError should fail with ErrAlreadyExists.
	err = client.Import(ctx, "import-error-test.yaml", data, prodcat.WithOnConflict(prodcat.OnConflictError))
	require.Error(t, err)
	assert.True(t, errors.Is(err, prodcat.ErrAlreadyExists))
}

// ─── Validation ───

func TestValidateCatalogDefinition(t *testing.T) {
	t.Run("valid definition", func(t *testing.T) {
		def := &prodcat.CatalogDefinition{
			Kind: "catalog",
			Rulesets: []prodcat.CatalogRuleset{
				{ID: "rs-1", Name: "Test", Evaluations: []prodcat.CatalogEvaluation{
					{Name: "check", Expression: "true", Writes: "result"},
				}},
			},
			Products: []prodcat.CatalogProduct{
				{ProductID: "p-1", Name: "Test"},
			},
		}
		err := prodcat.ValidateCatalogDefinition(def)
		assert.NoError(t, err)
	})

	t.Run("wrong kind", func(t *testing.T) {
		def := &prodcat.CatalogDefinition{Kind: "seed"}
		err := prodcat.ValidateCatalogDefinition(def)
		require.Error(t, err)
		assert.True(t, errors.Is(err, prodcat.ErrValidation))
	})

	t.Run("empty catalog", func(t *testing.T) {
		def := &prodcat.CatalogDefinition{Kind: "catalog"}
		err := prodcat.ValidateCatalogDefinition(def)
		require.Error(t, err)
		assert.True(t, errors.Is(err, prodcat.ErrValidation))
	})

	t.Run("duplicate product_id", func(t *testing.T) {
		def := &prodcat.CatalogDefinition{
			Kind: "catalog",
			Products: []prodcat.CatalogProduct{
				{ProductID: "dup", Name: "First"},
				{ProductID: "dup", Name: "Second"},
			},
		}
		err := prodcat.ValidateCatalogDefinition(def)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate product_id")
	})

	t.Run("duplicate ruleset id", func(t *testing.T) {
		def := &prodcat.CatalogDefinition{
			Kind: "catalog",
			Rulesets: []prodcat.CatalogRuleset{
				{ID: "dup", Name: "First", Evaluations: []prodcat.CatalogEvaluation{
					{Name: "check", Expression: "true", Writes: "result"},
				}},
				{ID: "dup", Name: "Second", Evaluations: []prodcat.CatalogEvaluation{
					{Name: "check2", Expression: "true", Writes: "result2"},
				}},
			},
		}
		err := prodcat.ValidateCatalogDefinition(def)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate id")
	})
}

func TestValidateRulesetContent(t *testing.T) {
	t.Run("valid content", func(t *testing.T) {
		err := prodcat.ValidateRulesetContent(validRulesetContent())
		assert.NoError(t, err)
	})

	t.Run("empty evaluations", func(t *testing.T) {
		err := prodcat.ValidateRulesetContent([]byte("evaluations: []"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("missing writes", func(t *testing.T) {
		err := prodcat.ValidateRulesetContent([]byte("evaluations:\n- name: test\n  expression: \"true\""))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "writes")
	})

	t.Run("missing expression", func(t *testing.T) {
		err := prodcat.ValidateRulesetContent([]byte("evaluations:\n- name: test\n  writes: result"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expression")
	})

	t.Run("invalid cache_ttl", func(t *testing.T) {
		err := prodcat.ValidateRulesetContent([]byte("evaluations:\n- name: test\n  expression: \"true\"\n  writes: result\n  cache_ttl: \"not-a-duration\""))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cache_ttl")
	})
}

// ─── Soft Delete ───

func TestDeleteProduct(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	_, err := client.RegisterProduct(ctx, prodcat.Product{
		ProductID: "delete-me", Name: "Delete Me",
	})
	require.NoError(t, err)

	err = client.DeleteProduct(ctx, "delete-me")
	require.NoError(t, err)

	_, err = client.GetProduct(ctx, "delete-me")
	require.Error(t, err)
	assert.True(t, errors.Is(err, prodcat.ErrNotFound))
}

func TestDeleteRuleset(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	_, err := client.CreateRuleset(ctx, prodcat.Ruleset{
		ID: "delete-me-rs", Name: "Delete Me", Content: validRulesetContent(),
	})
	require.NoError(t, err)

	err = client.DeleteRuleset(ctx, "delete-me-rs")
	require.NoError(t, err)

	_, err = client.GetRuleset(ctx, "delete-me-rs")
	require.Error(t, err)
	assert.True(t, errors.Is(err, prodcat.ErrNotFound))
}

// ─── Graph Relations ───

func TestAddRuleset_CreatesGraphRelation(t *testing.T) {
	client := sharedTestClient(t)
	ctx := context.Background()
	_, err := client.RegisterProduct(ctx, prodcat.Product{
		ProductID: "graph-test", Name: "Graph Test",
	})
	require.NoError(t, err)

	_, err = client.CreateRuleset(ctx, prodcat.Ruleset{
		ID: "graph-rs", Name: "Graph RS", Content: validRulesetContent(),
	})
	require.NoError(t, err)

	// AddRuleset should create both the BaseRulesetIDs entry and the graph relation.
	p, err := client.AddRuleset(ctx, "graph-test", "graph-rs")
	require.NoError(t, err)
	assert.Contains(t, p.BaseRulesetIDs, "graph-rs")

	// RemoveRuleset should remove both.
	p, err = client.RemoveRuleset(ctx, "graph-test", "graph-rs")
	require.NoError(t, err)
	assert.Empty(t, p.BaseRulesetIDs)
}
