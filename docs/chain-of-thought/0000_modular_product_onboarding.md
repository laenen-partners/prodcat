# Modular Onboarding Framework — Design Document

**Date**: March 4, 2026
**Status**: Draft
**Author**: David Henry

## Overview

A composable, eligibility-driven onboarding system for Mal Bank that replaces the current monolithic onboarding journey with modular components invoked on-demand based on product requirements. The system supports progressive unlock, incremental re-onboarding, multi-geography KYC, and configurable access degradation.

### Core Principles

1. **Onboarding is a service, not a flow** — invocable from anywhere in the app (product card, settings, degradation prompt), not a linear screen sequence
2. **Progressive unlock** — users start with Profile Creation and immediately access non-KYC products (PFM, Travel Agent). Banking products unlock through additional modules
3. **Incremental onboarding** — when requesting a new product, users provide only what's missing or stale, never repeat what's already collected
4. **Non-blocking journeys** — onboarding for a new product does not prevent access to already-active products. Users can exit and resume at any time
5. **Dynamic re-evaluation** — the onboarding plan updates mid-journey as new data triggers conditional requirements

---

## 1. Core Domain Model

### 1.1 Product Catalog

Two-tier structure: Product Archetype (grouping) → Product Instance (unit of eligibility).

| Product Archetype | Product Instance | Geo Availability | Regulatory Provider | Requires KYC? | Product Type | Acceptable ID Types |
|---|---|---|---|---|---|---|
| PFM | Mal PFM | Global | Mal | No | Primary | — |
| Travel Agent | Mal Travel Agent | Global | Mal | No | Primary | — |
| Current Account | AED Current Account | UAE | Mal (CBUAE-licensed) | Yes | Primary | Emirates ID only |
| Current Account | USD Current Account | USA | Zenus (US-regulated) | Yes | Primary | Any passport; national ID (per Zenus config) |
| Savings | AED On-Demand Savings | UAE | Mal (CBUAE-licensed) | Yes | Primary | Emirates ID only |
| Debit Card | AED Debit Card | UAE | Mal (CBUAE-licensed) | Yes | Supplementary → AED Current Account | Inherited from parent |
| Bill Payments | UAE Bill Payments | UAE | Mal (CBUAE-licensed) | Yes | Supplementary → AED Current Account | Inherited from parent |
| Wealth Management | Global Wealth Management | Global, except [exclusion list] | TBD | Yes | Primary | Any passport; any national ID (geo exclusion list applies) |

**Product types:**
- **Primary**: Carry their own KYC requirements. Can be activated independently.
- **Supplementary**: Linked to a parent primary product. Inherit parent's KYC. Require only their own legal agreements. Parent must be active.

**Geography availability modes:**
- `specific_countries` — inclusion list (e.g., UAE only)
- `global` — available everywhere
- `global_except` — exclusion list (e.g., "global except [country A]")

**Provider** = regulatory provider (the entity licensed to offer the product), not the technology platform. Technology partners (SaaScada, Paymentology) are implementation details, not part of the product model.

### 1.2 Data Element Registry

Every collectible piece of user data, with lifecycle metadata.

```
DataElement {
  element_id            // e.g., "emirates_id_number"
  display_name          // e.g., "Emirates ID"
  category              // profile | identity | kyc | legal | system
  collection_module     // which UI module collects this (1-6)
  validity_period       // e.g., "until_expiry_date", "365_days", "permanent"
  warning_window        // e.g., "30_days" before expiry — triggers non-blocking notification
  grace_period          // e.g., "60_days" after validity expires
  degradation_action    // "warn" | "soft_lock" | "hard_lock"
  geo_availability      // which geographies this element exists in
}
```

### 1.3 Requirement Matrix

Maps product instances to required data elements.

```
Requirement {
  product_instance_id
  data_element_id
  is_mandatory          // true | false
  freshness_override    // product can demand stricter freshness than element default
}
```

### 1.4 Conditional Requirements

Requirements triggered by collected data, not by the product itself.

```
ConditionalRequirement {
  trigger_element_id    // e.g., "id_document_nationality"
  trigger_condition     // e.g., "includes 'US'"
  required_element_id   // e.g., "tin_1"
  mandatory             // true
  context               // "US nationals must provide SSN as TIN 1"
}
```

