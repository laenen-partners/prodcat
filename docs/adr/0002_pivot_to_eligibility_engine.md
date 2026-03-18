# ADR-0002: Pivot to Eligibility Engine with AI-Native Architecture

**Date**: 2026-03-18
**Status**: Accepted
**Authors**: Pascal Laenen
**Supersedes**: ADR-0001 (partially — the eval engine approach and subscription model survive; the product catalogue hierarchy is retired)

---

## Context

After building the initial product catalogue (ADR-0001), we stepped back and evaluated the approach through an AI-native lens. The core question: **if we were starting today, knowing what AI can do in 2026, would we build a product catalogue library — or something else?**

### What ADR-0001 Got Right

- **Deterministic eligibility via the eval engine.** Regulators require auditable, reproducible decisions. CEL expressions with dependency graphs and resolution workflows are the correct execution model. This doesn't change.
- **Stripe-style subscriptions with granular capabilities.** The disabled state model with per-capability control and party-level evaluation is sound.
- **Separation from core banking.** Product operational details (fees, rates, limits) belong in SaaScada. The eligibility layer should not duplicate them.

### What ADR-0001 Got Wrong

**1. We built plumbing, not intelligence.**

The product catalogue was a CRUD store with enum types. The three-tier hierarchy (Family → Archetype → Product) was premature structure that would become a constraint at scale across multiple markets. The competitive advantage of "Revolut for Islamic banking" is not in how we store product definitions — it's in:

- How eligibility rules are **authored** (from regulatory documents, not hand-written YAML)
- How onboarding is **orchestrated** (conversational AI agent, not rigid modules)
- How products are **recommended** (personalized, not a static list)

**2. The hierarchy was solving the wrong problem.**

Family → Archetype → Product assumes we know the taxonomy upfront and that it's stable. In practice:

- New markets bring different product structures
- Regulatory changes create new product types overnight
- The hierarchy becomes a migration problem, not a feature

Tags, labels, and semantic search are more flexible than rigid hierarchies.

**3. Rule authoring was manual.**

The design assumed humans write YAML by hand. In 2026, the standard approach is:

```
Regulatory PDF / compliance policy
    → AI extracts requirements
    → Generates CEL rule YAML
    → Human reviews and approves
    → Eval engine executes deterministically
```

The eval engine is the **execution layer**. The AI is the **authoring layer**. We built the execution layer and skipped the authoring layer.

**4. The product catalogue duplicated core banking.**

SaaScada already has product configuration. The value we add is not "what products exist" — it's "who can have them and what do they need to do." That's an eligibility engine, not a product catalogue.

---

## Decision

### Rename and refocus: from Product Catalogue to Eligibility Engine

The system is renamed from **prodcat** (product catalogue) to an **eligibility engine** that answers three questions:

1. **Can this customer have this product?** (eligibility evaluation)
2. **What do they still need to do?** (onboarding requirements)
3. **Has anything changed that affects their access?** (ongoing access control)

Products are referenced by ID — their definition lives in the core banking system. The eligibility engine owns **rulesets** and **subscriptions**, not product definitions.

### Architecture: Three Layers

```
┌─────────────────────────────────────────────────────┐
│  AI Layer (genkit) — FUTURE, built incrementally    │
│                                                     │
│  ┌────────────┐  ┌────────────┐  ┌──────────────┐  │
│  │ Rule       │  │ Onboarding │  │ Product      │  │
│  │ Author     │  │ Agent      │  │ Recommender  │  │
│  │            │  │            │  │              │  │
│  │ reg docs → │  │ conversa-  │  │ customer     │  │
│  │ CEL rules  │  │ tional     │  │ profile →    │  │
│  │            │  │ onboarding │  │ suggestions  │  │
│  └─────┬──────┘  └─────┬──────┘  └──────┬───────┘  │
│        │               │                │           │
├────────┼───────────────┼────────────────┼───────────┤
│  Eligibility Engine — THIS IS WHAT WE BUILD NOW    │
│                                                     │
│  ┌────────────┐  ┌────────────┐  ┌──────────────┐  │
│  │ Eval       │  │ Ruleset    │  │ Subscription │  │
│  │ Engine     │  │ Store      │  │ + Capability │  │
│  │ (CEL)      │  │            │  │ State        │  │
│  └────────────┘  └────────────┘  └──────────────┘  │
│                                                     │
├─────────────────────────────────────────────────────┤
│  Core Banking (SaaScada)                            │
│  Product config, ledger, payments, cards            │
└─────────────────────────────────────────────────────┘
```

