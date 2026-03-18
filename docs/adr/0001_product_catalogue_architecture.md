# ADR-0001: Product Catalogue Architecture

**Date**: 2026-03-18
**Status**: Proposed
**Authors**: Pascal Laenen, David Henry

---

## Context

We are building a digital banking platform ("Mal Bank") positioned as the Revolut for Islamic banking. The platform needs a product catalogue that defines what products exist, who is eligible for them, and what customers must complete to subscribe. The actual product operational details (fees, interest/profit rates, transaction limits) live in the core banking system — this catalogue is deliberately lightweight.

The platform launches in the UAE (CBUAE-regulated) but must extend globally. Sharia compliance is a first-class concern, not an afterthought. Products range from non-KYC offerings (PFM, Travel Agent) to fully regulated banking products (CASA, cards, lending, investments).

### Goals

- Declarative product definitions stored as data (not code)
- Composable eligibility rules using the eval engine (CEL expressions)
- Stripe-inspired subscription model with granular capability control
- Progressive unlock: customers access products as they complete requirements
- Incremental onboarding: never ask for what's already been collected
- Geographic extensibility from day one

### Technology Stack

| Component | Technology |
|-----------|-----------|
| Language | Go |
| API | connect-go (gRPC + HTTP) |
| Database | PostgreSQL via SQLC |
| Proto tooling | buf |
| Rules engine | [evalengine](https://github.com/laenen-partners/evalengine) (CEL-based) |
| AI | genkit |

---

## Decision

### 1. Product Hierarchy

Products are organised in a three-tier hierarchy. Each tier can define eligibility rules that gate everything below it.

```
ProductFamily
  └── ProductArchetype
        └── Product
```

**ProductFamily** is the broadest grouping. Families define high-level eligibility gates that apply to all products within them.

| Family | Example gate |
|--------|-------------|
| CASA | Customer must have a verified identity |
| Investments | Customer must be 18 or older |
| Lending | Customer must not be a non-resident |
| Cards | Customer must have at least one active CASA product |
| PFM | No gates (available to all) |

**ProductArchetype** groups related products within a family. Archetypes define eligibility rules that apply to all products of that type.

| Family | Archetype | Example gate |
|--------|-----------|-------------|
| CASA | Current Account | Must have UAE residency (for UAE archetypes) |
| CASA | Savings Account | Must have UAE residency + Sharia commodity agreement |
| Investments | Wealth Management | Must be a professional investor |

**Product** is the subscribable unit. Each product is a row in the database with its own eligibility rules, compliance metadata, and eval engine ruleset.

| Archetype | Product | Currency | Provider |
|-----------|---------|----------|----------|
| Current Account | AED Current Account | AED | Mal (CBUAE) |
| Current Account | USD Current Account | USD | Zenus (US-regulated) |
| Savings Account | AED On-Demand Savings | AED | Mal (CBUAE) |

Products have a type:

- **Primary**: Standalone products with their own KYC requirements. Can be activated independently.
- **Supplementary**: Linked to a parent primary product. Inherit the parent's KYC. Require only their own legal agreements. Parent must be active.

Products have a lifecycle status:

```
DRAFT → ACTIVE → SUSPENDED → DEPRECATED → RETIRED
```

- `DRAFT`: Being defined, not visible to customers.
- `ACTIVE`: Available for new subscriptions.
- `SUSPENDED`: No new subscriptions, existing customers unaffected.
- `DEPRECATED`: Being phased out, existing customers migrated.
- `RETIRED`: Fully removed.

### 2. Eligibility & the Eval Engine

Eligibility is the core function of the product catalogue. Every level of the hierarchy (family, archetype, product) can define rules that are evaluated by the [eval engine](https://github.com/laenen-partners/evalengine).

#### 2.1 Evaluation Input

The `EvaluationInput` protobuf message is the contract between the platform and the eval engine. It contains all customer data needed to evaluate eligibility for any product. The onboarding service hydrates this message from the customer's profile and passes it to the engine.

```protobuf
message EvaluationInput {
  // Contact Information
  repeated Contact contacts = 1;            // Email, phone with verified status

  // Security
  bool mpin_created = 2;

  // Identity & Verification
  repeated IdentityDocument id_documents = 3; // Scanned documents with OCR data
  bool liveness_passed = 4;
  int32 age = 5;                             // Derived from DOB at eval time
  repeated string nationalities = 6;         // All nationalities (dual citizenship)

  // KYC Profile
  Address residential_address = 7;
  Document proof_of_address = 8;
  string employment_status = 9;
  string income_range = 10;
  repeated string income_sources = 11;
  repeated string tax_residency_countries = 12;
  string tin_1 = 13;
  string tin_2 = 14;
  string tin_3 = 15;
  string tin_4 = 16;
  optional bool pep_declared = 17;           // optional — CEL uses has() to check
  bool professional_investor = 18;
  string risk_tolerance = 19;                // "low", "medium", "high"

  // Risk (system-sourced from Oscilar)
  int32 risk_score = 20;                     // 0-100

  // EDD (Enhanced Due Diligence)
  string source_of_wealth = 21;
  repeated Document source_of_wealth_docs = 22;
  string source_of_funds = 23;
  string purpose_of_product = 24;
  string business_ownership = 25;

  // Legal Agreements
  repeated Agreement agreements = 26;        // Type + accepted + version

  // Additional Identity Documents
  repeated Document additional_id_documents = 27;

  // Context
  string country = 28;                       // Derived from phone country code
  string customer_type = 29;                 // Customer segment
}
```

CEL expressions reference fields as `input.contacts`, `input.age`, `input.pep_declared`, etc. The eval engine compiles these expressions against the `EvaluationInput` proto descriptor, ensuring type safety at ruleset validation time.

Supporting messages:

| Message | Fields | Purpose |
|---------|--------|---------|
| `Contact` | type (EMAIL=2, PHONE=3), value, primary, verified, verified_at | Contact methods with verification state |
| `IdentityDocument` | type, scanned, expired, document_number, full_name, date_of_birth, gender, city/country_of_birth, nationalities, issue/expiry_date, issuing_country | Scanned ID with OCR data |
| `Address` | address_line_1/2, city, state_province, post_code, country | Structured residential address |
| `Agreement` | type, accepted, version, accepted_at | Legal agreement acceptance |
| `Document` | document_type, uploaded, verified, uploaded_at, expiry_date | Uploaded documents (proof of address, EDD docs) |

Usage in Go:

```go
cfg, _ := evalengine.LoadDefinitions(mergedRulesetYAML)
engine, _ := evalengine.NewEngine(cfg, &prodcatv1.EvaluationInput{})
results := engine.Run(input)
```

#### 2.2 Eval Engine Overview

The eval engine is a stateless CEL-based rules engine. Rules are defined in YAML with the following structure:

```yaml
evaluations:
  - name: email_verified
    description: Primary email address is verified
    expression: >
      input.contacts.exists(c, c.type == 2 && c.primary == true && c.verified == true)
    reads: [input.contacts]
    writes: email_verified
    resolution_workflow: EmailVerificationWorkflow
    resolution: "Verify your email address"
    severity: blocking
    category: contact
```

Key concepts:

| Field | Purpose |
|-------|---------|
| `name` | Unique identifier for the evaluation |
| `expression` | CEL expression that must return a boolean |
| `reads` | Dependencies — input fields (`input.*`) or outputs from other evaluations |
| `writes` | Output field name produced by this evaluation |
| `severity` | `blocking` (hard stop) or `warning` (informational) |
| `resolution_workflow` | Workflow to trigger if the evaluation fails |
| `resolution` | Human-readable guidance for the customer |
| `category` | Grouping for reporting (contact, security, identity, kyc, legal, edd) |
| `cache_ttl` | Optional caching for evaluations that only read input fields |

The engine automatically resolves dependencies between evaluations and executes them in topological order. Circular dependencies are detected and rejected at validation time.

#### 2.2 Ruleset Configuration

Every level of the product hierarchy uses the same `RulesetConfig` structure:

```protobuf
message RulesetConfig {
  // Eval engine YAML content for this level.
  bytes ruleset = 1;
  // IDs of shared base rulesets this level inherits.
  repeated string base_ruleset_ids = 2;
}
```

This means a family, an archetype, and a product can all:

1. Define their own eval engine rules (the `ruleset` YAML blob)
2. Reference shared base rulesets by ID (the `base_ruleset_ids`)

#### 2.3 Base Rulesets

Base rulesets are reusable rule sets that can be referenced from any level of the hierarchy. They prevent duplication of common requirements.

Examples:

| Base Ruleset | Contains |
|-------------|----------|
| `base-contact-verification` | email_verified, phone_verified |
| `base-security` | mpin_created (depends on phone_verified) |
| `base-idv` | id_document_scanned, id_type_accepted, liveness_check_passed, age_eligible |
| `base-kyc` | residential_address_provided, employment_provided, tax_residency_provided, pep_declaration_provided |

Base rulesets are created and managed via the API (`CreateBaseRuleset`, `UpdateBaseRuleset`) and stored in the database. They are validated by the eval engine on create/update.

#### 2.4 Ruleset Resolution

When the system needs to evaluate a customer against a product, it resolves the full ruleset by merging all layers:

```
1. Base rulesets referenced by family    + family's own ruleset
2. Base rulesets referenced by archetype + archetype's own ruleset
3. Base rulesets referenced by product   + product's own ruleset
4. All merged into a single YAML → fed to the eval engine
```

The `ResolveProductRuleset` RPC performs this merge and validates the result (checking for circular dependencies across layers, duplicate writes, etc.).

```protobuf
rpc ResolveProductRuleset(ResolveProductRulesetRequest) returns (ResolveProductRulesetResponse);
```

The response includes:

- The merged YAML content
- Validation results
- Which layers contributed which evaluations (for debugging/auditing)

#### 2.5 Evaluation Hierarchy Example

```
Family: CASA
  base_ruleset_ids: ["base-contact-verification", "base-security"]
  ruleset: (empty — no family-specific rules beyond base)

  Archetype: Current Account
    base_ruleset_ids: ["base-idv", "base-kyc"]
    ruleset:
      evaluations:
        - name: uae_residency_check
          expression: >
            input.id_documents.exists(d, d.type == "emirates_id")
          severity: blocking
          category: eligibility

    Product: AED Current Account
      eligibility.rules:
        base_ruleset_ids: []
        ruleset:
          evaluations:
            - name: aed_ca_tc_accepted
              expression: >
                input.agreements.exists(a, a.type == "aed_ca_tc" && a.accepted)
              severity: blocking
              category: legal

            - name: key_facts_accepted
              expression: >
                input.agreements.exists(a, a.type == "key_facts" && a.accepted)
              severity: blocking
              category: legal
```

Merged result for "AED Current Account":

```
email_verified          (from base-contact-verification)
phone_verified          (from base-contact-verification)
mpin_created            (from base-security)
id_document_scanned     (from base-idv)
id_type_accepted        (from base-idv)
liveness_check_passed   (from base-idv)
age_eligible            (from base-idv)
residential_address_provided  (from base-kyc)
employment_provided     (from base-kyc)
tax_residency_provided  (from base-kyc)
pep_declaration_provided (from base-kyc)
uae_residency_check     (from archetype)
aed_ca_tc_accepted      (from product)
key_facts_accepted      (from product)
```

#### 2.6 Geographic Eligibility

Geographic availability is defined at the product level and supports three modes:

| Mode | Behaviour |
|------|-----------|
| `SPECIFIC_COUNTRIES` | Available only in listed countries (e.g., UAE only) |
| `GLOBAL` | Available everywhere |
| `GLOBAL_EXCEPT` | Available everywhere except listed countries |

Optional sub-national restrictions can narrow availability within a country using ISO 3166-2 subdivision codes (e.g., `AE-DU` for Dubai).

#### 2.7 Acceptable ID Configuration

Each product defines which identity document types it accepts. This is evaluated **post-scan** — the customer never selects their document type upfront.

```protobuf
message AcceptableIDConfig {
  string id_type_id = 1;        // e.g., "emirates_id" or "passport"
  bool is_category_wildcard = 2; // true = accept all types in this category
  repeated string issuing_geo_filter = 3; // restrict to IDs from specific countries
}
```

Example: AED Current Account accepts `category=national_id + issuing_geo_filter=["AE"]`, which means only UAE-issued national IDs (Emirates ID). A Kuwaiti national ID would be rejected post-scan with guidance to use an acceptable document.

#### 2.8 Customer Segments

Products can restrict eligibility by customer type:

| Segment | Description |
|---------|-------------|
| `INDIVIDUAL` | Retail / personal banking |
| `SOLE_PROPRIETOR` | Freelancers, sole traders |
| `SME` | Small and medium enterprises |
| `CORPORATE` | Large corporates |
| `MINOR` | Under 18 (specific products) |
| `NON_RESIDENT` | Non-resident customers |

### 3. Compliance

The product catalogue stores lightweight compliance metadata. Detailed regulatory reporting lives in the compliance system.

```protobuf
message ComplianceConfig {
  bool sharia_compliant = 1;
  repeated RegulatoryClassification classifications = 2;
  repeated LegalAgreement agreements = 3;
}
```

**Sharia compliance** is a boolean flag on the product. The underlying Islamic contract type (wadiah, mudarabah, murabaha, wakalah, etc.) is a core banking system concern — the catalogue only needs to know whether the product is Sharia-compliant to drive agreement requirements (e.g., Shariah Commodity Purchase Agreement).

**Legal agreements** are defined per product with type, version, and a document reference. Shared agreements (e.g., privacy policy) are flagged as `shared = true` so they don't trigger re-acceptance when subscribing to a second product — unless the version changes.

| Agreement Type | Scope |
|---------------|-------|
| Terms & Conditions | Per product |
| Privacy Policy | Shared |
| Key Facts Statement | Per product |
| Shariah Commodity Purchase | Per product (Sharia-compliant only) |
| Data Processing | Shared |

### 4. Subscriptions

A subscription is created when a customer begins the onboarding journey for a product. It tracks the eligibility state and provides granular control over what the customer can do.

#### 4.1 Subscription Status

Modelled after Stripe's subscription lifecycle:

| Status | Meaning |
|--------|---------|
| `INCOMPLETE` | Onboarding in progress, not all requirements met |
| `ACTIVE` | All requirements met, product is live |
| `PAST_DUE` | One or more data elements have expired (in grace period) |
| `CANCELED` | Canceled by customer or operations |

#### 4.2 Disabled State (Stripe-style)

Both the subscription as a whole and individual capabilities can be independently disabled with a reason. This replaces the traditional monolithic "suspended" state with something more granular and actionable.

```protobuf
message DisabledState {
  bool disabled = 1;
  DisabledReason reason = 2;
  string message = 3;
  google.protobuf.Timestamp disabled_at = 4;
  repeated string failed_evaluations = 5;
}
```

Disabled reasons:

| Reason | Trigger |
|--------|---------|
| `REQUIREMENTS_NOT_MET` | Eligibility requirement not satisfied |
| `EXPIRED_DATA` | Document or data element has expired |
| `FAILED_EVALUATION` | Re-evaluation failed (e.g., risk score changed) |
| `REGULATORY_HOLD` | Compliance, AML, or sanctions hold |
| `FRAUD_SUSPICION` | Fraud detection triggered |
| `CUSTOMER_REQUESTED` | Customer asked to disable |
| `OPERATIONS` | Manual action by operations team |
| `PARENT_DISABLED` | Parent subscription disabled (cascades to supplementary products) |

The `failed_evaluations` field links back to specific eval engine results, making it possible to show the customer exactly what they need to resolve.

#### 4.3 Capabilities

Each subscription has a set of capabilities that can be independently enabled or disabled. This models degradation gracefully — an expired Emirates ID disables transfers but keeps balance viewing active.

| Capability | Description |
|-----------|-------------|
| `VIEW` | View balances and transaction history |
| `DOMESTIC_TRANSFERS` | Make domestic transfers |
| `INTERNATIONAL_TRANSFERS` | Make international transfers |
| `CARD_PAYMENTS` | Use the debit/credit card |
| `ATM` | ATM withdrawals |
| `RECEIVE` | Receive incoming payments |
| `BILL_PAYMENTS` | Pay bills |
| `FX` | Currency conversion |
| `STANDING_ORDERS` | Recurring transfers |
| `CUSTOM` | Product-specific capabilities |

Each capability has its own status (`ACTIVE`, `DISABLED`, `PENDING`) and, if disabled, its own `DisabledState` with a reason and resolution path.

Capabilities can also have their own eval engine requirements. For example, `INTERNATIONAL_TRANSFERS` might require additional KYC beyond what the base product requires.

#### 4.4 Eval State

Every subscription tracks the result of the last eval engine run:

```protobuf
message EvalState {
  EvalStatus overall_status = 1;    // ALL_PASSED, WORKFLOW_ACTIVE, ACTION_REQUIRED, BLOCKED
  repeated EvalResult results = 2;  // Per-evaluation outcomes
  repeated string deferred = 3;     // Evaluations that can't run yet (missing data)
  Timestamp evaluated_at = 4;       // When the last evaluation ran
  repeated string layer_ids = 5;    // Which ruleset layers were included
}
```

The eval state is updated every time `Evaluate` is called — after the customer provides new data, after a workflow completes, or on a periodic access check.

#### 4.5 Subscription Lifecycle

```
Customer requests product
        │
        ▼
    Subscribe()
        │ Creates subscription with status=INCOMPLETE
        │ Runs initial evaluation
        │ Returns what the customer still needs to do
        ▼
    ┌─────────────────────┐
    │     INCOMPLETE       │ ◄─── Customer completes steps
    │                     │       Evaluate() after each
    │  eval_state shows   │       Plan updates dynamically
    │  what's left        │       (conditional requirements may appear)
    └─────────┬───────────┘
              │ All evaluations pass
              ▼
        Activate()
              │ Provision in core banking
              │ Set external_ref
              │ Enable capabilities
              ▼
    ┌─────────────────────┐
    │      ACTIVE          │ ◄─── CheckAccess() periodically
    │                     │
    │  All capabilities   │
    │  enabled            │
    └─────────┬───────────┘
              │ Data expires / re-eval fails
              ▼
    ┌─────────────────────┐
    │     PAST_DUE         │
    │                     │
    │  Some capabilities  │ ◄─── Disable(reason: EXPIRED_DATA)
    │  disabled with      │       or auto-disabled by Evaluate()
    │  specific reasons   │
    └─────────┬───────────┘
              │ Customer refreshes data
              │ Evaluate() → all pass again
              ▼
    ┌─────────────────────┐
    │      ACTIVE          │ ◄─── Enable() / EnableCapability()
    │     (restored)      │
    └─────────────────────┘
```

Non-blocking: being in `INCOMPLETE` for one product does not affect other active subscriptions. The customer can exit and resume at any time.

#### 4.6 Degradation Example

Customer has an active AED Current Account. Their Emirates ID expires.

**Before expiry** (warning phase, 30 days before):

```
subscription.status = ACTIVE
subscription.disabled = { disabled: false }
capabilities:
  - VIEW:                   ACTIVE
  - DOMESTIC_TRANSFERS:     ACTIVE
  - INTERNATIONAL_TRANSFERS: ACTIVE
  - CARD_PAYMENTS:          ACTIVE
  - ATM:                    ACTIVE
  - RECEIVE:                ACTIVE
```

Notification sent: "Your Emirates ID expires in 30 days. Update it to maintain full access."

**After expiry** (within 60-day grace period):

```
subscription.status = PAST_DUE
subscription.disabled = { disabled: false }
capabilities:
  - VIEW:                   ACTIVE
  - DOMESTIC_TRANSFERS:     DISABLED  { reason: EXPIRED_DATA, message: "Emirates ID expired", failed_evaluations: ["id_document_valid"] }
  - INTERNATIONAL_TRANSFERS: DISABLED  { reason: EXPIRED_DATA, ... }
  - CARD_PAYMENTS:          DISABLED  { reason: EXPIRED_DATA, ... }
  - ATM:                    DISABLED  { reason: EXPIRED_DATA, ... }
  - RECEIVE:                ACTIVE
```

The customer can still view balances and receive payments, but cannot transact. The disabled state tells them exactly why and what to do.

**Supplementary products** (e.g., Debit Card):

```
debit_card_subscription.status = PAST_DUE
debit_card_subscription.disabled = {
  disabled: true,
  reason: PARENT_DISABLED,
  message: "Parent account requires attention"
}
```

**After customer updates their Emirates ID**:

`Evaluate()` is called → `id_document_valid` passes → capabilities re-enabled → status returns to `ACTIVE`.

### 5. Services

The system exposes two connect-go services:

#### 5.1 ProductCatalogService

Manages the product hierarchy and eval engine rulesets.

| RPC | Purpose |
|-----|---------|
| `CreateFamily` / `GetFamily` / `ListFamilies` / `UpdateFamily` | Manage product families and their rulesets |
| `CreateArchetype` / `GetArchetype` / `ListArchetypes` / `UpdateArchetype` | Manage archetypes and their rulesets |
| `CreateProduct` / `GetProduct` / `ListProducts` / `UpdateProduct` | Manage products |
| `TransitionProductStatus` | Move products through their lifecycle (draft → active → ...) |
| `CreateBaseRuleset` / `GetBaseRuleset` / `ListBaseRulesets` / `UpdateBaseRuleset` | Manage reusable base rulesets |
| `ListAvailableProducts` | Product discovery with geographic + segment filtering |
| `ResolveProductRuleset` | Merge all ruleset layers for a product and validate |

All ruleset create/update operations return a `RulesetValidation` result from the eval engine (valid, errors, warnings, evaluation count, max dependency depth).

#### 5.2 SubscriptionService

Manages customer subscriptions and their lifecycle.

| RPC | Purpose |
|-----|---------|
| `Subscribe` | Start onboarding for a product (creates INCOMPLETE subscription) |
| `GetSubscription` / `ListSubscriptions` | Query subscriptions |
| `Evaluate` | Re-run the eval engine against current customer data; returns deltas |
| `Activate` | Mark subscription as ACTIVE after all requirements met; set external_ref |
| `Disable` / `Enable` | Disable/enable a subscription with a reason |
| `Cancel` | Cancel a subscription |
| `DisableCapability` / `EnableCapability` | Toggle individual capabilities |
| `CheckAccess` | Batch check all subscriptions for a customer; detects degradation |

### 6. Proto File Structure

```
proto/prodcat/v1/
├── types.proto                  — LocalizedText, DateRange
├── geography.proto              — GeographicAvailability, RegulatoryProvider
├── compliance.proto             — ComplianceConfig, LegalAgreement
├── eligibility.proto            — RulesetConfig, EligibilityConfig, AcceptableIDConfig, CustomerSegment
├── evaluation_input.proto       — EvaluationInput, Contact, IdentityDocument, Address, Agreement, Document
├── product.proto                — ProductFamilyDefinition, ProductArchetype, Product
├── product_service.proto        — ProductCatalogService RPCs
├── subscription.proto           — Subscription, DisabledState, Capability, EvalState
└── subscription_service.proto   — SubscriptionService RPCs
```

### 7. Data Flow

#### 7.1 Product Creation (Operations Hub)

```
Operations Hub
    │
    ├── CreateFamily("CASA", ruleset: base-contact + base-security)
    ├── CreateArchetype("Current Account", family: CASA, ruleset: base-idv + base-kyc + residency check)
    ├── CreateProduct("AED Current Account", archetype: Current Account, ruleset: legal agreements)
    │
    └── ResolveProductRuleset("AED Current Account")
            → Returns merged YAML with all 14 evaluations
            → Validates no circular deps, no duplicate writes
```

#### 7.2 Customer Onboarding

```
Customer taps "Open AED Account"
    │
    ▼
ListAvailableProducts(country: "AE", type: INDIVIDUAL)
    → Returns AED Current Account (among others)
    │
    ▼
Subscribe(customer_id, product_id: "aed-ca")
    → Creates subscription (INCOMPLETE)
    → Runs initial Evaluate()
    → Returns eval_state:
        email_verified: false (resolution: "Verify your email")
        phone_verified: false (resolution: "Verify your phone")
        mpin_created: false (blocked — depends on phone_verified)
        ... (12 more evaluations, most deferred)
    │
    ▼
Customer verifies email → Evaluate()
    → email_verified: true ✓
    → phone_verified: false (still needed)
    │
    ▼
Customer verifies phone → Evaluate()
    → phone_verified: true ✓
    → mpin_created: false (now unblocked — was waiting on phone_verified)
    │
    ▼
Customer creates MPIN → Evaluate()
    → mpin_created: true ✓
    → id_document_scanned: false (next step)
    │
    ▼
... (continues through ID&V, KYC, legal agreements)
    │
    ▼
All evaluations pass → Activate(subscription_id, external_ref: "saascada-123")
    → Status: ACTIVE
    → All capabilities enabled
```

#### 7.3 Periodic Access Check

```
CheckAccess(customer_id)
    │ For each active subscription:
    │   Evaluate() with current customer data
    │   Compare to previous eval_state
    │   If any evaluation newly fails:
    │     Disable relevant capabilities
    │     Set disabled reason + failed evaluations
    │     Update subscription status if needed
    │
    ▼
Returns SubscriptionAccess[] per subscription with:
    - Current status
    - Disabled state (if any)
    - Per-capability status + disabled reasons
```

---

## Consequences

### Benefits

- **Separation of concerns**: Product catalogue owns eligibility; core banking owns operational details. Neither system duplicates the other's responsibilities.
- **Composable rules**: Base rulesets + hierarchy-level rulesets eliminate duplication. A change to "base-kyc" propagates to all products that reference it.
- **Granular degradation**: Stripe-style disabled states with reasons give both the customer and operations clear visibility into what's wrong and how to fix it.
- **Geographic extensibility**: Adding a new market means creating new products with appropriate rulesets, not changing the system architecture.
- **Auditability**: Every eval engine run is recorded on the subscription. The `ResolveProductRuleset` RPC shows exactly which layers contribute which rules.
- **No-deploy configuration**: All product definitions, rulesets, and eligibility rules are data. Changes take effect at the next evaluation without an app deploy.

### Trade-offs

- **Eval engine dependency**: The entire eligibility model depends on the eval engine. If the engine has limitations (e.g., CEL expression complexity), they constrain what rules can express.
- **Ruleset merge complexity**: Merging rulesets across four layers (base + family + archetype + product) requires careful validation to avoid conflicts. The `ResolveProductRuleset` RPC mitigates this but adds a step.
- **Eventual consistency**: The eval state on a subscription is a snapshot. Between evaluations, the actual customer data may have changed. Periodic `CheckAccess` calls are needed.
- **Capability model maintenance**: The predefined `CapabilityType` enum must be extended as new product features are added. The `CUSTOM` type provides an escape hatch.

---

## Appendix A: Eval Engine Integration

### A.1 Base Ruleset: Contact Verification

```yaml
# base-contact-verification
evaluations:
  - name: email_verified
    description: Primary email address is verified
    expression: >
      input.contacts.exists(c, c.type == 2 && c.primary == true && c.verified == true)
    reads: [input.contacts]
    writes: email_verified
    resolution_workflow: EmailVerificationWorkflow
    resolution: "Verify your email address"
    severity: blocking
    category: contact

  - name: phone_verified
    description: Primary phone number is verified
    expression: >
      input.contacts.exists(c, c.type == 3 && c.primary == true && c.verified == true)
    reads: [input.contacts]
    writes: phone_verified
    resolution_workflow: PhoneVerificationWorkflow
    resolution: "Verify your phone number"
    severity: blocking
    category: contact
```

### A.2 Base Ruleset: Security

```yaml
# base-security
evaluations:
  - name: mpin_created
    description: Mobile PIN has been created
    expression: >
      phone_verified && input.mpin_created == true
    reads: [phone_verified, input.mpin_created]
    writes: mpin_created
    resolution_workflow: MpinCreationWorkflow
    resolution: "Create your mobile PIN"
    severity: blocking
    category: security
```

### A.3 Base Ruleset: Identity & Verification

```yaml
# base-idv
evaluations:
  - name: id_document_scanned
    description: A valid ID document has been scanned
    expression: >
      input.id_documents.exists(d, d.scanned == true && d.expired == false)
    reads: [input.id_documents]
    writes: id_document_scanned
    resolution_workflow: IDScanWorkflow
    resolution: "Scan your identity document"
    severity: blocking
    category: identity

  - name: liveness_check_passed
    description: Biometric liveness check completed
    expression: >
      id_document_scanned && input.liveness_passed == true
    reads: [id_document_scanned, input.liveness_passed]
    writes: liveness_check_passed
    resolution_workflow: LivenessCheckWorkflow
    resolution: "Complete the biometric liveness check"
    severity: blocking
    category: identity

  - name: age_eligible
    description: Customer is 18 years or older
    expression: >
      id_document_scanned && input.age >= 18
    reads: [id_document_scanned, input.age]
    writes: age_eligible
    resolution: "You must be 18 or older"
    severity: blocking
    category: eligibility
```

### A.4 Base Ruleset: KYC

```yaml
# base-kyc
evaluations:
  - name: residential_address_provided
    description: Residential address has been provided
    expression: >
      input.residential_address.country != ""
    reads: [input.residential_address]
    writes: residential_address_provided
    resolution_workflow: AddressCollectionWorkflow
    resolution: "Provide your residential address"
    severity: blocking
    category: kyc

  - name: employment_provided
    description: Employment status and income have been provided
    expression: >
      input.employment_status != "" && input.income_range != ""
    reads: [input.employment_status, input.income_range]
    writes: employment_provided
    resolution_workflow: EmploymentCollectionWorkflow
    resolution: "Provide your employment details"
    severity: blocking
    category: kyc

  - name: tax_residency_provided
    description: Tax residency and TIN have been provided
    expression: >
      size(input.tax_residency_countries) > 0 && input.tin_1 != ""
    reads: [input.tax_residency_countries, input.tin_1]
    writes: tax_residency_provided
    resolution_workflow: TaxResidencyWorkflow
    resolution: "Provide your tax residency information"
    severity: blocking
    category: kyc

  - name: pep_declaration_provided
    description: PEP declaration has been made
    expression: >
      input.pep_declared != null
    reads: [input.pep_declared]
    writes: pep_declaration_provided
    resolution_workflow: PEPDeclarationWorkflow
    resolution: "Complete the PEP declaration"
    severity: blocking
    category: kyc
```

### A.5 Product Ruleset: AED Current Account

```yaml
# Product-level rules (layered on top of base + family + archetype)
evaluations:
  - name: id_type_accepted
    description: Scanned ID is an Emirates ID
    expression: >
      id_document_scanned && input.id_documents.exists(d, d.type == "emirates_id" && d.scanned == true)
    reads: [id_document_scanned, input.id_documents]
    writes: id_type_accepted
    resolution: "An Emirates ID is required for this product"
    severity: blocking
    category: identity

  - name: us_person_ssn_provided
    description: US persons must provide SSN as TIN
    expression: >
      tax_residency_provided && (!input.nationalities.exists(n, n == "US") || input.tin_1 != "")
    reads: [tax_residency_provided, input.nationalities, input.tin_1]
    writes: us_person_ssn_provided
    resolution_workflow: TaxResidencyWorkflow
    resolution: "US nationals must provide their SSN"
    severity: blocking
    category: kyc

  - name: edd_not_required_or_completed
    description: EDD not required, or completed if required
    expression: >
      pep_declaration_provided && (
        (input.pep_declared == false && input.risk_score < 70)
        || (input.source_of_wealth != "" && input.source_of_funds != "")
      )
    reads:
      - pep_declaration_provided
      - input.pep_declared
      - input.risk_score
      - input.source_of_wealth
      - input.source_of_funds
    writes: edd_completed
    resolution_workflow: EDDWorkflow
    resolution: "Complete Enhanced Due Diligence"
    severity: blocking
    category: edd

  - name: aed_ca_tc_accepted
    description: Terms and conditions accepted
    expression: >
      input.agreements.exists(a, a.type == "aed_ca_tc" && a.accepted == true)
    reads: [input.agreements]
    writes: aed_ca_tc_accepted
    resolution_workflow: AgreementWorkflow
    resolution: "Accept the AED Current Account Terms & Conditions"
    severity: blocking
    category: legal

  - name: privacy_policy_accepted
    description: Privacy policy accepted
    expression: >
      input.agreements.exists(a, a.type == "privacy_policy" && a.accepted == true)
    reads: [input.agreements]
    writes: privacy_policy_accepted
    resolution_workflow: AgreementWorkflow
    resolution: "Accept the Privacy Policy"
    severity: blocking
    category: legal
    cache_ttl: "24h"

  - name: key_facts_accepted
    description: Key Facts Statement accepted
    expression: >
      input.agreements.exists(a, a.type == "key_facts" && a.accepted == true)
    reads: [input.agreements]
    writes: key_facts_accepted
    resolution_workflow: AgreementWorkflow
    resolution: "Accept the Key Facts Statement"
    severity: blocking
    category: legal
```

---

## Appendix B: Product Catalogue — Initial UAE Products

| Product | Family | Archetype | Type | Currency | Provider | Sharia | Geography |
|---------|--------|-----------|------|----------|----------|--------|-----------|
| Mal PFM | PFM | PFM | Primary | — | Mal | No | Global |
| Mal Travel Agent | Value Added | Travel Agent | Primary | — | Mal | No | Global |
| AED Current Account | CASA | Current Account | Primary | AED | Mal (CBUAE) | Yes | UAE |
| USD Current Account | CASA | Current Account | Primary | USD | Zenus (US) | No | USA |
| AED On-Demand Savings | CASA | Savings Account | Primary | AED | Mal (CBUAE) | Yes | UAE |
| AED Debit Card | Cards | Debit Card | Supplementary → AED CA | AED | Mal (CBUAE) | Yes | UAE |
| UAE Bill Payments | Payments | Bill Payments | Supplementary → AED CA | AED | Mal (CBUAE) | Yes | UAE |
| Global Wealth Management | Investments | Wealth Management | Primary | AED | TBD | TBD | Global except exclusion list |