### 1.5 User Profile (Runtime)

The user's collected data with timestamps, verification status, and collection context. Collection context fields are system-captured on every write — never user-provided — and are immutable after submission. They exist to support fraud investigation, device fingerprinting, and audit trails.

```
UserDataRecord {
  user_id
  data_element_id
  value                 // the actual data
  collected_at          // timestamp (ISO 8601 UTC)
  expires_at            // computed from validity_period or document expiry
  verification_status   // verified | pending | expired | rejected

  // Collection context — system-captured, immutable
  collection_ip         // IP address at time of submission
  collection_channel    // mobile_app | web (web = future)
  collection_device {
    platform            // ios | android | web
    os_version          // e.g., "iOS 18.3", "Android 15"
    device_make         // e.g., "Apple", "Samsung" (from device OS)
    device_model        // e.g., "iPhone 16 Pro", "Galaxy S25"
  }
}
```

**Why collection context matters**: If a fraud investigation requires reconstructing when and how a piece of data was submitted — for example, a document scan that later turns out to be fraudulent — investigators can cross-reference the IP, channel, and device against other sessions. Device make/model is particularly useful for detecting anomalous switching patterns (e.g., document submitted from a different device than all prior sessions).

### 1.6 Eligibility Rules (Pre-qualification)

Binary gates checked before onboarding begins. Separate from the Requirement Matrix (which drives data collection), these determine whether the user can have the product at all.

```
EligibilityRule {
  product_instance_id
  rule_id
  data_element_id        // element to check (from existing user profile)
  condition              // e.g., "age >= 21", "has_driving_license == true"
  failure_action         // "block" | "redirect" | "waitlist"
  failure_message        // "You must be 21+ to apply for this product"
}
```

Examples:

| Product | Rule | Element Checked | Condition | Failure Action |
|---|---|---|---|---|
| Car Loan | Age check | `id_document_dob` | age >= 18 | Block: "Must be 18+" |
| Car Loan | License required | `driving_license` | exists & valid | Block: "Driving license required" |
| AED Current Account | UAE residency | `id_document_type` | == Emirates ID | Block: "UAE residency required" |
| Wealth Management | Investor criteria (some geos) | `professional_investor` + `income_range` | meets threshold | Block: "Does not meet investor criteria" |

**Deferred rules**: Some eligibility rules reference data not yet collected (e.g., age unknown until Module 3). These are checked after the relevant module completes. If the user fails a deferred check, the journey stops with a clear explanation.

### 1.6b Acceptable ID Configuration

Defines which ID document types are accepted for a given product instance. Evaluated **post-scan** — the user is never asked to declare their ID type upfront. The explanation screen (shown before scanning begins) tells the user which documents are eligible in plain copy, but no selection step exists. The scan result determines the document type; the engine then validates it against config.

```
IDTypeDefinition {
  id_type_id            // e.g., "emirates_id" | "international_passport" | "gcc_national_id" | "national_id_card"
  display_name          // e.g., "Emirates ID", "International Passport"
  category              // national_id | passport | residence_permit | driving_license
  issuing_geos[]        // country codes that issue this type; empty = internationally issued (any country)
  implies_nationality   // true for national IDs and passports; false for residence permits
}

AcceptableIDConfig {
  product_instance_id
  acceptable[]{
    id_type_id            // specific type (e.g., "emirates_id") OR category wildcard (e.g., "passport")
    is_category_wildcard  // if true, id_type_id is a category — accept all types in that category
    issuing_geo_filter[]  // optional: restrict to IDs issued by specific countries only
                          // e.g., category=national_id + issuing_geo_filter=["AE"] → UAE national ID only
  }
}
```

**How this handles the Kuwait example**: User scans their Kuwait national ID. The scan extracts `id_document_type = "gcc_national_id"` and `id_document_nationality = ["KW"]`. The engine checks `AcceptableIDConfig` for the product — AED Current Account is configured as `category=national_id, issuing_geo_filter=["AE"]`. Kuwait ID fails the filter; user is shown a rejection screen with copy directing them to an acceptable document.

