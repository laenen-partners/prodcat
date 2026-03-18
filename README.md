# prodcat

A lightweight product catalogue library for digital banking. Prodcat manages **what products exist**, **who is eligible**, and **what customers need to complete to subscribe**. Product operational details (fees, rates, limits) live in your core banking system — prodcat owns identity, eligibility, and onboarding requirements.

## Concepts

### Product Hierarchy

Products are organized in a three-tier hierarchy, where each level can define eligibility rules that gate everything below it:

```
Family                    e.g., "CASA", "Cards", "Investments"
  └── Archetype           e.g., "Current Account", "Savings Account"
        └── Product       e.g., "AED Current Account", "USD Current Account"
```

**Families** define broad eligibility gates. For example, the Investments family might require `age >= 18`, blocking all investment products for minors.

**Archetypes** group related products and add archetype-level rules. For example, all Current Account products might require UAE residency.

**Products** are the subscribable units. Each product has its own eligibility ruleset, geographic availability, compliance metadata, and legal agreements.

### Eligibility & the Eval Engine

Eligibility is defined using [evalengine](https://github.com/laenen-partners/evalengine) — a CEL-based rules engine. Rules are written in YAML and stored as data on each level of the hierarchy:

```yaml
evaluations:
  - name: email_verified
    expression: >
      input.contacts.exists(c, c.type == 2 && c.primary == true && c.verified == true)
    reads: [input.contacts]
    writes: email_verified
    resolution_workflow: EmailVerificationWorkflow
    resolution: "Verify your email address"
    severity: blocking
    category: contact

  - name: mpin_created
    expression: >
      phone_verified && input.mpin_created == true
    reads: [phone_verified, input.mpin_created]
    writes: mpin_created
    resolution_workflow: MpinCreationWorkflow
    resolution: "Create your mobile PIN"
    severity: blocking
    category: security
```

The engine automatically resolves dependencies between evaluations and executes them in topological order. Each evaluation writes a boolean that downstream evaluations can read.

#### Base Rulesets

Common requirements can be extracted into **base rulesets** — reusable rule sets referenced from any level of the hierarchy. This prevents duplication:

| Base Ruleset | Contains |
|---|---|
| `base-contact-verification` | email_verified, phone_verified |
| `base-security` | mpin_created |
| `base-idv` | id_document_scanned, liveness_check_passed, age_eligible |
| `base-kyc` | residential_address, employment, tax_residency, pep_declaration |

#### Ruleset Resolution

At evaluation time, rulesets are merged across the full hierarchy:

```
base rulesets (family) + family ruleset
  → base rulesets (archetype) + archetype ruleset
    → base rulesets (product) + product ruleset
      → single merged YAML → eval engine
```

### Subscriptions

A subscription is created when a customer (or business) begins onboarding for a product. Subscriptions track eligibility state and provide granular control over capabilities.

#### Status

| Status | Meaning |
|---|---|
| `incomplete` | Onboarding in progress, not all requirements met |
| `active` | All requirements met, product is live |
| `past_due` | One or more data elements have expired |
| `canceled` | Canceled by customer or operations |

#### Disabled State (Stripe-style)

Both subscriptions and individual capabilities can be independently disabled with a reason:

```go
// Disable the whole subscription
store.Disable(ctx, subID, prodcat.DisabledReasonRegulatoryHold, "AML review pending")

// Disable a specific capability
store.DisableCapability(ctx, subID, capID, prodcat.DisabledReasonExpiredData, "Emirates ID expired")
```

Disabled reasons: `requirements_not_met`, `expired_data`, `failed_evaluation`, `regulatory_hold`, `fraud_suspicion`, `customer_requested`, `operations`, `parent_disabled`, `party_incomplete`, `party_removed`.

#### Capabilities

Each subscription has capabilities that can be independently toggled:

| Capability | Description |
|---|---|
| `view` | View balances and history |
| `domestic_transfers` | Domestic transfers |
| `international_transfers` | International transfers |
| `card_payments` | Card payments (POS/online) |
| `atm` | ATM withdrawals |
| `receive` | Receive incoming payments |
| `bill_payments` | Bill payments |
| `fx` | Currency conversion |
| `standing_orders` | Recurring transfers |

This enables graceful degradation — an expired ID disables transfers but keeps balance viewing active.

### Entities and Parties

Subscriptions support three account structures:

| Structure | Entity Type | Parties |
|---|---|---|
| Retail (single) | `individual` | 1 primary holder |
| Joint account | `individual` | 1 primary + N joint holders |
| SME / Corporate | `business` | Authorized signatories, directors, UBOs |

Each party has their own eval engine state — one party's expired data doesn't automatically block another party.

#### Signing Authority

Defines how many parties must authorize an action:

| Rule | Description |
|---|---|
| `any_one` | Any single party (default for retail) |
| `any_n` | Any N parties (e.g., "any two" for joint accounts) |
| `all` | All parties must authorize |

Signing authority can be overridden per capability type.

## Installation

```bash
go get github.com/laenen-partners/prodcat
```

## Usage

### Setup

```go
import (
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/laenen-partners/prodcat"
    "github.com/laenen-partners/prodcat/db"
)

pool, err := pgxpool.New(ctx, "postgres://localhost:5432/mydb")
if err != nil {
    log.Fatal(err)
}

store, err := db.New(db.WithPool(pool))
if err != nil {
    log.Fatal(err)
}

// Run migrations
if err := store.Migrate(ctx); err != nil {
    log.Fatal(err)
}
```

### Define the Product Hierarchy

```go
// Create a product family
_, err := store.CreateFamily(ctx, prodcat.FamilyDefinition{
    ID:     "casa",
    Family: prodcat.ProductFamilyCASA,
    Name:   map[string]string{"en": "CASA", "ar": "حسابات جارية"},
    Description: map[string]string{"en": "Current and Savings Accounts"},
    BaseRulesetIDs: []string{"base-contact-verification", "base-security"},
})

// Create an archetype
_, err := store.CreateArchetype(ctx, prodcat.Archetype{
    ID:       "current-account",
    FamilyID: "casa",
    Name:     map[string]string{"en": "Current Account"},
    BaseRulesetIDs: []string{"base-idv", "base-kyc"},
    Ruleset: []byte(`evaluations:
  - name: uae_residency
    expression: "input.id_documents.exists(d, d.type == 'emirates_id')"
    severity: blocking
    category: eligibility`),
})

// Create a product
_, err := store.CreateProduct(ctx, prodcat.Product{
    ID:          "aed-ca",
    ArchetypeID: "current-account",
    Name:        map[string]string{"en": "AED Current Account"},
    Status:      prodcat.ProductStatusActive,
    ProductType: prodcat.ProductTypePrimary,
    CurrencyCode: "AED",
    Provider: prodcat.RegulatoryProvider{
        ProviderID: "mal", Name: "Mal", Regulator: "CBUAE", RegulatoryCountry: "AE",
    },
    Compliance: prodcat.ComplianceConfig{ShariaCompliant: true},
    Eligibility: prodcat.EligibilityConfig{
        Geographic: prodcat.GeographicAvailability{
            Mode: prodcat.AvailabilityModeSpecificCountries,
            CountryCodes: []string{"AE"},
        },
        Ruleset: []byte(`evaluations:
  - name: aed_ca_tc_accepted
    expression: "input.agreements.exists(a, a.type == 'aed_ca_tc' && a.accepted)"
    severity: blocking
    category: legal`),
    },
    CreatedBy: "operations",
})
```

### Subscribe a Customer

```go
// Retail (single holder)
sub, err := store.Subscribe(ctx, prodcat.SubscribeRequest{
    EntityID:   "customer-123",
    EntityType: prodcat.EntityTypeIndividual,
    ProductID:  "aed-ca",
    InitialParties: []prodcat.PartyInput{
        {CustomerID: "customer-123", Role: prodcat.PartyRolePrimaryHolder},
    },
    SigningAuthority: prodcat.SigningAuthority{
        Rule: prodcat.SigningRuleAnyOne, RequiredCount: 1,
    },
})
// sub.Status == prodcat.SubscriptionStatusIncomplete

// Joint account
sub, err := store.Subscribe(ctx, prodcat.SubscribeRequest{
    EntityID:   "customer-123",
    EntityType: prodcat.EntityTypeIndividual,
    ProductID:  "aed-ca",
    InitialParties: []prodcat.PartyInput{
        {CustomerID: "customer-123", Role: prodcat.PartyRolePrimaryHolder},
        {CustomerID: "customer-456", Role: prodcat.PartyRoleJointHolder},
    },
    SigningAuthority: prodcat.SigningAuthority{
        Rule: prodcat.SigningRuleAnyN, RequiredCount: 2,
    },
})

// SME
sub, err := store.Subscribe(ctx, prodcat.SubscribeRequest{
    EntityID:   "business-xyz-llc",
    EntityType: prodcat.EntityTypeBusiness,
    ProductID:  "sme-aed-ca",
    InitialParties: []prodcat.PartyInput{
        {CustomerID: "director-1", Role: prodcat.PartyRoleAuthorizedSignatory},
        {CustomerID: "ubo-1", Role: prodcat.PartyRoleUBO},
    },
    SigningAuthority: prodcat.SigningAuthority{
        Rule: prodcat.SigningRuleAnyN, RequiredCount: 2,
    },
})
```

### Activate and Manage

```go
// Activate after all requirements are met
sub, err := store.Activate(ctx, sub.ID, "core-banking-ref-123", []prodcat.CapabilityType{
    prodcat.CapabilityTypeView,
    prodcat.CapabilityTypeDomesticTransfers,
    prodcat.CapabilityTypeInternationalTransfers,
    prodcat.CapabilityTypeReceive,
})

// Disable a capability (e.g., ID expired)
sub, err := store.DisableCapability(ctx, sub.ID, capID,
    prodcat.DisabledReasonExpiredData, "Emirates ID expired")

// Re-enable after refresh
sub, err := store.EnableCapability(ctx, sub.ID, capID)

// Add a party to an existing subscription
sub, err := store.AddParty(ctx, sub.ID, "customer-789", prodcat.PartyRoleJointHolder)

// Update signing authority
sub, err := store.UpdateSigningAuthority(ctx, sub.ID, prodcat.SigningAuthority{
    Rule: prodcat.SigningRuleAll, RequiredCount: 2,
})
```

## Service Interfaces

The library exposes two interfaces that the PostgreSQL store implements:

```go
type CatalogService interface {
    // Families, Archetypes, Products — full CRUD
    // Base rulesets — create, read, update
    // Product discovery — geographic + segment filtering
    // Ruleset resolution — merge all layers
}

type SubscriptionService interface {
    // Subscribe, Activate, Cancel
    // Disable / Enable (subscription + capability level)
    // Evaluate / EvaluateParty / CheckAccess
    // AddParty / RemoveParty
    // UpdateSigningAuthority
}
```

See [`service.go`](service.go) for the full interface definitions.

## Database

The PostgreSQL schema uses native enums for type safety:

```sql
CREATE TYPE product_family AS ENUM ('casa', 'lending', 'cards', ...);
CREATE TYPE subscription_status AS ENUM ('incomplete', 'active', 'past_due', 'canceled');
CREATE TYPE party_role AS ENUM ('primary_holder', 'joint_holder', 'authorized_signatory', ...);
CREATE TYPE capability_type AS ENUM ('view', 'domestic_transfers', ...);
CREATE TYPE disabled_reason AS ENUM ('expired_data', 'regulatory_hold', ...);
```

Migrations are embedded in the binary and run via [laenen-partners/migrate](https://github.com/laenen-partners/migrate):

```go
store.Migrate(ctx) // applies all pending migrations
```

## Testing

Tests use [testcontainers-go](https://github.com/testcontainers/testcontainers-go) with PostgreSQL 16:

```bash
go test ./db/ -v -timeout 300s
```

Requires Docker running locally.

## Architecture

For the full architecture specification, see [ADR-0001](docs/adr/0001_product_catalogue_architecture.md).

## License

Private — Laenen Partners.