### Layer 1: Eligibility Engine (build now)

The boring infrastructure that must be correct, auditable, and fast.

#### What it stores

| Entity | Purpose |
|---|---|
| **Ruleset** | A named, versioned eval engine YAML blob. Can be a base ruleset (reusable) or bound to a product. |
| **Product Eligibility** | A product ID (from core banking) + its ruleset + base ruleset references + geographic availability + tags. Flat — no hierarchy. |
| **Subscription** | A customer/entity's relationship with a product. Parties, capabilities, disabled states, eval state. |

#### What it does

| Operation | Description |
|---|---|
| `Evaluate(productID, customerData)` | Merge rulesets (base + product), run eval engine, return pass/fail per rule with resolution steps |
| `Subscribe(entityID, productID, parties)` | Create subscription, run initial evaluation, return what's needed |
| `Activate(subscriptionID, externalRef)` | Mark active, enable capabilities |
| `CheckAccess(entityID)` | Re-evaluate all active subscriptions, detect degradation, disable/enable capabilities |
| `Disable/Enable` | Subscription-level and capability-level control with reasons |

#### Product model: flat with tags

No hierarchy. Products are flat records with tags for filtering and grouping:

```go
type ProductEligibility struct {
    ProductID       string            // references core banking
    Tags            []string          // ["sharia:true", "market:uae", "segment:retail", "family:casa", "type:current_account"]
    Status          ProductStatus
    Ruleset         []byte            // eval engine YAML
    BaseRulesetIDs  []string
    Geographic      GeographicAvailability
    ShariaCompliant bool
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

Tags replace the rigid hierarchy:

| Old hierarchy | New tags |
|---|---|
| Family: CASA | `family:casa` |
| Archetype: Current Account | `type:current_account` |
| Market: UAE | `market:uae` |
| Segment: Retail | `segment:retail` |
| Sharia: Yes | `sharia:true` |
| Requires KYC: Yes | `requires_kyc:true` |

Querying is flexible: "give me all sharia-compliant CASA products available in UAE for retail customers" is a tag filter, not a hierarchy traversal.

#### Rulesets: base + product, no hierarchy levels

Base rulesets are reusable building blocks. Product rulesets are product-specific. Merge is simple:

```
base rulesets (by ID) + product ruleset → merged YAML → eval engine
```

No family-level or archetype-level rulesets. If you want "all investment products require age >= 18," create a base ruleset `base-investment-age-gate` and reference it from every investment product. The AI authoring layer (Layer 3) will automate this.

### Layer 2: AI Authoring Layer (build next)

Uses genkit to add intelligence on top of the eligibility engine.

#### Rule Author Agent

```
Input:  CBUAE regulatory PDF + product compliance requirements
Output: eval engine YAML (CEL expressions, reads/writes, resolution workflows)
Flow:
  1. Parse regulatory document
  2. Extract eligibility requirements as structured data
  3. Generate CEL expressions for each requirement
  4. Generate dependency graph (reads/writes)
  5. Validate via eval engine (no circular deps, valid CEL)
  6. Present to compliance officer for review
  7. On approval → store as base ruleset or product ruleset
