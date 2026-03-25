# prodcat

Product catalogue and ruleset store. Prodcat is the **data layer** for products and eligibility rulesets — it stores what you offer and the rules that determine who can access it. Rule evaluation and onboarding orchestration live in the [onboarding](https://github.com/laenen-partners/onboarding) package.

Built on [entitystore](https://github.com/laenen-partners/entitystore) (PostgreSQL + pgvector) with transactional preconditions for referential integrity. Uses [tags](https://github.com/laenen-partners/tags) for consistent lifecycle management across all entities.

## Architecture

```
prodcat/                  Core: domain types, Store interface, Client
prodcat/entitystore/      Entitystore-backed Store + ImportTracker
prodcat/connectrpc/       (Phase 2) Connect-RPC API
prodcat/ui/               (Phase 3) DSX UI
```

```
┌─────────────────────────────────────────────────────────────┐
│  Consumers                                                  │
│                                                             │
│  onboarding (eval + orchestration)    product recommender   │
└──────────┬──────────────────────────────┬───────────────────┘
           │                              │
┌──────────▼──────────────────────────────▼───────────────────┐
│  prodcat.Client                                             │
│                                                             │
│  ┌───────────────┐  ┌──────────────────────────────────┐   │
│  │  Products     │  │  Rulesets                         │   │
│  │  Register     │  │  Create / Disable / Enable       │   │
│  │  Get / List   │  │  AddRuleset / RemoveRuleset      │   │
│  │  Update       │  │  ResolveRuleset → merged YAML    │   │
│  └───────────────┘  └──────────────────────────────────┘   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Import                                              │   │
│  │  Catalogue definitions → products + rulesets         │   │
│  │  ImportTracker (idempotent, precondition-guarded)    │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  Business Rules                                             │
│  ✓ Duplicate prevention (MustNotExist precondition)         │
│  ✓ Referential integrity (MustExist precondition)           │
│  ✓ Disabled guard (TagForbidden precondition)               │
│  ✓ Input validation (required fields)                       │
│  ✓ Import dedup (MustNotExist precondition on tracker)      │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│  entitystore/Store                                          │
│  Proto-driven MatchConfigs + transactional preconditions    │
│  Well-known tags: status, disabled-reason, entity           │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│  entitystore (PostgreSQL + pgvector)                        │
└─────────────────────────────────────────────────────────────┘
```

## Installation

```bash
go get github.com/laenen-partners/prodcat
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/laenen-partners/entitystore"
    "github.com/laenen-partners/prodcat"
    esstore "github.com/laenen-partners/prodcat/entitystore"
)

func main() {
    ctx := context.Background()

    // 1. Set up persistence
    pool, _ := pgxpool.New(ctx, "postgres://localhost:5432/mydb?sslmode=disable")
    entitystore.Migrate(ctx, pool)
    es, _ := entitystore.New(entitystore.WithPgStore(pool))
    defer es.Close()

    // 2. Create client
    store := esstore.NewStore(es)
    tracker := esstore.NewImportTracker(es)
    client := prodcat.NewClient(store)

    // 3. Import catalogue definitions
    data, _ := os.ReadFile("catalog/2026031801_platform_subscription.yaml")
    client.Import(ctx, "2026031801_platform_subscription.yaml", data, tracker)

    // 4. Resolve rulesets for evaluation
    resolved, _ := client.ResolveRuleset(ctx, "platform-subscription")
    fmt.Println(string(resolved.Merged))
}
```

## Concepts

### Products

Products are flat records identified by a `ProductID`. No rigid hierarchy — use **tags** for flexible classification and filtering.

#### Fields

| Field | Type | Description |
|-------|------|-------------|
| `ProductID` | `string` | Unique identifier (required) |
| `Name` | `string` | Display name (required) |
| `Description` | `string` | Human-readable description |
| `Tags` | `[]string` | User-defined tags for filtering (`family:casa`, `type:savings`) |
| `Status` | `ProductStatus` | `active` (default) or `suspended` |
| `CurrencyCode` | `string` | ISO 4217 currency code |
| `ParentProductID` | `string` | Optional parent product for supplementary products (e.g. debit card → current account) |
| `Availability` | `GeoAvailability` | Geographic availability (see below) |
| `BaseRulesetIDs` | `[]string` | Referenced rulesets for eligibility evaluation |
| `Ruleset` | `[]byte` | Inline product-specific ruleset YAML |

#### Geographic Availability

Products define where they're available via `GeoAvailability`:

```go
// Available only in specific countries
prodcat.GeoAvailability{
    Mode:         prodcat.AvailabilityModeSpecificCountries,
    CountryCodes: []string{"US", "GB"},
}

// Available globally
prodcat.GeoAvailability{
    Mode: prodcat.AvailabilityModeGlobal,
}

// Available globally except specific countries
prodcat.GeoAvailability{
    Mode:         prodcat.AvailabilityModeGlobalExcept,
    CountryCodes: []string{"KP", "IR"},
}
```

#### Registering a product

```go
prov := prodcat.Provenance{SourceURN: "user:admin-1", Reason: "initial setup"}

product, err := client.RegisterProduct(ctx, prodcat.Product{
    ProductID:    "casa-current-account",
    Name:         "Current Account",
    Tags:         []string{"family:casa", "type:current_account", "segment:retail"},
    CurrencyCode: "USD",
    Availability: prodcat.GeoAvailability{
        Mode: prodcat.AvailabilityModeGlobal,
    },
    BaseRulesetIDs: []string{"base-platform-access", "base-retail-kyc"},
}, prov)
```

`RegisterProduct` enforces:
- **Required fields** — `ProductID` and `Name` must be non-empty
- **Uniqueness** — fails with `ErrAlreadyExists` if the ProductID is already taken (transactional `MustNotExist` precondition)
- **Referential integrity** — all rulesets in `BaseRulesetIDs` must exist and not be disabled (transactional `MustExist` + `TagForbidden` preconditions)

#### Retrieving products

```go
// Get a single product by ID
product, err := client.GetProduct(ctx, "casa-current-account")

// List all products
products, err := client.ListProducts(ctx, prodcat.ListFilter{})

// Filter by tags — all tags must match (AND)
products, err := client.ListProducts(ctx, prodcat.ListFilter{
    Tags: []string{"family:casa", "segment:retail"},
})

// Filter by status
products, err := client.ListProducts(ctx, prodcat.ListFilter{
    Status: prodcat.ProductStatusActive,
})
```

#### Updating a product

```go
product.Description = "Updated description"
product, err := client.UpdateProduct(ctx, *product, prov)
```

`UpdateProduct` also enforces referential integrity — if you change `BaseRulesetIDs`, all referenced rulesets must exist and not be disabled.

#### Entity tags

Products are automatically tagged with well-known [tags](https://github.com/laenen-partners/tags) in entitystore:

| Tag | Value | When |
|-----|-------|------|
| `entity:product` | Always | Entity type identifier |
| `status:active` | `ProductStatusActive` | Product is active |
| `status:disabled` | `ProductStatusSuspended` | Product is suspended |
| `disabled-reason:suspended` | `ProductStatusSuspended` | Reason for disabled state |

User-defined tags (e.g. `family:casa`) are stored alongside system tags but filtered out when reading the `Tags` field back.

---

### Rulesets

Eligibility rulesets define the rules that determine whether a customer can access a product. Rules are stored as YAML with CEL expressions. Products reference rulesets by ID to compose their eligibility criteria.

#### Fields

| Field | Type | Description |
|-------|------|-------------|
| `ID` | `string` | Unique identifier (auto-generated UUID if empty) |
| `Name` | `string` | Display name (required) |
| `Description` | `string` | Human-readable description |
| `Content` | `[]byte` | YAML ruleset content (required) |
| `Version` | `string` | Semantic version |
| `Disabled` | `bool` | Whether the ruleset is soft-deleted |
| `DisabledReason` | `DisabledReason` | Why the ruleset was disabled |

#### Creating a ruleset

```go
ruleset, err := client.CreateRuleset(ctx, prodcat.Ruleset{
    ID:   "base-age-gate-18",
    Name: "Age Gate (18+)",
    Content: []byte(`evaluations:
  - name: age_check
    expression: "primary_holder.age >= 18"
    writes: age_verified
    failure_mode: definitive
    category: demographics
    resolution: "You must be at least 18 years old"`),
    Version: "1.0.0",
}, prov)
```

`CreateRuleset` enforces:
- **Required fields** — `Name` and `Content` must be non-empty
- **Content validation** — delegates to [evalengine](https://github.com/laenen-partners/evalengine) `ValidateConfig()` to check required evaluation fields (`name`, `expression`, `writes`), duplicate writes, `cache_ttl` format, and precondition expressions
- **Uniqueness** — fails with `ErrAlreadyExists` if the ID is already taken (transactional `MustNotExist` precondition)

#### Retrieving rulesets

```go
// Get a single ruleset by ID
ruleset, err := client.GetRuleset(ctx, "base-age-gate-18")

// List all rulesets (including disabled)
rulesets, err := client.ListRulesets(ctx)
```

#### Linking rulesets to products

```go
// Add a ruleset to a product (idempotent — no-op if already linked)
product, err := client.AddRuleset(ctx, "casa-current-account", "base-age-gate-18", prov)

// Remove a ruleset from a product
product, err = client.RemoveRuleset(ctx, "casa-current-account", "base-age-gate-18", prov)

// Set inline ruleset content directly on a product
product, err = client.SetProductRuleset(ctx, "casa-current-account", yamlContent, prov)
```

`AddRuleset` validates the ruleset exists and is not disabled — at the Client level (fast feedback) and at the Store level (transactional precondition, no TOCTOU gap).

#### Resolving rulesets

`ResolveRuleset` merges all base rulesets and the product's inline ruleset into a single YAML document for the [evalengine](https://github.com/laenen-partners/evalengine):

```go
resolved, err := client.ResolveRuleset(ctx, "casa-current-account")

// resolved.ProductID  — the product that was resolved
// resolved.Merged     — merged YAML ready for evalengine
// resolved.Layers     — which rulesets contributed (for audit trail)

for _, layer := range resolved.Layers {
    fmt.Printf("  %s: %s\n", layer.Source, layer.SourceID)
    // "base: base-platform-access"
    // "base: base-age-gate-18"
    // "product: casa-current-account"
}
```

The merge order is: base rulesets (in the order listed in `BaseRulesetIDs`) then the product's inline ruleset. Disabled rulesets are **skipped** with a log warning.

#### Ruleset YAML schema

```yaml
evaluations:
  - name: email_verified                              # unique name within the ruleset
    description: "Verify the customer's email"         # optional human-readable description
    expression: "primary_holder.email.verified"        # CEL expression
    reads:                                             # fields this rule reads (for dependency graph)
      - phone_verified
    writes: email_verified                             # field this rule writes to
    failure_mode: actionable                           # actionable | input_required | manual_review | definitive
    severity: blocking                                 # blocking (default)
    category: contact                                  # grouping category
    resolution: "Please verify your email address"     # customer-facing resolution message
    resolution_workflow: "email_verification"          # optional workflow to trigger
    cache_ttl: "24h"                                   # optional cache duration
    preconditions:                                     # optional guards (evalengine v0.5.0)
      - expression: "has(input.primary_holder)"
        description: "Primary holder data must be present"
```

---

### Soft Delete (Disable/Enable)

Rulesets support soft delete. A disabled ruleset remains in the store for audit purposes but is excluded from resolution and cannot be linked to new products.

```go
// Soft-delete a ruleset
rs, err := client.DisableRuleset(ctx, "base-age-gate-18", prodcat.DisabledReasonDeleted, prov)

// Re-enable it
rs, err = client.EnableRuleset(ctx, "base-age-gate-18", prov)
```

| Reason | Constant | Use case |
|--------|----------|----------|
| `deleted` | `DisabledReasonDeleted` | Ruleset is no longer needed |
| `superseded` | `DisabledReasonSuperseded` | Replaced by a newer version |
| `regulatory_hold` | `DisabledReasonRegulatoryHold` | Regulatory review pending |
| `operations` | `DisabledReasonOperations` | Operational pause |

Disabled rulesets are tagged with `status:disabled` + `disabled-reason:<reason>` using the [tags](https://github.com/laenen-partners/tags) package. The `TagForbidden` precondition on `status:disabled` prevents linking disabled rulesets to products atomically inside the entitystore transaction.

---

### Import

Other services can import catalogue definitions — YAML files containing products and rulesets. This is the public API for provisioning the product catalogue.

```go
data, _ := os.ReadFile("catalog/2026031801_platform_subscription.yaml")
err := client.Import(ctx, "2026031801_platform_subscription.yaml", data, tracker)
```

#### Catalogue definition files

Stored in `catalog/` with timestamped filenames:

```
catalog/
  2026031801_platform_subscription.yaml      — base platform access
  2026031805_casa_current_account.yaml       — current account
  2026031803_base_retail_kyc.yaml            — retail KYC rulesets
  2026031804_base_sme_kyc.yaml               — SME KYC rulesets
```

A catalogue definition defines rulesets and products together. Rulesets are imported first (since products reference them):

```yaml
kind: catalog
version: "1.0"

rulesets:
  - id: base-platform-access
    name: Base Platform Access
    description: Core requirements — email, phone, T&C
    version: "0.0.1"
    evaluations:
      - name: email_verified
        expression: "primary_holder.email.verified"
        writes: email_verified
        severity: blocking
        category: contact
        resolution: "Please verify your email address"

products:
  - product_id: platform-subscription
    name: Platform Subscription
    description: Base platform access — required for all users
    tags: ["type:platform_access", "segment:all"]
    status: active
    availability:
      mode: global
    base_ruleset_ids:
      - base-platform-access
```

#### Idempotency

Imports are idempotent through two mechanisms:

1. **ImportTracker** — records which files have been imported, using a `MustNotExist` precondition to atomically prevent duplicate tracking records. Already-imported files are skipped entirely.
2. **Upsert semantics** — rulesets and products are upserted (create-or-update), so re-importing the same file without a tracker is also safe.

#### Import without a tracker

The tracker is optional. Without it, every call to `Import` re-applies the entire file:

```go
// Without tracker — always applies
client.Import(ctx, "my-catalog.yaml", data, nil)
```

#### Import validation

`Import` validates the entire catalogue definition before applying any changes. Invalid files are rejected without side effects:

```go
err := client.Import(ctx, "bad-catalog.yaml", data, tracker)
// errors.Is(err, prodcat.ErrValidation) == true
```

---

### Validation

Prodcat validates both catalogue definitions and ruleset content. Validation is automatic — `CreateRuleset` and `Import` validate before persisting — but both functions are also available for standalone use.

#### ValidateCatalogDefinition

Structural validation for a full catalogue YAML file:

```go
var def prodcat.CatalogDefinition
yaml.Unmarshal(data, &def)
err := prodcat.ValidateCatalogDefinition(&def)
```

Checks:
- `kind` must be `"catalog"`
- At least one ruleset or product
- Products: required `product_id`, `name`, `status`; status must be `active` or `suspended`; no duplicate IDs
- Rulesets: required `id`, `name`, `evaluations`; no duplicate IDs
- Ruleset content validated against evalengine schema (see below)

#### ValidateRulesetContent

Validates ruleset YAML content against the [evalengine](https://github.com/laenen-partners/evalengine) v0.5.0 schema:

```go
err := prodcat.ValidateRulesetContent(content)
```

Delegates to `evalengine.ValidateConfig()` which checks:
- Evaluations list is not empty
- Each evaluation has `name`, `expression`, and `writes`
- No duplicate `writes` fields across evaluations
- `cache_ttl` is a valid Go duration (e.g. `"10m"`, `"1h"`)
- Precondition expressions are non-empty strings

---

### Provenance

Every write operation requires a `Provenance` — who made the change and why. This maps to entitystore's provenance system for a full audit trail queryable via `entitystore.GetProvenanceForEntity()`.

```go
prov := prodcat.Provenance{
    SourceURN: "user:admin-123",     // who/what made the change
    Reason:    "quarterly review",   // optional human-readable reason
}
```

| Source | SourceURN format | Set by |
|--------|-----------------|--------|
| Admin user | `user:admin-123` | Caller |
| API service | `api:onboarding` | Caller |
| Catalogue import | `import:<filename>` | `Import()` automatically |

---

### Client Options

```go
// Default logger (slog.Default)
client := prodcat.NewClient(store)

// Custom logger
client := prodcat.NewClient(store, prodcat.WithLogger(myLogger))
```

The logger is used by `ResolveRuleset` to warn when disabled rulesets are skipped.

---

### Business Rules

| Rule | Enforcement | Error |
|------|-------------|-------|
| Product ID required | Client validation | `ErrValidation` |
| Product name required | Client validation | `ErrValidation` |
| Ruleset name required | Client validation | `ErrValidation` |
| Ruleset content required | Client validation | `ErrValidation` |
| Duplicate product ID | `MustNotExist` precondition | `ErrAlreadyExists` |
| Duplicate ruleset ID | `MustNotExist` precondition | `ErrAlreadyExists` |
| Duplicate import record | `MustNotExist` precondition | `ErrAlreadyExists` |
| Link nonexistent ruleset | `MustExist` precondition | `ErrNotFound` |
| Link disabled ruleset | `TagForbidden` precondition | `ErrRulesetDisabled` |
| Get nonexistent entity | Anchor lookup | `ErrNotFound` |

Preconditions run **inside the entitystore transaction** — no TOCTOU gap between validation and write.

---

### Error Handling

All sentinel errors support `errors.Is`:

```go
product, err := client.RegisterProduct(ctx, product, prov)

switch {
case errors.Is(err, prodcat.ErrAlreadyExists):
    // product ID already taken
case errors.Is(err, prodcat.ErrNotFound):
    // a referenced ruleset doesn't exist
case errors.Is(err, prodcat.ErrRulesetDisabled):
    // a referenced ruleset is disabled
case errors.Is(err, prodcat.ErrValidation):
    // missing required fields (product_id, name, etc.)
case errors.Is(err, prodcat.ErrNoStore):
    // client was created without a store
}
```

---

## Client API Reference

### Products

| Method | Description |
|--------|-------------|
| `RegisterProduct(ctx, Product, Provenance) (*Product, error)` | Create a new product (fails on duplicate) |
| `GetProduct(ctx, productID) (*Product, error)` | Get a product by ID |
| `ListProducts(ctx, ListFilter) ([]Product, error)` | List products with optional tag/status filter |
| `UpdateProduct(ctx, Product, Provenance) (*Product, error)` | Update an existing product |

### Rulesets

| Method | Description |
|--------|-------------|
| `CreateRuleset(ctx, Ruleset, Provenance) (*Ruleset, error)` | Create a new ruleset (fails on duplicate) |
| `GetRuleset(ctx, id) (*Ruleset, error)` | Get a ruleset by ID |
| `ListRulesets(ctx) ([]Ruleset, error)` | List all rulesets |
| `DisableRuleset(ctx, id, DisabledReason, Provenance) (*Ruleset, error)` | Soft-delete a ruleset |
| `EnableRuleset(ctx, id, Provenance) (*Ruleset, error)` | Re-enable a disabled ruleset |

### Ruleset management on products

| Method | Description |
|--------|-------------|
| `AddRuleset(ctx, productID, rulesetID, Provenance) (*Product, error)` | Link a ruleset to a product (idempotent) |
| `RemoveRuleset(ctx, productID, rulesetID, Provenance) (*Product, error)` | Unlink a ruleset from a product |
| `SetProductRuleset(ctx, productID, content, Provenance) (*Product, error)` | Set inline ruleset content |
| `ResolveRuleset(ctx, productID) (*ResolvedRuleset, error)` | Merge all rulesets into one YAML document |

### Import

| Method | Description |
|--------|-------------|
| `Import(ctx, filename, data, ImportTracker) error` | Import a catalogue definition (validates, then upserts) |

### Validation (standalone)

| Function | Description |
|----------|-------------|
| `ValidateCatalogDefinition(def *CatalogDefinition) error` | Validate catalogue YAML structure + evalengine schema |
| `ValidateRulesetContent(content []byte) error` | Validate ruleset YAML against evalengine v0.5.0 |

---

## Store Interface

For custom persistence implementations:

```go
type Store interface {
    CreateProduct(ctx context.Context, p Product, prov Provenance) error
    PutProduct(ctx context.Context, p Product, prov Provenance) error
    GetProduct(ctx context.Context, productID string) (*Product, error)
    ListProducts(ctx context.Context, filter ListFilter) ([]Product, error)

    CreateRuleset(ctx context.Context, r Ruleset, prov Provenance) error
    PutRuleset(ctx context.Context, r Ruleset, prov Provenance) error
    GetRuleset(ctx context.Context, id string) (*Ruleset, error)
    ListRulesets(ctx context.Context) ([]Ruleset, error)
}
```

`CreateProduct`/`CreateRuleset` fail with `ErrAlreadyExists` on duplicates. `PutProduct`/`PutRuleset` are upserts used by the import system. When a product references rulesets, `PutProduct` verifies each one exists and is not disabled.

The entitystore-backed implementation (`prodcat/entitystore`) uses transactional preconditions for all business rules and the [tags](https://github.com/laenen-partners/tags) package for lifecycle management.

---

## Code Generation

Proto definitions in `proto/prodcat/v1/` carry entitystore annotations. Running `buf generate` produces:

- `gen/prodcat/v1/*.pb.go` — proto Go code
- `gen/prodcat/v1/*_entitystore.go` — `MatchConfig()` and `ExtractionSchema()` per entity type

```bash
buf generate  # or: task generate
```

## Development

Tool versions managed by [mise](https://mise.jdx.dev/). Commands via [Task](https://taskfile.dev/):

```bash
task generate    # buf generate
task build       # go build ./...
task format      # gofumpt
task lint        # golangci-lint
task test:ci     # gotestsum with JUnit output
task ci          # full pipeline
```

Tests use testcontainers with `pgvector/pgvector:pg17`. Requires Docker.

## Related Packages

| Package | Purpose |
|---------|---------|
| [onboarding](https://github.com/laenen-partners/onboarding) | Eval engine + onboarding orchestration |
| [evalengine](https://github.com/laenen-partners/evalengine) | CEL expression compilation and execution |
| [inbox](https://github.com/laenen-partners/inbox) | Task-driven inbox for human/AI resolution |
| [entitystore](https://github.com/laenen-partners/entitystore) | PostgreSQL persistence with anchors, matching, and preconditions |
| [tags](https://github.com/laenen-partners/tags) | Well-known tag types for status lifecycle |

## ADRs

- [ADR-0001](docs/adr/0001_product_catalogue_architecture.md) — Original product catalogue (superseded)
- [ADR-0002](docs/adr/0002_pivot_to_eligibility_engine.md) — Pivot to eligibility engine
- [ADR-0003](docs/adr/0003_entitystore_batch_preconditions.md) — Entitystore batch write preconditions

## License

Private — Laenen Partners.
