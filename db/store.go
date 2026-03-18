// Package db implements the prodcat services using PostgreSQL.
package db

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/laenen-partners/migrate"
	"github.com/laenen-partners/prodcat"
	gen "github.com/laenen-partners/prodcat/db/gen"
)

//go:embed migrations/*.sql
var migrations embed.FS

const migrationScope = "prodcat"

// Store implements prodcat.CatalogService and prodcat.SubscriptionService.
type Store struct {
	pool    *pgxpool.Pool
	queries *gen.Queries
}

// Option configures the Store.
type Option func(*Store)

// WithPool sets the pgx connection pool.
func WithPool(pool *pgxpool.Pool) Option {
	return func(s *Store) {
		s.pool = pool
	}
}

// New creates a new Store with the given options.
func New(opts ...Option) (*Store, error) {
	s := &Store{}
	for _, opt := range opts {
		opt(s)
	}
	if s.pool == nil {
		return nil, fmt.Errorf("prodcat/postgres: pool is required, use WithPool()")
	}
	s.queries = gen.New(s.pool)
	return s, nil
}

// Migrate runs all pending database migrations.
func (s *Store) Migrate(ctx context.Context) error {
	return migrate.Up(ctx, s.pool, migrations, migrationScope)
}

// Ensure Store implements both interfaces.
var (
	_ prodcat.CatalogService      = (*Store)(nil)
	_ prodcat.SubscriptionService = (*Store)(nil)
)

// ─── CatalogService: Families ───

func (s *Store) CreateFamily(ctx context.Context, f prodcat.FamilyDefinition) (prodcat.FamilyDefinition, error) {
	row, err := s.queries.CreateFamily(ctx, gen.CreateFamilyParams{
		ID:             f.ID,
		Family:         gen.ProductFamily(f.Family),
		Name:           f.Name,
		Description:    f.Description,
		Ruleset:        f.Ruleset,
		BaseRulesetIds: f.BaseRulesetIDs,
	})
	if err != nil {
		return prodcat.FamilyDefinition{}, fmt.Errorf("create family: %w", err)
	}
	return familyFromRow(row), nil
}

func (s *Store) GetFamily(ctx context.Context, id string) (prodcat.FamilyDefinition, error) {
	row, err := s.queries.GetFamily(ctx, id)
	if err != nil {
		return prodcat.FamilyDefinition{}, fmt.Errorf("get family: %w", err)
	}
	return familyFromRow(row), nil
}

func (s *Store) ListFamilies(ctx context.Context) ([]prodcat.FamilyDefinition, error) {
	rows, err := s.queries.ListFamilies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list families: %w", err)
	}
	result := make([]prodcat.FamilyDefinition, len(rows))
	for i, row := range rows {
		result[i] = familyFromRow(row)
	}
	return result, nil
}

func (s *Store) UpdateFamily(ctx context.Context, f prodcat.FamilyDefinition) (prodcat.FamilyDefinition, error) {
	row, err := s.queries.UpdateFamily(ctx, gen.UpdateFamilyParams{
		ID:             f.ID,
		Name:           f.Name,
		Description:    f.Description,
		Ruleset:        f.Ruleset,
		BaseRulesetIds: f.BaseRulesetIDs,
	})
	if err != nil {
		return prodcat.FamilyDefinition{}, fmt.Errorf("update family: %w", err)
	}
	return familyFromRow(row), nil
}

// ─── CatalogService: Archetypes ───

func (s *Store) CreateArchetype(ctx context.Context, a prodcat.Archetype) (prodcat.Archetype, error) {
	row, err := s.queries.CreateArchetype(ctx, gen.CreateArchetypeParams{
		ID:             a.ID,
		FamilyID:       a.FamilyID,
		Name:           a.Name,
		Description:    a.Description,
		Ruleset:        a.Ruleset,
		BaseRulesetIds: a.BaseRulesetIDs,
	})
	if err != nil {
		return prodcat.Archetype{}, fmt.Errorf("create archetype: %w", err)
	}
	return archetypeFromRow(row), nil
}

