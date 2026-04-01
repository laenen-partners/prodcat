# CLAUDE.md — prodcat

## What is this project?

Prodcat is a product catalogue and ruleset store. It is the **data layer** — stores products and rulesets. Rule evaluation and onboarding orchestration live in the separate [onboarding](https://github.com/laenen-partners/onboarding) package.

## Tech stack

- **Language**: Go 1.26+
- **Persistence**: [entitystore](https://github.com/laenen-partners/entitystore) v0.26.0 on PostgreSQL (pgvector), with transactional preconditions
- **Tags**: [tags](https://github.com/laenen-partners/tags) v0.2.0 for well-known status/disabled-reason/entity tags
- **Proto**: buf with `protoc-gen-entitystore` for match config generation
- **Testing**: testcontainers-go with `pgvector/pgvector:pg17`
- **Tooling**: mise (tool versions), Task (commands), gofumpt (formatting), golangci-lint (linting)

## Project structure

```
prodcat.go              — domain types: Product, Ruleset, enums, ListFilter
errors.go               — sentinel errors (ErrNotFound, ErrAlreadyExists, ErrRulesetDisabled, ErrValidation)
client.go               — Client: product/ruleset CRUD, ResolveRuleset, Disable/Enable, Routing, convenience methods
persistence.go          — entitystore persistence: proto conversions, tags, events, BatchWrite operations
import.go               — Import: catalogue definition parsing, OnConflict options
events.go               — business event types (ProductCreated, RulesetUpdated, CatalogImported, etc.)
prodcat_test.go         — Integration tests (testcontainers, 36 tests)

proto/prodcat/v1/       — Proto definitions with entitystore annotations
gen/prodcat/v1/         — Generated Go code (pb.go + *_entitystore.go match configs)
examples/               — Catalogue definition YAML files (timestamped)
docs/adr/               — Architecture Decision Records
```

## Build and test

```bash
task generate    # buf generate → proto Go code + entitystore match configs
task build       # go build ./...
task test:ci     # gotestsum with JUnit XML
task ci          # full pipeline
```

Tests require Docker (testcontainers spins up PostgreSQL).

## Key design decisions

### Prodcat is a store, not an engine

Prodcat stores products and rulesets. It does NOT evaluate rules — that's the onboarding package's job. The main entry point is `prodcat.Client`:

```go
client := prodcat.NewClient(entityStore)
client.RegisterProduct(ctx, product)
client.ResolveRuleset(ctx, productID) // → merged YAML for evalengine
client.AddRuleset(ctx, productID, rulesetID)
client.Import(ctx, filename, data)    // import catalogue definitions
client.SetRoute(ctx, productID, "banking", "provider-abc")
```

### Flat architecture

Everything lives in the root `prodcat` package. No subpackages, no Store interface — `Client` holds an `entitystore.EntityStorer` directly and uses it for all persistence. The `NewClient(es)` constructor takes the entitystore instance.

### Transactional preconditions

Business rules are enforced atomically inside entitystore transactions via preconditions:
- `MustNotExist` — prevents duplicate products and rulesets
- `MustExist` + `TagForbidden` — ensures referenced rulesets exist and are not disabled
- No TOCTOU gap between validation and write

### Tags lifecycle

All entities use well-known tags from the `tags` package for consistent status management:
- Products: `entity:product`, `status:active` or `status:disabled` + `status_reason:<reason>`
- Rulesets: `entity:ruleset`, `status:active` or `status:disabled` + `status_reason:<reason>`
- Preconditions use `TagForbidden: "status:disabled"` for atomic disabled checks

### Entitystore integration
- All persistence goes through entitystore directly on the Client
- Proto definitions carry `entitystore.v1.field` and `entitystore.v1.message` annotations
- Generated `*WriteOp()` and `*MatchConfig()` functions wire anchors, tokens, and data automatically
- Domain fields like CurrencyCode, Availability are stored in the proto's `meta` map
- Routing is a dedicated `map<string, string>` proto field

### Ruleset composition
- Rulesets are flat with a shared `reads`/`writes` namespace
- Products compose rulesets via `base_ruleset_ids` (stored as `ruleset_ids` in proto)
- `ResolveRuleset` merges all rulesets into one flat YAML document (skips disabled rulesets)
- The merged YAML is consumed by evalengine (in the onboarding package)
- Rulesets have a `ContentHash` (SHA-256) computed on every write for change detection

### Import system
- Catalogue definition files are YAML with `kind: catalog`
- All rulesets and products are written atomically in a single `BatchWrite` transaction
- `OnConflictUpdate` (default) upserts; `OnConflictError` fails on duplicates
- Per-entity business events (Created/Updated) and a `CatalogImportedEvent` are emitted
- Graph relations (product→ruleset) are created for `base_ruleset_ids`

### Routing
- Products have a `Routing` map: capability name → provider ID
- Keys: "banking", "cards", "screening", "payments", "notifications"
- Values: provider IDs registered in the adaptor registry
- Convenience methods: `SetRouting`, `SetRoute`, `RemoveRoute`

### Soft delete
- Products and rulesets support soft delete via `DeleteProduct` / `DeleteRuleset`
- Disabled rulesets are excluded from `ResolveRuleset` and cannot be linked to products
- The disabled state is stored in proto fields AND entitystore tags (`status:disabled`)

### Business events
All write operations emit business events stored alongside entities:
- Product: Created, Updated, Disabled, Enabled, Deleted
- Ruleset: Created, Updated, Disabled, Enabled, Deleted
- Linking: RulesetLinkedToProduct, RulesetUnlinkedFromProduct
- Import: CatalogImported (catalog-level summary)

Actor is extracted from context via the `identity` package.

## Common patterns

### Adding a new entity type
1. Create proto in `proto/prodcat/v1/` with entitystore annotations
2. Run `buf generate`
3. Register the generated `*MatchConfig()` in `matchRegistry` in `persistence.go`
4. Add persistence methods on Client in `persistence.go`
5. Add public API methods on Client in `client.go`

### Adding a new product
1. Create a catalogue definition YAML file in `examples/` with timestamped filename
2. Define rulesets with CEL expressions
3. Reference base rulesets via `base_ruleset_ids`
4. Import with `client.Import()`

## What NOT to do

- Do not add eval logic to prodcat — that belongs in onboarding
- Do not use `json:"id"` for anchor fields — must match proto field name
- Do not put product operational details (fees, rates) in prodcat — core banking owns that
- Do not commit changes to `gen/` without running `buf generate` first
- Do not use raw tag strings — use the `tags` package for well-known tag types
- Do not add interfaces or subpackages unless truly needed — keep it flat

## Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/laenen-partners/entitystore` | Persistence (PostgreSQL, anchors, tokens, matching, preconditions, events) |
| `github.com/laenen-partners/tags` | Well-known tag types (status, disabled-reason, entity) |
| `github.com/laenen-partners/identity` | Actor extraction from context for event attribution |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `github.com/google/uuid` | UUID generation |
| `gopkg.in/yaml.v3` | YAML parsing (catalogue definitions, rulesets) |
| `github.com/testcontainers/testcontainers-go` | Integration tests |
| `github.com/stretchr/testify` | Test assertions |
| `google.golang.org/protobuf` | Proto runtime (generated code) |