**Operations Hub**: `AcceptableIDConfig` is managed per product instance, not hardcoded. Adding support for a new ID type to an existing product requires no app deploy.

### 1.7 Core Eligibility Calculation

```
Step 1: Check EligibilityRules against existing profile
  → Any rule fails? → Return failure_action + message. No onboarding starts.
  → Rules referencing uncollected data? → Defer to post-module check.

Step 2: Compute onboarding plan
  required = RequirementMatrix.get(product_instance)
  satisfied = UserProfile.get(user_id).filter(valid AND fresh for product)
  conditional = ConditionalRules.evaluate(satisfied)
  needed = (required ∪ conditional) - satisfied
  → render onboarding steps for 'needed' elements, grouped by collection_module

Step 3: After each module, re-evaluate:
  - Check deferred eligibility rules
  - Re-run conditional rules with newly collected data
  - Update remaining plan
```

---

## 2. Onboarding Modules

Six self-contained UI modules, each collecting a group of related data elements.

### Module 1: User Profile Creation
- **Trigger**: App first launch (mandatory for everyone)
- **Collects**: `mobile_number` (+ OTP verification), `email`, `first_name`, `middle_names`, `last_name`, `profile_picture_url`, `mpin`, `biometric_preference`
- **OTP flow**: After mobile number entry, a one-time passcode is sent via SMS. Successful verification sets `mobile_number_verified = true` and is a hard prerequisite — `user_id` is not created until the number is verified. Prevents account creation with an unowned number.
- **Name fields**: Separated into `first_name`, `middle_names` (optional, array), and `last_name` to avoid first/middle commingling. `id_document_name` in Module 3 captures the full legal name from OCR separately — the two are not merged. Display name in-app uses the Module 1 fields; legal name for compliance uses Module 3.
- **Products**: All — universal prerequisite
- **Result**: User exists with a `user_id`. PFM and Travel Agent accessible immediately, for example.

### Module 2: Product Intent (Orchestration)
- **Trigger**: User requests a product (taps "Cards" bubble, taps "Accounts" bubble, etc.)
- **Function**: Resolves product instance from user context (geo from phone country code + selected product). Runs eligibility diff. Returns the OnboardingPlan.
- **No data collected** — this is the orchestration layer, not a UI step.

### Module 3: Identity & Verification (ID&V)
- **Trigger**: Eligibility engine determines ID&V is required
- **Collects**: `id_document_type`, `id_document_scan`, `id_document_number`, `id_document_nationality` (supports multiple for dual citizens), `id_document_expiry`, `id_document_issue_date`, `id_document_name`, `id_document_dob`, `id_document_gender`, `id_document_city_of_birth`, `id_document_country_of_birth`, `liveness_check`, optionally `additional_id_document`
- **UX**: An explanation screen shown before scanning tells the user which documents are acceptable (in copy). No upfront selection step — the user simply scans whatever they have.
- **Post-scan validation**: The scan result determines `id_document_type` and `id_document_nationality`. The engine then checks `AcceptableIDConfig` for the product instance. If the scanned document fails (wrong type or issuing country), the user is shown a rejection screen with guidance on acceptable documents and can retry.
- **Post-scan deferred eligibility**: After type validation passes, deferred EligibilityRules evaluate `id_document_nationality` — catches cases where the document type is acceptable but the nationality implies ineligibility (e.g., a UAE residence permit held by a national from an ineligible country).
- **Geo-specific**: The module is a shell — acceptable ID types, verification provider, and liveness requirements are all driven by the product instance's `AcceptableIDConfig` and regulatory provider. UAE default = Emirates ID + EFR liveness. USA = per Zenus requirements.

### Module 4: KYC Profile Questions
- **Trigger**: Eligibility engine determines KYC elements are needed
- **Collects**: Variable set drawn from Data Element Registry: `residential_address`, `proof_of_address`, `employment_status`, `income_range`, `income_sources`, `tax_residency_countries`, `tin_1` through `tin_4`, `pep_declaration`, `professional_investor`, `risk_tolerance`
- **Key**: Frontend renders only elements the user hasn't provided (or that are stale). If they already gave their address for AED Current Account and now want USD, they skip address but get prompted for proof of address.

