-- migrate:up

-- ─── Enums ───

CREATE TYPE product_family AS ENUM (
    'casa', 'lending', 'cards', 'payments',
    'investments', 'insurance', 'pfm', 'value_added'
);

CREATE TYPE product_status AS ENUM (
    'draft', 'active', 'suspended', 'deprecated', 'retired'
);

CREATE TYPE product_type AS ENUM (
    'primary', 'supplementary'
);

CREATE TYPE availability_mode AS ENUM (
    'specific_countries', 'global', 'global_except'
);

CREATE TYPE entity_type AS ENUM (
    'individual', 'business'
);

CREATE TYPE subscription_status AS ENUM (
    'incomplete', 'active', 'past_due', 'canceled'
);

CREATE TYPE party_role AS ENUM (
    'primary_holder', 'joint_holder', 'authorized_signatory',
    'director', 'ubo', 'secretary', 'poa', 'guardian'
);

CREATE TYPE signing_rule AS ENUM (
    'any_one', 'any_n', 'all'
);

CREATE TYPE capability_type AS ENUM (
    'view', 'domestic_transfers', 'international_transfers',
    'card_payments', 'atm', 'receive', 'bill_payments',
    'fx', 'standing_orders', 'custom'
);

CREATE TYPE capability_status AS ENUM (
    'active', 'disabled', 'pending'
);

CREATE TYPE disabled_reason AS ENUM (
    'requirements_not_met', 'expired_data', 'failed_evaluation',
    'regulatory_hold', 'fraud_suspicion', 'customer_requested',
    'operations', 'parent_disabled', 'party_incomplete', 'party_removed'
);

-- ─── Product Hierarchy ───

