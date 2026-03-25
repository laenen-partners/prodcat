# CLAUDE.md — prodcat

## What is this project?

Prodcat is a product catalogue and ruleset store. It is the **data layer** — stores products and rulesets. Rule evaluation and onboarding orchestration live in the separate [onboarding](https://github.com/laenen-partners/onboarding) package.

## Tech stack

- **Language**: Go 1.26+
- **Persistence**: [entitystore](https://github.com/laenen-partners/entitystore) v0.15.0 on PostgreSQL (pgvector), with transactional preconditions
- **Tags**: [tags](https://github.com/laenen-partners/tags) v0.2.0 for well-known status/disabled-reason/entity tags
- **Proto**: buf with `protoc-gen-entitystore` for match config generation
- **Testing**: testcontainers-go with `pgvector/pgvector:pg17`
- **Tooling**: mise (tool versions), Task (commands), gofumpt (formatting), golangci-lint (linting)

## Project structure

```
prodcat.go              — domain types: Product, Ruleset, enums, ListFilter, Provenance
errors.go               — sentinel errors (ErrNotFound, ErrAlreadyExists, ErrRulesetDisabled, ErrValidation)
store.go                — Store interface (persistence contract with CreateProduct/CreateRuleset)
client.go               — Client: product/ruleset CRUD, ResolveRuleset, Disable/Enable, convenience methods
import.go               — Import: catalogue definition parsing, ImportTracker interface
prodcat_test.go         — Integration tests (testcontainers, 13 tests)

entitystore/store.go    — entitystore-backed Store with preconditions + tags lifecycle
entitystore/tracker.go  — entitystore-backed ImportTracker with MustNotExist precondition

proto/prodcat/v1/       — Proto definitions with entitystore annotations
gen/prodcat/v1/         — Generated Go code (pb.go + *_entitystore.go match configs)
catalog/                — Catalogue definition YAML files (timestamped)
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
client := prodcat.NewClient(store)
client.RegisterProduct(ctx, product, provenance)
client.ResolveRuleset(ctx, productID) // → merged YAML for evalengine
client.AddRuleset(ctx, productID, rulesetID, provenance)
client.Import(ctx, filename, data, tracker) // import catalogue definitions
```

### Layered architecture

Core Go types + Store interface live in the root `prodcat` package. The `entitystore` subpackage provides the persistence implementation. All subpackages share one `go.mod`.

### Provenance

All write operations accept a `Provenance` (SourceURN + Reason) that maps to entitystore's provenance system. This gives a full audit trail: who changed what, when, and why.

```go
prov := prodcat.Provenance{SourceURN: "import:20260318", Reason: "initial load"}
```

### Transactional preconditions (entitystore v0.15.0)

Business rules are enforced atomically inside entitystore transactions via preconditions:
- `MustNotExist` — prevents duplicate products, rulesets, and import records
- `MustExist` + `TagForbidden` — ensures referenced rulesets exist and are not disabled
- No TOCTOU gap between validation and write

### Tags lifecycle (tags v0.2.0)

All entities use well-known tags from the `tags` package for consistent status management:
- Products: `entity:product`, `status:active` or `status:disabled` + `disabled-reason:suspended`
- Rulesets: `entity:ruleset`, `status:active` or `status:disabled` + `disabled-reason:<reason>`
- Preconditions use `TagForbidden: "status:disabled"` for atomic disabled checks

### Entitystore integration
- All persistence goes through entitystore (in the `entitystore/` subpackage)
- Proto definitions carry `entitystore.v1.field` and `entitystore.v1.message` annotations
- The entitystore `Store` uses `matching.BuildAnchors(data, cfg)` and `matching.BuildTokens(data, cfg)` from generated configs
- Domain fields like CurrencyCode, Availability are stored in the proto's `meta` map

### Ruleset composition
- Rulesets are flat with a shared `reads`/`writes` namespace
- Products compose rulesets via `base_ruleset_ids` (stored as `ruleset_ids` in proto)
- `ResolveRuleset` merges all rulesets into one flat YAML document (skips disabled rulesets)
- The merged YAML is consumed by evalengine (in the onboarding package)

### Import system
- Catalogue definition files are YAML with `kind: catalog`. Timestamped filenames
- `ImportTracker` is entitystore-backed with `MustNotExist` precondition for dedup
- Imports contain both rulesets and products. Rulesets are imported first
- Imports use provenance with `SourceURN: "import:<filename>"`
- Rulesets and products are upserted for idempotency

### Soft delete
- Rulesets support soft delete via `DisableRuleset` / `EnableRuleset`
- Disabled rulesets are excluded from `ResolveRuleset` and cannot be linked to products
- The disabled state is stored in proto fields AND entitystore tags (`status:disabled`)

## Common patterns

### Adding a new entity type
1. Create proto in `proto/prodcat/v1/` with entitystore annotations
2. Run `buf generate`
3. Register the generated `*MatchConfig()` in `matchRegistry` in `entitystore/store.go`
4. Add Store interface methods + entitystore Store implementation

### Adding a new product
1. Create a catalogue definition YAML file in `catalog/` with timestamped filename
2. Define rulesets with CEL expressions
3. Reference base rulesets via `base_ruleset_ids`
4. Import with `client.Import()`

## What NOT to do

- Do not add eval logic to prodcat — that belongs in onboarding
- Do not hand-code anchor queries in entitystore Store — use `matching.BuildAnchors()`
- Do not use `json:"id"` for anchor fields — must match proto field name
- Do not put product operational details (fees, rates) in prodcat — core banking owns that
- Do not commit changes to `gen/` without running `buf generate` first
- Do not use raw tag strings — use the `tags` package for well-known tag types

## Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/laenen-partners/entitystore` | Persistence (PostgreSQL, anchors, tokens, matching, preconditions) |
| `github.com/laenen-partners/tags` | Well-known tag types (status, disabled-reason, entity) |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `github.com/google/uuid` | UUID generation |
| `gopkg.in/yaml.v3` | YAML parsing (catalogue definitions, rulesets) |
| `github.com/testcontainers/testcontainers-go` | Integration tests |
| `github.com/stretchr/testify` | Test assertions |
| `google.golang.org/protobuf` | Proto runtime (generated code) |