### Module 5: Legal Agreements
- **Trigger**: All required data elements collected for the requested product
- **Collects**: `agreement_{product}_tc`, `agreement_privacy_policy`, `agreement_key_facts`, `agreement_shariah_commodity`
- **Key**: Each product instance has its own T&C documents. Previously accepted shared documents (like privacy policy) don't need re-acceptance unless the version changes.

### Module 6: Enhanced Due Diligence (EDD)
- **Trigger**: Risk-based — activated by `customer_risk_score` (from Oscilar), `pep_declaration` = true, or conditional rules
- **Collects**: `source_of_wealth`, `source_of_wealth_docs`, `source_of_funds`, `purpose_of_product`, `business_ownership`
- **Key difference from Module 4**: KYC is product-driven and predictable. EDD is risk-driven and may trigger after initial onboarding (e.g., post-screening flags something). Can also trigger during onboarding if conditional rules fire.
- **Data storage**: EDD elements live in the same unified Data Element Registry as all other elements. Module 6 is a UI/workflow grouping (a distinct collection experience triggered by risk), not a separate data store. The `collection_module` field on each element indicates which module's UI collects it.

---

## 3. Data Element Registry (Full Reference)

All data elements live in a single unified registry. The `collection_module` field indicates which UI module collects each element, but the data store is shared. Elements are organized below by collection module for readability.

### Module 1: User Profile Creation

| Element ID | Display Name | Type | Validity | Warning | Grace | Degradation |
|---|---|---|---|---|---|---|
| `mobile_number` | Mobile Number | phone | Permanent | N/A | N/A | N/A |
| `mobile_number_verified` | Mobile Number Verified | boolean (system) | Permanent (re-verified on number change) | N/A | N/A | hard_lock |
| `email` | Email Address | email | Permanent (user-changeable) | N/A | N/A | N/A |
| `first_name` | First Name | string | Permanent (user-changeable) | N/A | N/A | N/A |
| `middle_names` | Middle Name(s) | array | Permanent (user-changeable) | N/A | N/A | N/A |
| `last_name` | Last Name | string | Permanent (user-changeable) | N/A | N/A | N/A |
| `profile_picture_url` | Profile Picture | image | Permanent (user-changeable) | N/A | N/A | N/A |
| `mpin` | MPIN | credential | Permanent (user-changeable) | N/A | N/A | N/A |
| `biometric_preference` | Biometric Login | boolean | Permanent (user-changeable) | N/A | N/A | N/A |
| `phone_country_code` | Country Code | derived | Permanent | N/A | N/A | N/A |

### Module 3: Identity & Verification

| Element ID | Display Name | Type | Validity | Warning | Grace | Degradation |
|---|---|---|---|---|---|---|
| `id_document_type` | ID Document Type | enum | Tied to document | — | — | — |
| `id_document_scan` | ID Document Scan | image | Until document expiry | 30 days | 60 days | hard_lock |
| `id_document_number` | ID Number | string | Until document expiry | 30 days | 60 days | hard_lock |
| `id_document_nationality` | Nationalities | array | Until document expiry | 30 days | 60 days | — |
| `id_document_expiry` | Document Expiry Date | date | Source of validity | — | — | — |
| `id_document_issue_date` | Document Issue Date | date | Until document expiry | — | — | — |
| `id_document_name` | Full Name (from ID) | string | Until document expiry | — | — | — |
| `id_document_dob` | Date of Birth (from ID) | date | Permanent | — | — | — |
| `id_document_gender` | Gender (from ID) | enum | Until document expiry | — | — | — |
| `id_document_city_of_birth` | City of Birth | string | Until document expiry | — | — | — |
| `id_document_country_of_birth` | Country of Birth | string | Until document expiry | — | — | — |
| `liveness_check` | Biometric Liveness | verification | Until document expiry | 30 days | 60 days | hard_lock |
| `additional_id_document` | Additional ID Document | document | Until expiry | 30 days | 60 days | hard_lock |

### Module 4: KYC Profile Questions

