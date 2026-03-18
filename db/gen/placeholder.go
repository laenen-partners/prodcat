// Package db contains SQLC-generated database access code.
// Run `sqlc generate` from the postgres/ directory to regenerate.
//
// This file is a placeholder so go mod tidy works before code generation.
package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Enum types matching PostgreSQL enums. SQLC will regenerate these.
type ProductFamily string
type ProductStatus string
type ProductType string
type AvailabilityMode string
type EntityType string
type SubscriptionStatus string
type PartyRole string
type SigningRule string
type CapabilityType string
type CapabilityStatus string
type DisabledReason string

const CapabilityStatusActive CapabilityStatus = "active"

// DBTX is the interface for database operations.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Queries wraps the database connection.
type Queries struct {
	db DBTX
}

// New creates a new Queries instance.
func New(db DBTX) *Queries {
	return &Queries{db: db}
}

// WithTx returns a new Queries that uses the given transaction.
func (q *Queries) WithTx(tx pgx.Tx) *Queries {
	return &Queries{db: tx}
}

// Row types matching the database tables. SQLC will regenerate these.

type Family struct {
	ID             string
	Family         ProductFamily
	Name           map[string]string
	Description    map[string]string
	Ruleset        []byte
	BaseRulesetIds []string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Archetype struct {
	ID             string
	FamilyID       string
	Name           map[string]string
	Description    map[string]string
	Ruleset        []byte
	BaseRulesetIds []string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Product struct {
	ID               string
	ArchetypeID      string
	Name             map[string]string
	Description      map[string]string
	Tagline          map[string]string
	Status           ProductStatus
	ProductType      ProductType
	CurrencyCode     string
	ParentProductID  *string
	ProviderID       string
	ProviderName     string
	Regulator        string
	LicenseNumber    string
	RegulatoryCountry string
	ShariaCompliant  bool
	AvailabilityMode AvailabilityMode
	CountryCodes     []string
	Ruleset          []byte
	BaseRulesetIds   []string
	EffectiveFrom    *time.Time
	EffectiveTo      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CreatedBy        string
}

type BaseRuleset struct {
	ID          string
	Name        string
	Description string
	Content     []byte
	Version     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Subscription struct {
	ID                     string
	ProductID              string
	EntityID               string
	EntityType             EntityType
	Status                 SubscriptionStatus
	SigningRule             SigningRule
	RequiredCount          int32
	ParentSubscriptionID   *string
	ExternalRef            *string
	Disabled               bool
	DisabledReason         DisabledReason
	DisabledMessage        string
	DisabledAt             *time.Time
	EvalState              []byte
	CreatedAt              time.Time
	ActivatedAt            *time.Time
	CanceledAt             *time.Time
}

type Party struct {
	ID              string
	SubscriptionID  string
	CustomerID      string
	Role            PartyRole
	RequirementsMet bool
	Disabled        bool
	DisabledReason  DisabledReason
	DisabledMessage string
	DisabledAt      *time.Time
	EvalState       []byte
	AddedAt         time.Time
	RemovedAt       *time.Time
}

type Capability struct {
	ID              string
	SubscriptionID  string
	CapabilityType  CapabilityType
	Status          CapabilityStatus
	Disabled        bool
	DisabledReason  DisabledReason
	DisabledMessage string
	DisabledAt      *time.Time
	CreatedAt       time.Time
}

// Param types for queries. SQLC will regenerate these.

type CreateFamilyParams struct {
	ID             string
	Family         ProductFamily
	Name           map[string]string
	Description    map[string]string
	Ruleset        []byte
	BaseRulesetIds []string
}

type UpdateFamilyParams struct {
	ID             string
	Name           map[string]string
	Description    map[string]string
	Ruleset        []byte
	BaseRulesetIds []string
}

type CreateArchetypeParams struct {
	ID             string
	FamilyID       string
	Name           map[string]string
	Description    map[string]string
	Ruleset        []byte
	BaseRulesetIds []string
}

type UpdateArchetypeParams struct {
	ID             string
	Name           map[string]string
	Description    map[string]string
	Ruleset        []byte
	BaseRulesetIds []string
}

type CreateProductParams struct {
	ID                string
	ArchetypeID       string
	Name              map[string]string
	Description       map[string]string
	Tagline           map[string]string
	Status            ProductStatus
	ProductType       ProductType
	CurrencyCode      string
	ParentProductID   *string
	ProviderID        string
	ProviderName      string
	Regulator         string
	LicenseNumber     string
	RegulatoryCountry string
	ShariaCompliant   bool
	AvailabilityMode  AvailabilityMode
	CountryCodes      []string
	Ruleset           []byte
	BaseRulesetIds    []string
	EffectiveFrom     *time.Time
	EffectiveTo       *time.Time
	CreatedBy         string
}

type UpdateProductParams struct {
	ID               string
	Name             map[string]string
	Description      map[string]string
	Tagline          map[string]string
	CurrencyCode     string
	ShariaCompliant  bool
	AvailabilityMode AvailabilityMode
	CountryCodes     []string
	Ruleset          []byte
	BaseRulesetIds   []string
	EffectiveFrom    *time.Time
	EffectiveTo      *time.Time
}

type TransitionProductStatusParams struct {
	ID     string
	Status ProductStatus
}

type ListProductsParams struct {
	ArchetypeID  *string
	FamilyID     *string
	Status       *string
	CurrencyCode *string
	CountryCode  *string
	ResultLimit  int32
	ResultOffset int32
}

type ListAvailableProductsParams struct {
	CountryCode  string
	CustomerType *string
	Family       *string
}

type CreateBaseRulesetParams struct {
	ID          string
	Name        string
	Description string
	Content     []byte
	Version     string
}

type UpdateBaseRulesetParams struct {
	ID          string
	Name        string
	Description string
	Content     []byte
	Version     string
}

type CreateSubscriptionParams struct {
	EntityID      string
	EntityType    EntityType
	ProductID     string
	SigningRule    SigningRule
	RequiredCount int32
}

type ListSubscriptionsParams struct {
	EntityID     *string
	CustomerID   *string
	Status       *string
	ResultLimit  int32
	ResultOffset int32
}

type ActivateSubscriptionParams struct {
	ID          string
	ExternalRef *string
}

type DisableSubscriptionParams struct {
	ID              string
	DisabledReason  DisabledReason
	DisabledMessage string
}

type UpdateSigningAuthorityParams struct {
	ID            string
	SigningRule    SigningRule
	RequiredCount int32
}

type AddPartyParams struct {
	SubscriptionID string
	CustomerID     string
	Role           PartyRole
}

type CreateCapabilityParams struct {
	SubscriptionID string
	CapabilityType CapabilityType
	Status         CapabilityStatus
}

type DisableCapabilityParams struct {
	ID              string
	DisabledReason  DisabledReason
	DisabledMessage string
}

// Query method stubs. SQLC will regenerate these with actual implementations.

func (q *Queries) CreateFamily(ctx context.Context, arg CreateFamilyParams) (Family, error) {
	return Family{}, nil
}
func (q *Queries) GetFamily(ctx context.Context, id string) (Family, error) {
	return Family{}, nil
}
func (q *Queries) ListFamilies(ctx context.Context) ([]Family, error) {
	return nil, nil
}
func (q *Queries) UpdateFamily(ctx context.Context, arg UpdateFamilyParams) (Family, error) {
	return Family{}, nil
}
func (q *Queries) CreateArchetype(ctx context.Context, arg CreateArchetypeParams) (Archetype, error) {
	return Archetype{}, nil
}
func (q *Queries) GetArchetype(ctx context.Context, id string) (Archetype, error) {
	return Archetype{}, nil
}
func (q *Queries) ListArchetypes(ctx context.Context, familyID string) ([]Archetype, error) {
	return nil, nil
}
func (q *Queries) UpdateArchetype(ctx context.Context, arg UpdateArchetypeParams) (Archetype, error) {
	return Archetype{}, nil
}
func (q *Queries) CreateProduct(ctx context.Context, arg CreateProductParams) (Product, error) {
	return Product{}, nil
}
func (q *Queries) GetProduct(ctx context.Context, id string) (Product, error) {
	return Product{}, nil
}
func (q *Queries) ListProducts(ctx context.Context, arg ListProductsParams) ([]Product, error) {
	return nil, nil
}
func (q *Queries) UpdateProduct(ctx context.Context, arg UpdateProductParams) (Product, error) {
	return Product{}, nil
}
func (q *Queries) TransitionProductStatus(ctx context.Context, arg TransitionProductStatusParams) (Product, error) {
	return Product{}, nil
}
func (q *Queries) ListAvailableProducts(ctx context.Context, arg ListAvailableProductsParams) ([]Product, error) {
	return nil, nil
}
func (q *Queries) CreateBaseRuleset(ctx context.Context, arg CreateBaseRulesetParams) (BaseRuleset, error) {
	return BaseRuleset{}, nil
}
func (q *Queries) GetBaseRuleset(ctx context.Context, id string) (BaseRuleset, error) {
	return BaseRuleset{}, nil
}
func (q *Queries) ListBaseRulesets(ctx context.Context) ([]BaseRuleset, error) {
	return nil, nil
}
func (q *Queries) UpdateBaseRuleset(ctx context.Context, arg UpdateBaseRulesetParams) (BaseRuleset, error) {
	return BaseRuleset{}, nil
}
func (q *Queries) CreateSubscription(ctx context.Context, arg CreateSubscriptionParams) (Subscription, error) {
	return Subscription{}, nil
}
func (q *Queries) GetSubscription(ctx context.Context, id string) (Subscription, error) {
	return Subscription{}, nil
}
func (q *Queries) ListSubscriptions(ctx context.Context, arg ListSubscriptionsParams) ([]Subscription, error) {
	return nil, nil
}
func (q *Queries) ActivateSubscription(ctx context.Context, arg ActivateSubscriptionParams) error {
	return nil
}
func (q *Queries) CancelSubscription(ctx context.Context, id string) error {
	return nil
}
func (q *Queries) DisableSubscription(ctx context.Context, arg DisableSubscriptionParams) error {
	return nil
}
func (q *Queries) EnableSubscription(ctx context.Context, id string) error {
	return nil
}
func (q *Queries) UpdateSigningAuthority(ctx context.Context, arg UpdateSigningAuthorityParams) error {
	return nil
}
func (q *Queries) AddParty(ctx context.Context, arg AddPartyParams) (Party, error) {
	return Party{}, nil
}
func (q *Queries) ListParties(ctx context.Context, subscriptionID string) ([]Party, error) {
	return nil, nil
}
func (q *Queries) RemoveParty(ctx context.Context, id string) error {
	return nil
}
func (q *Queries) CreateCapability(ctx context.Context, arg CreateCapabilityParams) (Capability, error) {
	return Capability{}, nil
}
func (q *Queries) ListCapabilities(ctx context.Context, subscriptionID string) ([]Capability, error) {
	return nil, nil
}
func (q *Queries) DisableCapability(ctx context.Context, arg DisableCapabilityParams) error {
	return nil
}
func (q *Queries) EnableCapability(ctx context.Context, id string) error {
	return nil
}

// Keep pgxpool imported for the New() function signature compatibility.
var _ *pgxpool.Pool