```

This turns a weeks-long compliance analysis into a review-and-approve workflow.

#### Onboarding Agent

Replaces the rigid 6-module onboarding sequence from ADR-0001:

```
Input:  Customer context + eval engine results (what's failing)
Output: Conversational next step (what to collect, how to explain it)
Flow:
  1. Call Evaluate() to get current state
  2. Identify the highest-priority unmet requirement
  3. Generate a conversational prompt to collect the data
  4. After collection → call Evaluate() again
  5. Repeat until all requirements pass or customer exits
  6. Adapt dynamically (e.g., US passport detected → "I'll need your SSN")
```

The eval engine is the constraint system. The agent decides how to navigate it.

#### Product Recommender

```
Input:  Customer profile (what we know about them)
Output: Ranked list of products they're likely eligible for
Flow:
  1. Filter products by geography + tags
  2. For each candidate, run a lightweight pre-screen (how many rules would pass?)
  3. Rank by completeness (products closest to activation first)
  4. Personalize based on customer segment and behavior
```

### Layer 3: Product Configuration Engine (build later)

Once the eligibility engine and AI authoring layer are stable, extend to own product configuration:

- Product creation via natural language ("I need an AED savings account for UAE residents, Sharia-compliant, with a 90-day notice period")
- Configuration sync with core banking system
- A/B testing of product configurations
- Market expansion automation ("replicate this product for the Saudi market, adjusting for SAMA regulations")

This is where prodcat eventually becomes a full product configuration engine — but starting from intelligence, not CRUD.

---

## Implementation Plan

### Phase 1: Eligibility Engine (now)

Simplify the existing codebase:

1. **Flatten the product model** — remove Family and Archetype tables, add tags
2. **Integrate the eval engine** — actually merge and execute rulesets, not just store YAML
3. **Keep subscriptions** — the Stripe-style model with parties and capabilities is solid
4. **Service interface** — `EligibilityService` replaces `CatalogService`

```go
type EligibilityService interface {
    // Rulesets
    CreateRuleset(ctx, Ruleset) (Ruleset, RulesetValidation, error)
    GetRuleset(ctx, id) (Ruleset, error)
    ListRulesets(ctx) ([]Ruleset, error)
    UpdateRuleset(ctx, Ruleset) (Ruleset, RulesetValidation, error)

    // Product eligibility (flat, tagged)
    RegisterProduct(ctx, ProductEligibility) (ProductEligibility, error)
    GetProduct(ctx, productID) (ProductEligibility, error)
    ListProducts(ctx, TagFilter) ([]ProductEligibility, error)
    UpdateProduct(ctx, ProductEligibility) (ProductEligibility, error)

    // Evaluation
    Evaluate(ctx, productID, EvaluationInput) (EvaluationResult, error)
    ResolveRuleset(ctx, productID) (ResolvedRuleset, error)
}

type SubscriptionService interface {
    // Same as before — Subscribe, Activate, Cancel,
    // Disable/Enable, Capabilities, Parties, SigningAuthority, CheckAccess
}
```

### Phase 2: AI Rule Authoring (next)

1. **Genkit flow** for regulatory document → CEL rules
2. **Review/approval workflow** — generated rules go to compliance officer
3. **Version control** — rulesets are versioned, changes are auditable
4. **Feedback loop** — flag rules that frequently cause customer drop-off

### Phase 3: Onboarding Agent (after)

1. **Genkit agent** that uses eval engine as constraint system
2. **Conversational onboarding** — adapts to customer context
3. **Multi-channel** — works in-app, via chat, eventually voice
4. **Learning** — improves step ordering based on completion rates

### Phase 4: Product Configuration Engine (later)

1. **Natural language product creation**
2. **Core banking sync** — push config to SaaScada
3. **Market expansion automation**
4. **A/B testing infrastructure**

---

## Consequences

### Benefits

- **Focus on intelligence, not plumbing.** The eligibility engine is minimal infrastructure. The AI layers are where we differentiate.
- **Flexible product model.** Tags instead of hierarchy means no migrations when product structure changes.
- **Incremental AI adoption.** Each layer can be built and deployed independently. The eligibility engine works without AI. AI makes it better.
- **Regulatory compliance by design.** The eval engine provides deterministic, auditable decisions. AI assists humans in writing rules — it doesn't make compliance decisions.

### Trade-offs

- **Tags require discipline.** Without a hierarchy enforcing structure, tag conventions must be documented and enforced (potentially by the AI authoring layer itself).
- **Base ruleset management is manual (Phase 1).** Until the AI authoring layer exists, someone still writes YAML. But the scope is smaller — just rulesets, not a full product catalogue.
- **Dependency on eval engine.** The entire system depends on the eval engine's CEL capabilities. This is acceptable since we control it.

### What we're explicitly not building

- **Product CRUD** — core banking system owns this
- **Fee/rate configuration** — core banking system owns this
- **Static onboarding flows** — the AI agent replaces rigid modules
- **Rigid product hierarchies** — tags replace taxonomy

---

## References

- [ADR-0001: Product Catalogue Architecture](0001_product_catalogue_architecture.md) — superseded approach
- [Eval Engine](https://github.com/laenen-partners/evalengine) — CEL-based rules engine
- [Modular Onboarding Framework](../chain-of-thought/0001_product_cat.md) — original design document (onboarding modules replaced by AI agent in Phase 3)