CREATE TABLE families (
    id               TEXT PRIMARY KEY,
    family           product_family NOT NULL,
    name             JSONB NOT NULL DEFAULT '{}',
    description      JSONB NOT NULL DEFAULT '{}',
    ruleset          BYTEA,
    base_ruleset_ids TEXT[] NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE archetypes (
    id               TEXT PRIMARY KEY,
    family_id        TEXT NOT NULL REFERENCES families(id),
    name             JSONB NOT NULL DEFAULT '{}',
    description      JSONB NOT NULL DEFAULT '{}',
    ruleset          BYTEA,
    base_ruleset_ids TEXT[] NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE products (
    id                 TEXT PRIMARY KEY,
    archetype_id       TEXT NOT NULL REFERENCES archetypes(id),
    name               JSONB NOT NULL DEFAULT '{}',
    description        JSONB NOT NULL DEFAULT '{}',
    tagline            JSONB NOT NULL DEFAULT '{}',
    status             product_status NOT NULL DEFAULT 'draft',
    product_type       product_type NOT NULL DEFAULT 'primary',
    currency_code      TEXT NOT NULL DEFAULT '',
    parent_product_id  TEXT REFERENCES products(id),

    -- Provider (flattened — not a separate table for simplicity)
    provider_id        TEXT NOT NULL DEFAULT '',
    provider_name      TEXT NOT NULL DEFAULT '',
    regulator          TEXT NOT NULL DEFAULT '',
    license_number     TEXT NOT NULL DEFAULT '',
    regulatory_country TEXT NOT NULL DEFAULT '',

    -- Compliance
    sharia_compliant   BOOLEAN NOT NULL DEFAULT false,

    -- Eligibility
    availability_mode  availability_mode NOT NULL DEFAULT 'global',
    country_codes      TEXT[] NOT NULL DEFAULT '{}',
    ruleset            BYTEA,
    base_ruleset_ids   TEXT[] NOT NULL DEFAULT '{}',

    -- Effective period
    effective_from     TIMESTAMPTZ,
    effective_to       TIMESTAMPTZ,

    -- Metadata
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by         TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_products_archetype ON products(archetype_id);
CREATE INDEX idx_products_status ON products(status);

-- ─── Base Rulesets ───

CREATE TABLE base_rulesets (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    content     BYTEA NOT NULL,
    version     TEXT NOT NULL DEFAULT '1',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ─── Legal Agreements ───

CREATE TABLE legal_agreements (
    id             TEXT PRIMARY KEY,
    product_id     TEXT NOT NULL REFERENCES products(id),
    agreement_type TEXT NOT NULL,
    title          JSONB NOT NULL DEFAULT '{}',
    version        TEXT NOT NULL DEFAULT '1',
    document_ref   TEXT NOT NULL DEFAULT '',
    shared         BOOLEAN NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_legal_agreements_product ON legal_agreements(product_id);

-- ─── Acceptable ID Configs ───

CREATE TABLE acceptable_id_configs (
    id                   TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    product_id           TEXT NOT NULL REFERENCES products(id),
    id_type_id           TEXT NOT NULL,
    is_category_wildcard BOOLEAN NOT NULL DEFAULT false,
    issuing_geo_filter   TEXT[] NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_acceptable_ids_product ON acceptable_id_configs(product_id);

-- ─── Customer Segments ───

CREATE TABLE customer_segments (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    product_id   TEXT NOT NULL REFERENCES products(id),
    segment_id   TEXT NOT NULL,
    customer_type TEXT NOT NULL
);

CREATE INDEX idx_customer_segments_product ON customer_segments(product_id);

-- ─── Subscriptions ───

CREATE TABLE subscriptions (
    id                      TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    product_id              TEXT NOT NULL REFERENCES products(id),
    entity_id               TEXT NOT NULL,
    entity_type             entity_type NOT NULL,
    status                  subscription_status NOT NULL DEFAULT 'incomplete',
    signing_rule            signing_rule NOT NULL DEFAULT 'any_one',
    required_count          INT NOT NULL DEFAULT 1,
    parent_subscription_id  TEXT REFERENCES subscriptions(id),
    external_ref            TEXT,

    -- Disabled state
    disabled                BOOLEAN NOT NULL DEFAULT false,
    disabled_reason         disabled_reason,
    disabled_message        TEXT NOT NULL DEFAULT '',
    disabled_at             TIMESTAMPTZ,

    -- Eval state (stored as JSONB for flexibility)
    eval_state              JSONB,

    -- Lifecycle
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    activated_at            TIMESTAMPTZ,
    canceled_at             TIMESTAMPTZ
);

CREATE INDEX idx_subscriptions_entity ON subscriptions(entity_id);
CREATE INDEX idx_subscriptions_product ON subscriptions(product_id);
CREATE INDEX idx_subscriptions_status ON subscriptions(status);

-- ─── Parties ───

CREATE TABLE parties (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    subscription_id  TEXT NOT NULL REFERENCES subscriptions(id),
    customer_id      TEXT NOT NULL,
    role             party_role NOT NULL,
    requirements_met BOOLEAN NOT NULL DEFAULT false,

    -- Disabled state
    disabled         BOOLEAN NOT NULL DEFAULT false,
    disabled_reason  disabled_reason,
    disabled_message TEXT NOT NULL DEFAULT '',
    disabled_at      TIMESTAMPTZ,

    -- Eval state
    eval_state       JSONB,

    added_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    removed_at       TIMESTAMPTZ
);

CREATE INDEX idx_parties_subscription ON parties(subscription_id);
CREATE INDEX idx_parties_customer ON parties(customer_id);

-- ─── Capabilities ───

CREATE TABLE capabilities (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    subscription_id  TEXT NOT NULL REFERENCES subscriptions(id),
    capability_type  capability_type NOT NULL,
    status           capability_status NOT NULL DEFAULT 'pending',

    -- Disabled state
    disabled         BOOLEAN NOT NULL DEFAULT false,
    disabled_reason  disabled_reason,
    disabled_message TEXT NOT NULL DEFAULT '',
    disabled_at      TIMESTAMPTZ,

    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_capabilities_subscription ON capabilities(subscription_id);

-- migrate:down

DROP TABLE IF EXISTS capabilities;
DROP TABLE IF EXISTS parties;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS customer_segments;
DROP TABLE IF EXISTS acceptable_id_configs;
DROP TABLE IF EXISTS legal_agreements;
DROP TABLE IF EXISTS base_rulesets;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS archetypes;
DROP TABLE IF EXISTS families;

DROP TYPE IF EXISTS disabled_reason;
DROP TYPE IF EXISTS capability_status;
DROP TYPE IF EXISTS capability_type;
DROP TYPE IF EXISTS signing_rule;
DROP TYPE IF EXISTS party_role;
DROP TYPE IF EXISTS subscription_status;
DROP TYPE IF EXISTS entity_type;
DROP TYPE IF EXISTS availability_mode;
DROP TYPE IF EXISTS product_type;
DROP TYPE IF EXISTS product_status;
DROP TYPE IF EXISTS product_family;