| Element ID | Display Name | Type | Validity | Warning | Grace | Degradation |
|---|---|---|---|---|---|---|
| `residential_address` | Residential Address | structured (see below) | 365 days | 30 days | 180 days | soft_lock |
| `proof_of_address` | Proof of Address Document | document | 90 days | 14 days | 30 days | soft_lock |
| `employment_status` | Employment Status | enum | 365 days | 30 days | 180 days | warn |
| `income_range` | Income Range | enum | 365 days | 30 days | 180 days | warn |
| `income_sources` | Income Sources | multi_enum | 365 days | 30 days | 180 days | warn |
| `tax_residency_countries` | Tax Residency Countries | array | 365 days | 30 days | 90 days | soft_lock |
| `tin_1` | TIN (Primary) | string | 365 days | 30 days | 90 days | soft_lock |
| `tin_2` | TIN (Second) | string | 365 days | 30 days | 90 days | soft_lock |
| `tin_3` | TIN (Third) | string | 365 days | 30 days | 90 days | soft_lock |
| `tin_4` | TIN (Fourth) | string | 365 days | 30 days | 90 days | soft_lock |
| `pep_declaration` | PEP Declaration | boolean | 365 days | 30 days | 90 days | soft_lock |
| `professional_investor` | Professional Investor Status | boolean | 365 days | 30 days | 90 days | soft_lock |
| `risk_tolerance` | Risk Tolerance | enum (low/medium/high) | 365 days | 30 days | 90 days | warn |
| `customer_risk_score` | Customer Risk Score | score | System-sourced (Oscilar) | — | — | — |

**`residential_address` structure** — mandatory sub-fields marked with *:

| Sub-field | Type | Required |
|---|---|---|
| `address_line_1`* | string | Yes |
| `address_line_2` | string | No |
| `city`* | string | Yes |
| `state_province` | string | No |
| `post_code` | string | No |
| `country`* | CountryCode (ISO 3166-1 alpha-2) | Yes |

### Module 5: Legal Agreements

| Element ID | Display Name | Type | Validity | Warning | Grace | Degradation |
|---|---|---|---|---|---|---|
| `agreement_{product}_tc` | Terms & Conditions | consent per product | Until T&C version changes | — | 30 days | soft_lock |
| `agreement_privacy_policy` | Privacy Policy | consent (shared) | Until version changes | — | 30 days | soft_lock |
| `agreement_key_facts` | Key Facts Statement | consent per product | Until version changes | — | 30 days | soft_lock |
| `agreement_shariah_commodity` | Shariah Commodity Purchase Agreement | consent | Until version changes | — | 30 days | soft_lock |

### Module 6: Enhanced Due Diligence

| Element ID | Display Name | Type | Validity | Warning | Grace | Degradation |
|---|---|---|---|---|---|---|
| `source_of_wealth` | Source of Wealth | enum + text | 365 days | 30 days | 90 days | hard_lock |
| `source_of_wealth_docs` | Source of Wealth Documents | document | 365 days | 30 days | 90 days | hard_lock |
| `source_of_funds` | Source of Funds | enum + text | 365 days | 30 days | 90 days | hard_lock |
| `purpose_of_product` | Purpose of Product | text | 365 days | 30 days | 90 days | hard_lock |
| `business_ownership` | Business Ownership Details | structured | 365 days | 30 days | 90 days | hard_lock |

---

## 4. Requirement Matrix

Legend: **R** = Required | **—** = Not Required | **C** = Conditional | **S** = System-sourced | **P** = Parent product prerequisite

