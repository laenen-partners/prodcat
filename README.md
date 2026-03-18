# prodcat

An eligibility engine for digital banking. Prodcat determines **who can have which products**, **what they still need to do**, and **whether anything has changed that affects their access**. Product operational details (fees, rates, limits) live in your core banking system — prodcat owns eligibility, subscriptions, and onboarding requirements.

Built on [entitystore](https://github.com/laenen-partners/entitystore) for persistence and designed for [evalengine](https://github.com/laenen-partners/evalengine) (CEL-based rules) integration.

## Architecture

```
                          ┌─────────────────────────────────────────────┐
                          │            Consumers                        │
                          │                                             │
                          │  Onboarding Agent    Product Recommender    │
                          │  (genkit)            (genkit)               │
                          └──────────┬──────────────────┬───────────────┘
                                     │                  │
                          ┌──────────▼──────────────────▼───────────────┐
                          │         Eligibility Engine (prodcat)        │
                          │                                             │
                          │  ┌─────────────────────────────────────┐    │
                          │  │         CheckEligibility            │    │
                          │  │  []productID + EvaluationInput      │    │
                          │  │  → EligibilityReport (per-product)  │    │
                          │  └────────────────┬────────────────────┘    │
                          │                   │                         │
                          │  ┌────────────────▼────────────────────┐    │
                          │  │           Evaluate (per product)    │    │
                          │  │                                     │    │
                          │  │  1. ResolveRuleset                  │    │
                          │  │     base rulesets + product ruleset  │    │
                          │  │     → merged YAML                   │    │
                          │  │                                     │    │
                          │  │  2. Evaluate each rule              │    │
                          │  │     input data → passed/pending/    │    │
                          │  │                  failed per rule    │    │
                          │  │                                     │    │
                          │  │  3. Derive verdict                  │    │
                          │  │     eligible / incomplete /         │    │
                          │  │     not_eligible                    │    │
                          │  └─────────────────────────────────────┘    │
                          │                                             │
                          │  ┌──────────────┐  ┌────────────────────┐   │
                          │  │ Subscribe    │  │ Activate / Disable │   │
                          │  │ AddParty     │  │ Capabilities       │   │
                          │  │ Cancel       │  │ Enable / Disable   │   │
                          │  └──────────────┘  └────────────────────┘   │
                          └──────────────────────┬──────────────────────┘
                                                 │
                          ┌──────────────────────▼──────────────────────┐
                          │              Store (interface)              │
                          │                                             │
                          │  PutProduct / GetProduct / ListProducts     │
                          │  PutRuleset / GetRuleset / ListRulesets     │
                          │  PutSubscription / GetSubscription          │
                          └──────────────────────┬──────────────────────┘
                                                 │
                          ┌──────────────────────▼──────────────────────┐
                          │         entitystore (PostgreSQL)            │
                          │                                             │
                          │  entities table   (JSON data + tags)        │
                          │  entity_anchors   (dedup by product_id,     │
                          │                    ruleset_id, etc.)        │
                          │  entity_relations (product → ruleset)       │
                          └──────────────────────┬──────────────────────┘
                                                 │
                          ┌──────────────────────▼──────────────────────┐
                          │         Seed System                         │
                          │                                             │
                          │  YAML files → ApplySeed → products +        │
                          │  rulesets + SeedTracker (idempotent)        │
                          │                                             │
                          │  seed/2026031801_mal_subscription.yaml      │
                          │  seed/2026031802_casa_current_account.yaml  │
                          └─────────────────────────────────────────────┘
```

## Concepts

### Products: Flat with Tags

Products are flat records with tags for filtering — no rigid hierarchy. Tags replace taxonomy:

| Tag | Purpose |
|---|---|
| `family:casa` | Product family |
| `type:current_account` | Product type |
| `market:uae` | Target market |
| `sharia:true` | Sharia compliance |
| `segment:retail` | Customer segment |

Querying is flexible: "all sharia-compliant CASA products available in UAE for retail customers" is a tag filter.

### Eligibility & the Eval Engine

Eligibility is defined using YAML rulesets with CEL expressions. Rules are stored as data on products and reusable base rulesets:

```yaml
evaluations:
  - name: email_verified
    expression: "input.contacts.exists(c, c.type == 1 && c.primary && c.verified)"
    writes: email_verified
    severity: blocking
    category: contact
    failure_mode: actionable
    resolution: "Please verify your email address"

  - name: age_gate
    expression: "input.age >= 18"
    writes: age_eligible
    severity: blocking
    category: eligibility
    failure_mode: definitive
    resolution: "You must be at least 18 years old"
```

#### Three-State Verdict

Evaluation returns one of three verdicts:

| Verdict | Meaning |
|---|---|
| `eligible` | All blocking requirements pass |
| `incomplete` | No definitive failures, but some requirements need action |
| `not_eligible` | At least one requirement definitively fails — hard reject |

#### Four Failure Modes

Each rule declares what happens when it fails:

| `failure_mode` | Who acts | Example |
|---|---|---|
| `actionable` (default) | Customer self-service | Verify email, accept T&C |
| `input_required` | Customer provides data | Upload passport, provide nationality |
| `manual_review` | Internal human decision | PEPs screening, compliance review |
| `definitive` | Nobody — hard reject | Age < 18, blocked jurisdiction |

#### Ruleset Resolution

At evaluation time, base rulesets + product-specific rulesets are merged:

```
base rulesets (by ID) + product ruleset → merged YAML → eval engine
```

### Subscriptions

A subscription is created when a customer begins onboarding for a product.

| Status | Meaning |
|---|---|
| `incomplete` | Onboarding in progress |
| `active` | All requirements met, product is live |
| `past_due` | Data elements have expired |
| `canceled` | Canceled by customer or operations |

#### Capabilities

Each subscription has independently controllable capabilities:

| Capability | Description |
|---|---|
| `view` | View balances and history |
| `domestic_transfers` | Domestic transfers |
| `international_transfers` | International transfers |
| `card_payments` | Card payments (POS/online) |
| `atm` | ATM withdrawals |
| `receive` | Receive incoming payments |

Both subscriptions and capabilities can be disabled with a reason (Stripe-style).

### Seed System

Product and ruleset definitions are managed as seed files (like database migrations). Each seed has a timestamped filename and is tracked via entitystore to ensure idempotent application:

```
seed/
  2026031801_mal_subscription.yaml
  2026031802_casa_current_account_uae.yaml
```

## Installation

```bash
go get github.com/laenen-partners/prodcat
```

## Usage

### Setup

```go
import (
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/laenen-partners/entitystore"
    "github.com/laenen-partners/prodcat"
)

pool, _ := pgxpool.New(ctx, "postgres://localhost:5432/mydb")
entitystore.Migrate(ctx, pool)

es, _ := entitystore.New(entitystore.WithPgStore(pool))
store := prodcat.NewESStore(es)
tracker := prodcat.NewESSeedTracker(es)
engine := prodcat.NewEngine(store)
```

### Apply Seeds

```go
data, _ := os.ReadFile("seed/2026031801_mal_subscription.yaml")
engine.ApplySeed(ctx, "2026031801_mal_subscription.yaml", data, tracker)
```

### Check Eligibility (multiple products)

```go
report, _ := engine.CheckEligibility(ctx,
    []string{"mal-subscription", "casa-current-account-uae"},
    prodcat.EvaluationInput{
        Contacts: []prodcat.ContactInput{
            {Type: 1, Value: "user@example.com", Primary: true, Verified: true},
            {Type: 2, Value: "+971501234567", Primary: true, Verified: true},
        },
        Agreements: []prodcat.AgreementInput{
            {Type: "general_terms_and_conditions", Accepted: true},
        },
        Age: 25, CountryOfResidence: "AE",
        IDDocuments: []prodcat.IDDocumentInput{
            {DocumentType: "passport", IssuingCountry: "AE", Verified: true},
            {DocumentType: "uae_pass", IssuingCountry: "AE", Verified: true},
        },
    },
)

for _, r := range report.Results {
    fmt.Printf("%s: %s\n", r.ProductID, r.Verdict)
    // mal-subscription: eligible
    // casa-current-account-uae: eligible
}
```

### Evaluate Single Product

```go
result, _ := engine.Evaluate(ctx, "casa-current-account-uae", input)
// result.Verdict: "eligible", "incomplete", or "not_eligible"
// result.Requirements: per-rule status + failure_mode
```

### Subscribe and Activate

```go
sub, _ := engine.Subscribe(ctx, prodcat.SubscribeRequest{
    EntityID:   "customer-123",
    EntityType: prodcat.EntityTypeIndividual,
    ProductID:  "casa-current-account-uae",
    Parties:    []prodcat.PartyInput{
        {CustomerID: "customer-123", Role: prodcat.PartyRolePrimaryHolder},
    },
})

sub, _ = engine.Activate(ctx, sub.ID, "core-banking-ref", []prodcat.CapabilityType{
    prodcat.CapabilityTypeView,
    prodcat.CapabilityTypeDomesticTransfers,
})
```

## Service Interfaces

```go
type EligibilityService interface {
    RegisterProduct(ctx, ProductEligibility) error
    GetProduct(ctx, productID) (ProductEligibility, error)
    ListProducts(ctx, TagFilter) ([]ProductEligibility, error)
    CreateRuleset(ctx, BaseRuleset) (BaseRuleset, error)
    Evaluate(ctx, productID, EvaluationInput) (EvaluationResult, error)
    CheckEligibility(ctx, []productID, EvaluationInput) (EligibilityReport, error)
    ResolveRuleset(ctx, productID) (ResolvedRuleset, error)
}

type SubscriptionService interface {
    Subscribe(ctx, SubscribeRequest) (Subscription, error)
    Activate(ctx, id, externalRef, capabilities) (Subscription, error)
    Cancel(ctx, id, reason) (Subscription, error)
    Disable / Enable (subscription + capability level)
    AddParty / RemoveParty
}
```

See [`service.go`](service.go) for full definitions.

## Persistence

Uses [entitystore](https://github.com/laenen-partners/entitystore) with PostgreSQL (pgvector). Products, rulesets, subscriptions, and seed records are stored as entities with anchor-based dedup lookups. Proto definitions in [`proto/eligibility/v1/`](proto/eligibility/v1/) carry entitystore annotations for matching configuration.

## Testing

Tests use [testcontainers-go](https://github.com/testcontainers/testcontainers-go) with `pgvector/pgvector:pg17`:

```bash
go test ./... -timeout 300s
```

Requires Docker running locally.

## ADRs

- [ADR-0001](docs/adr/0001_product_catalogue_architecture.md) — Original product catalogue (superseded)
- [ADR-0002](docs/adr/0002_pivot_to_eligibility_engine.md) — Pivot to eligibility engine

## License

Private — Laenen Partners.
