// Package gen contains database access code.
// This is a hand-written implementation matching the SQLC interface contract.
// Run `sqlc generate` from the db/ directory to regenerate from SQL queries.
package gen

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Enum types matching PostgreSQL enums.
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

type Queries struct{ db DBTX }

func New(db DBTX) *Queries         { return &Queries{db: db} }
func (q *Queries) WithTx(tx pgx.Tx) *Queries { return &Queries{db: tx} }

// Keep pgxpool imported.
var _ *pgxpool.Pool

// ─── Row types ───

type Family struct {
	ID, FamilyID    string
	Family          ProductFamily
	Name            map[string]string
	Description     map[string]string
	Ruleset         []byte
	BaseRulesetIds  []string
	CreatedAt       time.Time
	UpdatedAt       time.Time
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
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CreatedBy         string
}

type BaseRuleset struct {
	ID, Name, Description string
	Content               []byte
	Version               string
	CreatedAt, UpdatedAt  time.Time
}

type Subscription struct {
	ID, ProductID, EntityID string
	EntityType              EntityType
	Status                  SubscriptionStatus
	SigningRule              SigningRule
	RequiredCount           int32
	ParentSubscriptionID    *string
	ExternalRef             *string
	Disabled                bool
	DisabledReason          DisabledReason
	DisabledMessage         string
	DisabledAt              *time.Time
	EvalState               []byte
	CreatedAt               time.Time
	ActivatedAt, CanceledAt *time.Time
}

type Party struct {
	ID, SubscriptionID, CustomerID string
	Role                           PartyRole
	RequirementsMet                bool
	Disabled                       bool
	DisabledReason                 DisabledReason
	DisabledMessage                string
	DisabledAt                     *time.Time
	EvalState                      []byte
	AddedAt                        time.Time
	RemovedAt                      *time.Time
}

type Capability struct {
	ID, SubscriptionID string
	CapabilityType     CapabilityType
	Status             CapabilityStatus
	Disabled           bool
	DisabledReason     DisabledReason
	DisabledMessage    string
	DisabledAt         *time.Time
	CreatedAt          time.Time
}

// ─── Param types ───

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
	ID, FamilyID   string
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
	ID, ArchetypeID       string
	Name                  map[string]string
	Description           map[string]string
	Tagline               map[string]string
	Status                ProductStatus
	ProductType           ProductType
	CurrencyCode          string
	ParentProductID       *string
	ProviderID            string
	ProviderName          string
	Regulator             string
	LicenseNumber         string
	RegulatoryCountry     string
	ShariaCompliant       bool
	AvailabilityMode      AvailabilityMode
	CountryCodes          []string
	Ruleset               []byte
	BaseRulesetIds        []string
	EffectiveFrom         *time.Time
	EffectiveTo           *time.Time
	CreatedBy             string
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
	ArchetypeID, FamilyID, Status, CurrencyCode, CountryCode *string
	ResultLimit, ResultOffset                                 int32
}

type ListAvailableProductsParams struct {
	CountryCode            string
	CustomerType, Family   *string
}

type CreateBaseRulesetParams struct {
	ID, Name, Description string
	Content               []byte
	Version               string
}

type UpdateBaseRulesetParams struct {
	ID, Name, Description string
	Content               []byte
	Version               string
}

type CreateSubscriptionParams struct {
	EntityID      string
	EntityType    EntityType
	ProductID     string
	SigningRule    SigningRule
	RequiredCount int32
}

type ListSubscriptionsParams struct {
	EntityID, CustomerID, Status *string
	ResultLimit, ResultOffset    int32
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
	SubscriptionID, CustomerID string
	Role                       PartyRole
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