| Data Element | PFM | Travel Agent | AED Current Acct | USD Current Acct | AED Savings | AED Debit Card | UAE Bill Payments | Wealth Mgmt |
|---|---|---|---|---|---|---|---|---|
| **Product Type** | Primary | Primary | Primary | Primary | Primary | Suppl. (→CA) | Suppl. (→CA) | Primary |
| | | | | | | | | |
| **Module 1: Profile** | | | | | | | | |
| `mobile_number` | R | R | R | R | R | P | P | R |
| `email` | R | R | — | — | — | — | — | — |
| `first_name` | R | R | — | — | — | — | — | — |
| `middle_names` | — | — | — | — | — | — | — | — |
| `last_name` | R | R | — | — | — | — | — | — |
| `profile_picture_url` | — | — | — | — | — | — | — | — |
| `mpin` | R | R | R | R | R | P | P | R |
| `biometric_preference` | — | — | — | — | — | — | — | — |
| | | | | | | | | |
| **Module 3: ID&V** | | | | | | | | |
| `id_document_type` | — | — | R | R | R | P | P | R |
| `id_document_scan` | — | — | R | R | R | P | P | R |
| `id_document_number` | — | — | R | R | R | P | P | R |
| `id_document_nationality` | — | — | R | R | R | P | P | R |
| `id_document_expiry` | — | — | R | R | R | P | P | R |
| `id_document_issue_date` | — | — | R | R | R | P | P | R |
| `id_document_name` | — | — | R | R | R | P | P | R |
| `id_document_dob` | — | — | R | R | R | P | P | R |
| `id_document_gender` | — | — | R | R | R | P | P | R |
| `id_document_city_of_birth` | — | — | R | R | R | P | P | R |
| `id_document_country_of_birth` | — | — | R | R | R | P | P | R |
| `liveness_check` | — | — | R | R | R | P | P | R |
| `additional_id_document` | — | — | — | — | — | — | — | C |
| | | | | | | | | |
| **Module 4: KYC** | | | | | | | | |
| `residential_address` | — | — | R | R | R | P | P | R |
| `proof_of_address` | — | — | — | R | — | — | — | C |
| `employment_status` | — | — | R | R | R | P | P | R |
| `income_range` | — | — | R | R | R | P | P | R |
| `income_sources` | — | — | R | R | R | P | P | R |
| `tax_residency_countries` | — | — | R | R | R | P | P | R |
| `tin_1` | — | — | R | R | R | P | P | R |
| `tin_2` | — | — | C | C | C | P | P | C |
| `tin_3` | — | — | C | C | C | P | P | C |
| `tin_4` | — | — | C | C | C | P | P | C |
| `pep_declaration` | — | — | R | R | R | P | P | R |
| `professional_investor` | — | — | — | — | — | — | — | R |
| `risk_tolerance` | — | — | — | — | — | — | — | R |
| `customer_risk_score` | — | — | S | S | S | P | P | S |
| | | | | | | | | |
| **Module 5: Legal Agreements** | | | | | | | | |
| `agreement_{product}_tc` | — | — | R | R | R | R | R | R |
| `agreement_privacy_policy` | R | R | R | R | R | R | R | R |
| `agreement_key_facts` | — | — | R | R | R | R | — | R |
| `agreement_shariah_commodity` | — | — | — | — | R | — | — | — |
| | | | | | | | | |
| **Module 6: EDD** | | | | | | | | |
| `source_of_wealth` | — | — | C | C | C | — | — | C |
| `source_of_wealth_docs` | — | — | C | C | C | — | — | C |
| `source_of_funds` | — | — | C | C | C | — | — | C |
| `purpose_of_product` | — | — | C | C | C | — | — | C |
| `business_ownership` | — | — | C | C | C | — | — | C |

### Conditional Rules

| Trigger Element | Condition | Requires | Mandatory |
|---|---|---|---|
| `id_document_nationality` | implies residency in geo not in product's `geo_availability` | EligibilityRule failure: block with geo ineligibility message | Yes (deferred, checked post-Module 3) |
| `id_document_nationality` | includes US | `tin_1` (SSN) | Yes |
| `tax_residency_countries` | includes UAE | `tin_1` = EID number (auto-populated from `id_document_number`) | Yes |
| `tax_residency_countries` | count > 1 | Additional `tin_n` per country | Yes |
| `pep_declaration` | == true | Module 6 (EDD) | Yes |
| `customer_risk_score` | >= high | Module 6 (EDD) | Yes |
| Product instance | geo == USA | `proof_of_address` | Yes |
| Product instance | specific products | `additional_id_document` | Per config |

---

## 5. Access Degradation & Rescreening

### Degradation Lifecycle

```
Valid → Approaching Expiry (warning) → Expired (grace period) → Degraded (action taken)
```

**Warning phase**: Driven by `warning_window` on each data element. Configurable number of days before expiry (e.g., 30 days before Emirates ID expires). Non-blocking notification prompting the user to update. Elements with no `warning_window` (e.g., permanent validity elements) skip this phase.