func (s *Store) GetArchetype(ctx context.Context, id string) (prodcat.Archetype, error) {
	row, err := s.queries.GetArchetype(ctx, id)
	if err != nil {
		return prodcat.Archetype{}, fmt.Errorf("get archetype: %w", err)
	}
	return archetypeFromRow(row), nil
}

func (s *Store) ListArchetypes(ctx context.Context, familyID string) ([]prodcat.Archetype, error) {
	rows, err := s.queries.ListArchetypes(ctx, familyID)
	if err != nil {
		return nil, fmt.Errorf("list archetypes: %w", err)
	}
	result := make([]prodcat.Archetype, len(rows))
	for i, row := range rows {
		result[i] = archetypeFromRow(row)
	}
	return result, nil
}

func (s *Store) UpdateArchetype(ctx context.Context, a prodcat.Archetype) (prodcat.Archetype, error) {
	row, err := s.queries.UpdateArchetype(ctx, gen.UpdateArchetypeParams{
		ID:             a.ID,
		Name:           a.Name,
		Description:    a.Description,
		Ruleset:        a.Ruleset,
		BaseRulesetIds: a.BaseRulesetIDs,
	})
	if err != nil {
		return prodcat.Archetype{}, fmt.Errorf("update archetype: %w", err)
	}
	return archetypeFromRow(row), nil
}

// ─── CatalogService: Products ───

func (s *Store) CreateProduct(ctx context.Context, p prodcat.Product) (prodcat.Product, error) {
	row, err := s.queries.CreateProduct(ctx, gen.CreateProductParams{
		ID:              p.ID,
		ArchetypeID:     p.ArchetypeID,
		Name:            p.Name,
		Description:     p.Description,
		Tagline:         p.Tagline,
		Status:          gen.ProductStatus(p.Status),
		ProductType:     gen.ProductType(p.ProductType),
		CurrencyCode:    p.CurrencyCode,
		ParentProductID: p.ParentProductID,
		ShariaCompliant: p.Compliance.ShariaCompliant,
		AvailabilityMode: gen.AvailabilityMode(p.Eligibility.Geographic.Mode),
		CountryCodes:    p.Eligibility.Geographic.CountryCodes,
		Ruleset:         p.Eligibility.Ruleset,
		BaseRulesetIds:  p.Eligibility.BaseRulesetIDs,
		ProviderID:      p.Provider.ProviderID,
		ProviderName:    p.Provider.Name,
		Regulator:       p.Provider.Regulator,
		LicenseNumber:   p.Provider.LicenseNumber,
		RegulatoryCountry: p.Provider.RegulatoryCountry,
		EffectiveFrom:   p.EffectiveFrom,
		EffectiveTo:     p.EffectiveTo,
		CreatedBy:       p.CreatedBy,
	})
	if err != nil {
		return prodcat.Product{}, fmt.Errorf("create product: %w", err)
	}
	return productFromRow(row), nil
}

func (s *Store) GetProduct(ctx context.Context, id string) (prodcat.Product, error) {
	row, err := s.queries.GetProduct(ctx, id)
	if err != nil {
		return prodcat.Product{}, fmt.Errorf("get product: %w", err)
	}
	return productFromRow(row), nil
}