**Grace period**: Element has expired but user gets a buffer before action. Duration is per-element `grace_period` (from Data Element Registry).

### Degradation Actions

| Action | Behavior | Scope |
|---|---|---|
| **warn** | Banner/notification. Full functionality retained. | Product-specific |
| **soft_lock** | View balances and history. Cannot transact. | Products requiring the stale element |
| **hard_lock** | No access to affected products. Redirected to refresh flow. | Products requiring the expired element |

**PFM and Travel Agent are never affected by degradation** — they don't require KYC elements.

### Rescreening Flow

1. Engine computes the delta: what's missing or stale relative to what the user needs
2. Presents only the refresh steps: not the full onboarding, just specific screens for stale/missing elements
3. Modules are reusable: the same Module 3 (ID&V) screen used in initial onboarding is reused for ID refresh
4. On completion: element timestamps updated, degradation lifted, product access restored

---

## 6. Eligibility Engine

### Three Core Operations

#### `evaluateEligibility(user_id, product_instance_id) → EligibilityResult | OnboardingPlan`

Called when a user requests a product.

1. **Pre-qualification**: Check EligibilityRules against existing user profile
   - Any rule fails with available data? → Return failure (block/redirect/waitlist + message). No onboarding starts.
   - Rules referencing uncollected data? → Mark as deferred (checked after relevant module)
2. Check product type: Primary or Supplementary?
   - If Supplementary: verify parent product is active. If not, return parent product onboarding first.
3. Fetch product requirements from Requirement Matrix
4. Fetch user's current profile (all UserDataRecords)
5. For each required element:
   a. Does the user have it? Check UserDataRecord exists
   b. Is it still valid? Check expires_at > now
   c. Does the product have a freshness override? Apply stricter check if so
6. Apply conditional rules based on collected data
7. Return OnboardingPlan: ordered list of modules with specific elements to collect, plus any deferred eligibility rules to check post-module

**Dynamic re-evaluation**: The plan is not static. After each module completes, the engine re-evaluates because:
- New data may trigger conditional requirements (e.g., US passport detected in Module 3 → SSN added to Module 4)
- Deferred eligibility rules can now be checked (e.g., age verified from ID → under-18 blocked)
The frontend never caches the full plan — it always renders the next step from the engine's latest response.

#### `evaluateAccess(user_id) → AccessReport`

Called periodically (app launch, background job) to check if any active products have degraded.

1. Fetch all active products for user
2. For each product, check all required elements for validity
3. Return per-product status (active / warn / soft_lock / hard_lock) with specific elements causing each status

#### `evaluateRefresh(user_id, stale_elements[]) → OnboardingPlan`

Called when user responds to a degradation prompt. Same logic as evaluateEligibility but scoped to specific stale elements rather than a product.

### State Machine (Per User × Product)

```
            ┌──────────┐
            │  No User  │
            └─────┬─────┘
                  │ Profile Creation (Module 1)
            ┌─────▼─────┐
            │  Profile   │──── PFM, Travel Agent accessible
            │  Only      │
            └─────┬─────┘
                  │ User requests KYC product
            ┌─────▼─────┐
            │ Onboarding │──── evaluateEligibility() returns plan
            │ In Progress│──── Modules 3→4→5→6 (as needed)
            └─────┬─────┘     User can exit anytime, resume later
                  │ All requirements met
            ┌─────▼─────┐
            │  Product   │──── Full product access
            │  Active    │──── evaluateAccess() monitors ongoing
            └─────┬─────┘
                  │ Element expires / stale
            ┌─────▼─────┐
            │ Degraded   │──── warn → soft_lock → hard_lock
            │            │──── evaluateRefresh() when user acts
            └─────┬─────┘
                  │ User refreshes data
            ┌─────▼─────┐
            │  Product   │──── Restored
            │  Active    │
            └────────────┘
```

### Non-Blocking Journeys

- Being in an onboarding journey does not prevent access to the rest of the app
- Users can exit at any point — go back to PFM, Travel Agent, or any already-active product
- Progress is persisted server-side via UserDataRecord entries
- When they return, the engine re-evaluates and picks up where they left off

### API Surface