func (s *Store) ListProducts(ctx context.Context, filter prodcat.ProductFilter) ([]prodcat.Product, error) {
	rows, err := s.queries.ListProducts(ctx, gen.ListProductsParams{
		ArchetypeID:  nilIfEmpty(filter.ArchetypeID),
		FamilyID:     nilIfEmpty(filter.FamilyID),
		Status:       nilIfEmpty(string(filter.Status)),
		CurrencyCode: nilIfEmpty(filter.CurrencyCode),
		CountryCode:  nilIfEmpty(filter.CountryCode),
		ResultLimit:  int32(filter.Limit),
		ResultOffset: int32(filter.Offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	result := make([]prodcat.Product, len(rows))
	for i, row := range rows {
		result[i] = productFromRow(row)
	}
	return result, nil
}

func (s *Store) UpdateProduct(ctx context.Context, p prodcat.Product) (prodcat.Product, error) {
	row, err := s.queries.UpdateProduct(ctx, gen.UpdateProductParams{
		ID:              p.ID,
		Name:            p.Name,
		Description:     p.Description,
		Tagline:         p.Tagline,
		CurrencyCode:    p.CurrencyCode,
		ShariaCompliant: p.Compliance.ShariaCompliant,
		AvailabilityMode: gen.AvailabilityMode(p.Eligibility.Geographic.Mode),
		CountryCodes:    p.Eligibility.Geographic.CountryCodes,
		Ruleset:         p.Eligibility.Ruleset,
		BaseRulesetIds:  p.Eligibility.BaseRulesetIDs,
		EffectiveFrom:   p.EffectiveFrom,
		EffectiveTo:     p.EffectiveTo,
	})
	if err != nil {
		return prodcat.Product{}, fmt.Errorf("update product: %w", err)
	}
	return productFromRow(row), nil
}

func (s *Store) TransitionProductStatus(ctx context.Context, id string, target prodcat.ProductStatus, reason string) (prodcat.Product, error) {
	row, err := s.queries.TransitionProductStatus(ctx, gen.TransitionProductStatusParams{
		ID:     id,
		Status: gen.ProductStatus(target),
	})
	if err != nil {
		return prodcat.Product{}, fmt.Errorf("transition product status: %w", err)
	}
	return productFromRow(row), nil
}

// ─── CatalogService: Base Rulesets ───

func (s *Store) CreateBaseRuleset(ctx context.Context, r prodcat.BaseRuleset) (prodcat.BaseRuleset, prodcat.RulesetValidation, error) {
	row, err := s.queries.CreateBaseRuleset(ctx, gen.CreateBaseRulesetParams{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Content:     r.Content,
		Version:     r.Version,
	})
	if err != nil {
		return prodcat.BaseRuleset{}, prodcat.RulesetValidation{}, fmt.Errorf("create base ruleset: %w", err)
	}
	// TODO: validate ruleset via eval engine
	validation := prodcat.RulesetValidation{Valid: true}
	return baseRulesetFromRow(row), validation, nil
}

func (s *Store) GetBaseRuleset(ctx context.Context, id string) (prodcat.BaseRuleset, error) {
	row, err := s.queries.GetBaseRuleset(ctx, id)
	if err != nil {
		return prodcat.BaseRuleset{}, fmt.Errorf("get base ruleset: %w", err)
	}
	return baseRulesetFromRow(row), nil
}

func (s *Store) ListBaseRulesets(ctx context.Context) ([]prodcat.BaseRuleset, error) {
	rows, err := s.queries.ListBaseRulesets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list base rulesets: %w", err)
	}
	result := make([]prodcat.BaseRuleset, len(rows))
	for i, row := range rows {
		result[i] = baseRulesetFromRow(row)
	}
	return result, nil
}

func (s *Store) UpdateBaseRuleset(ctx context.Context, r prodcat.BaseRuleset) (prodcat.BaseRuleset, prodcat.RulesetValidation, error) {
	row, err := s.queries.UpdateBaseRuleset(ctx, gen.UpdateBaseRulesetParams{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Content:     r.Content,
		Version:     r.Version,
	})
	if err != nil {
		return prodcat.BaseRuleset{}, prodcat.RulesetValidation{}, fmt.Errorf("update base ruleset: %w", err)
	}
	validation := prodcat.RulesetValidation{Valid: true}
	return baseRulesetFromRow(row), validation, nil
}

// ─── CatalogService: Discovery & Resolution ───

func (s *Store) ListAvailableProducts(ctx context.Context, filter prodcat.AvailableProductFilter) ([]prodcat.Product, error) {
	rows, err := s.queries.ListAvailableProducts(ctx, gen.ListAvailableProductsParams{
		CountryCode:  filter.CountryCode,
		CustomerType: nilIfEmpty(string(filter.CustomerType)),
		Family:       nilIfEmpty(string(filter.Family)),
	})
	if err != nil {
		return nil, fmt.Errorf("list available products: %w", err)
	}
	result := make([]prodcat.Product, len(rows))
	for i, row := range rows {
		result[i] = productFromRow(row)
	}
	return result, nil
}

func (s *Store) ResolveProductRuleset(ctx context.Context, productID string) (prodcat.ResolvedRuleset, error) {
	// TODO: merge family + archetype + base + product rulesets
	return prodcat.ResolvedRuleset{ProductID: productID}, nil
}

// ─── SubscriptionService ───

func (s *Store) Subscribe(ctx context.Context, req prodcat.SubscribeRequest) (prodcat.Subscription, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return prodcat.Subscription{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.queries.WithTx(tx)

	sub, err := qtx.CreateSubscription(ctx, gen.CreateSubscriptionParams{
		EntityID:       req.EntityID,
		EntityType:     gen.EntityType(req.EntityType),
		ProductID:      req.ProductID,
		SigningRule:     gen.SigningRule(req.SigningAuthority.Rule),
		RequiredCount:  int32(req.SigningAuthority.RequiredCount),
	})
	if err != nil {
		return prodcat.Subscription{}, fmt.Errorf("create subscription: %w", err)
	}

	for _, pi := range req.InitialParties {
		_, err := qtx.AddParty(ctx, gen.AddPartyParams{
			SubscriptionID: sub.ID,
			CustomerID:     pi.CustomerID,
			Role:           gen.PartyRole(pi.Role),
		})
		if err != nil {
			return prodcat.Subscription{}, fmt.Errorf("add party: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return prodcat.Subscription{}, fmt.Errorf("commit: %w", err)
	}

	return s.GetSubscription(ctx, sub.ID)
}

func (s *Store) GetSubscription(ctx context.Context, id string) (prodcat.Subscription, error) {
	row, err := s.queries.GetSubscription(ctx, id)
	if err != nil {
		return prodcat.Subscription{}, fmt.Errorf("get subscription: %w", err)
	}
	sub := subscriptionFromRow(row)

	parties, err := s.queries.ListParties(ctx, id)
	if err != nil {
		return prodcat.Subscription{}, fmt.Errorf("list parties: %w", err)
	}
	sub.Parties = make([]prodcat.Party, len(parties))
	for i, p := range parties {
		sub.Parties[i] = partyFromRow(p)
	}

	caps, err := s.queries.ListCapabilities(ctx, id)
	if err != nil {
		return prodcat.Subscription{}, fmt.Errorf("list capabilities: %w", err)
	}
	sub.Capabilities = make([]prodcat.Capability, len(caps))
	for i, c := range caps {
		sub.Capabilities[i] = capabilityFromRow(c)
	}

	return sub, nil
}

func (s *Store) ListSubscriptions(ctx context.Context, filter prodcat.SubscriptionFilter) ([]prodcat.Subscription, error) {
	rows, err := s.queries.ListSubscriptions(ctx, gen.ListSubscriptionsParams{
		EntityID:   nilIfEmpty(filter.EntityID),
		CustomerID: nilIfEmpty(filter.CustomerID),
		Status:     nilIfEmpty(string(filter.Status)),
		ResultLimit:  int32(filter.Limit),
		ResultOffset: int32(filter.Offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	result := make([]prodcat.Subscription, len(rows))
	for i, row := range rows {
		result[i] = subscriptionFromRow(row)
	}
	return result, nil
}

func (s *Store) Activate(ctx context.Context, id string, externalRef string, capabilities []prodcat.CapabilityType) (prodcat.Subscription, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return prodcat.Subscription{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.queries.WithTx(tx)

	if err := qtx.ActivateSubscription(ctx, gen.ActivateSubscriptionParams{
		ID:          id,
		ExternalRef: &externalRef,
	}); err != nil {
		return prodcat.Subscription{}, fmt.Errorf("activate subscription: %w", err)
	}

	for _, ct := range capabilities {
		_, err := qtx.CreateCapability(ctx, gen.CreateCapabilityParams{
			SubscriptionID: id,
			CapabilityType: gen.CapabilityType(ct),
			Status:         gen.CapabilityStatusActive,
		})
		if err != nil {
			return prodcat.Subscription{}, fmt.Errorf("create capability: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return prodcat.Subscription{}, fmt.Errorf("commit: %w", err)
	}

	return s.GetSubscription(ctx, id)
}

func (s *Store) Cancel(ctx context.Context, id string, reason string) (prodcat.Subscription, error) {
	if err := s.queries.CancelSubscription(ctx, id); err != nil {
		return prodcat.Subscription{}, fmt.Errorf("cancel subscription: %w", err)
	}
	return s.GetSubscription(ctx, id)
}

func (s *Store) Disable(ctx context.Context, id string, reason prodcat.DisabledReason, message string) (prodcat.Subscription, error) {
	if err := s.queries.DisableSubscription(ctx, gen.DisableSubscriptionParams{
		ID:             id,
		DisabledReason: gen.DisabledReason(reason),
		DisabledMessage: message,
	}); err != nil {
		return prodcat.Subscription{}, fmt.Errorf("disable subscription: %w", err)
	}
	return s.GetSubscription(ctx, id)
}

func (s *Store) Enable(ctx context.Context, id string) (prodcat.Subscription, error) {
	if err := s.queries.EnableSubscription(ctx, id); err != nil {
		return prodcat.Subscription{}, fmt.Errorf("enable subscription: %w", err)
	}
	return s.GetSubscription(ctx, id)
}

func (s *Store) DisableCapability(ctx context.Context, subscriptionID string, capabilityID string, reason prodcat.DisabledReason, message string) (prodcat.Subscription, error) {
	if err := s.queries.DisableCapability(ctx, gen.DisableCapabilityParams{
		ID:             capabilityID,
		DisabledReason: gen.DisabledReason(reason),
		DisabledMessage: message,
	}); err != nil {
		return prodcat.Subscription{}, fmt.Errorf("disable capability: %w", err)
	}
	return s.GetSubscription(ctx, subscriptionID)
}

func (s *Store) EnableCapability(ctx context.Context, subscriptionID string, capabilityID string) (prodcat.Subscription, error) {
	if err := s.queries.EnableCapability(ctx, capabilityID); err != nil {
		return prodcat.Subscription{}, fmt.Errorf("enable capability: %w", err)
	}
	return s.GetSubscription(ctx, subscriptionID)
}

func (s *Store) Evaluate(ctx context.Context, subscriptionID string) (prodcat.Subscription, []prodcat.EvalDelta, error) {
	// TODO: load merged ruleset, hydrate EvaluationInput, run eval engine
	sub, err := s.GetSubscription(ctx, subscriptionID)
	return sub, nil, err
}

func (s *Store) EvaluateParty(ctx context.Context, subscriptionID string, partyID string) (prodcat.Party, []prodcat.EvalDelta, error) {
	// TODO: load merged ruleset, hydrate EvaluationInput for party, run eval engine
	return prodcat.Party{}, nil, nil
}

func (s *Store) CheckAccess(ctx context.Context, entityID string) ([]prodcat.SubscriptionAccess, error) {
	// TODO: evaluate all active subscriptions for entity
	return nil, nil
}

func (s *Store) AddParty(ctx context.Context, subscriptionID string, customerID string, role prodcat.PartyRole) (prodcat.Subscription, error) {
	_, err := s.queries.AddParty(ctx, gen.AddPartyParams{
		SubscriptionID: subscriptionID,
		CustomerID:     customerID,
		Role:           gen.PartyRole(role),
	})
	if err != nil {
		return prodcat.Subscription{}, fmt.Errorf("add party: %w", err)
	}
	return s.GetSubscription(ctx, subscriptionID)
}

func (s *Store) RemoveParty(ctx context.Context, subscriptionID string, partyID string, reason string) (prodcat.Subscription, error) {
	if err := s.queries.RemoveParty(ctx, partyID); err != nil {
		return prodcat.Subscription{}, fmt.Errorf("remove party: %w", err)
	}
	return s.GetSubscription(ctx, subscriptionID)
}

func (s *Store) UpdateSigningAuthority(ctx context.Context, subscriptionID string, authority prodcat.SigningAuthority) (prodcat.Subscription, error) {
	if err := s.queries.UpdateSigningAuthority(ctx, gen.UpdateSigningAuthorityParams{
		ID:            subscriptionID,
		SigningRule:    gen.SigningRule(authority.Rule),
		RequiredCount: int32(authority.RequiredCount),
	}); err != nil {
		return prodcat.Subscription{}, fmt.Errorf("update signing authority: %w", err)
	}
	return s.GetSubscription(ctx, subscriptionID)
}

// ─── Helpers ───

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