**Examples only, to be confirmed by Engineering Team**

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/v1/onboarding/eligibility?product={id}` | GET | Returns OnboardingPlan for a product |
| `/api/v1/onboarding/access` | GET | Returns AccessReport for current user |
| `/api/v1/onboarding/submit` | POST | Submits collected data for one or more elements; returns updated remaining plan |
| `/api/v1/onboarding/progress` | GET | Returns in-flight onboarding state (for resume) |

### Operations Hub Configuration

The Operations Hub admin panel manages all runtime configuration:

- **Product Catalog**: Add/edit product archetypes, instances, geo availability, providers, primary/supplementary classification
- **Data Element Registry**: Add/edit elements, validity periods, grace periods, degradation rules (unified registry — all modules)
- **Requirement Matrix**: Map elements to products, set mandatory/conditional flags
- **Eligibility Rules**: Define pre-qualification gates per product (age, residency, license requirements, etc.)
- **Conditional Rules**: Define trigger conditions for dynamic requirements
- **Legal Agreement versions**: Manage T&C documents per product, trigger re-acceptance on version change

All configuration changes take effect at next eligibility evaluation — no app deploy needed.

---

## 7. Worked Examples

### New user wants AED Current Account

```
Profile Creation → [taps "Open AED Account"] → evaluateEligibility()
→ Plan: [Module 3: Emirates ID + liveness, Module 4: address + employment + tax + PEP,
         Module 5: AED CA T&C + privacy policy + key facts]
→ User completes each module → Account activated
```

### Existing AED Current Account user wants AED Savings Account

```
[taps "Open Savings Account"] → evaluateEligibility() runs diff
→ Module 1: satisfied ✓
→ Module 3 (ID&V): all elements satisfied ✓
→ Module 4 (KYC): all elements satisfied ✓ (identical requirements to AED CA)
→ Module 5: agreement_savings_tc (new) + agreement_shariah_commodity (new — Savings only)
→ Plan: [Module 5: Savings T&C + Shariah Commodity Purchase Agreement]
→ User accepts both agreements on a single screen → Savings Account activated instantly
```

No document scanning, no KYC re-collection. The only journey is legal agreements for the new product. If any KYC element has since expired (e.g., residential address > 365 days old), the engine surfaces only that stale element before proceeding to agreements.

### Existing AED CA user wants USD Account

```
[taps "Open USD Account"] → evaluateEligibility() runs diff
→ ID&V: already done ✓ → KYC: proof of address needed (USA requirement)
→ Plan: [Module 4: proof_of_address only, Module 5: USD CA T&C + key facts]
→ User uploads proof of address, accepts terms → Account activated
```

### User only wants PFM

```
Profile Creation → done. Full PFM access. No further onboarding.
```

### Emirates ID expires

```
evaluateAccess() detects id_document_expiry has passed
→ Within 60-day grace: soft_lock on banking products, warning banner
→ PFM and Travel Agent unaffected
→ User taps "Update ID" → evaluateRefresh() → Module 3 (ID&V with new ID)
→ Access restored
```

### US passport detected during onboarding

```
User in Module 3 → scans US passport → nationality = US detected
→ Engine re-evaluates → conditional rule fires: tin_1 (SSN) now mandatory
→ Updated plan: Module 4 now includes SSN field alongside other KYC
→ User enters SSN as TIN 1 → continues normally
```

### Deferred eligibility rule — under-18 blocked

```
User requests AED Current Account → evaluateEligibility()
→ EligibilityRule: age >= 18 — but id_document_dob not yet collected → deferred
→ Plan: [Module 3: Emirates ID + liveness, Module 4: KYC, Module 5: legal]
→ User completes Module 3 → engine re-evaluates → dob reveals age = 16
→ Deferred rule fails → journey stops: "You must be 18+ to open a current account"
→ User retains PFM/Travel Agent access
```

### PEP declared → EDD triggered

```
User in Module 4 → declares PEP = true
→ Engine re-evaluates → conditional rule fires: Module 6 (EDD) added to plan
→ After Module 5 (legal agreements), user enters source of wealth + purpose of product
→ Screening result from Oscilar confirms risk level → account activated (or manual review)
```