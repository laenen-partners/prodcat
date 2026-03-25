package prodcat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// Client provides product/ruleset management, ruleset resolution,
// and catalogue import backed by a Store.
type Client struct {
	store  Store
	logger *slog.Logger
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithLogger sets the logger for the client.
func WithLogger(l *slog.Logger) ClientOption {
	return func(c *Client) { c.logger = l }
}

// NewClient creates a new client backed by the given store.
func NewClient(store Store, opts ...ClientOption) *Client {
	c := &Client{store: store, logger: slog.Default()}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Store returns the underlying store.
func (c *Client) Store() Store {
	return c.store
}

// ─── Product Management ───

// RegisterProduct creates a new product. Fails if the ProductID already exists.
func (c *Client) RegisterProduct(ctx context.Context, p Product, prov Provenance) (*Product, error) {
	if c.store == nil {
		return nil, ErrNoStore
	}
	if p.ProductID == "" {
		return nil, fmt.Errorf("product_id is required: %w", ErrValidation)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("name is required: %w", ErrValidation)
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	if err := c.store.CreateProduct(ctx, p, prov); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) GetProduct(ctx context.Context, productID string) (*Product, error) {
	if c.store == nil {
		return nil, ErrNoStore
	}
	return c.store.GetProduct(ctx, productID)
}

func (c *Client) ListProducts(ctx context.Context, filter ListFilter) ([]Product, error) {
	if c.store == nil {
		return nil, ErrNoStore
	}
	return c.store.ListProducts(ctx, filter)
}

func (c *Client) UpdateProduct(ctx context.Context, p Product, prov Provenance) (*Product, error) {
	if c.store == nil {
		return nil, ErrNoStore
	}
	p.UpdatedAt = time.Now().UTC()
	if err := c.store.PutProduct(ctx, p, prov); err != nil {
		return nil, err
	}
	return &p, nil
}

// DisableProduct disables a product with a reason.
func (c *Client) DisableProduct(ctx context.Context, productID string, reason DisabledReason, prov Provenance) (*Product, error) {
	if c.store == nil {
		return nil, ErrNoStore
	}
	p, err := c.store.GetProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	p.Disabled = true
	p.DisabledReason = reason
	p.UpdatedAt = time.Now().UTC()
	if err := c.store.PutProduct(ctx, *p, prov); err != nil {
		return nil, fmt.Errorf("put product: %w", err)
	}
	return p, nil
}

// EnableProduct re-enables a disabled product.
func (c *Client) EnableProduct(ctx context.Context, productID string, prov Provenance) (*Product, error) {
	if c.store == nil {
		return nil, ErrNoStore
	}
	p, err := c.store.GetProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	p.Disabled = false
	p.DisabledReason = ""
	p.UpdatedAt = time.Now().UTC()
	if err := c.store.PutProduct(ctx, *p, prov); err != nil {
		return nil, fmt.Errorf("put product: %w", err)
	}
	return p, nil
}

// ─── Ruleset Management ───

// CreateRuleset creates a new ruleset. Fails if a ruleset with the same ID already exists.
func (c *Client) CreateRuleset(ctx context.Context, r Ruleset, prov Provenance) (*Ruleset, error) {
	if c.store == nil {
		return nil, ErrNoStore
	}
	if r.Name == "" {
		return nil, fmt.Errorf("ruleset name is required: %w", ErrValidation)
	}
	if len(r.Content) == 0 {
		return nil, fmt.Errorf("ruleset content is required: %w", ErrValidation)
	}
	if err := ValidateRulesetContent(r.Content); err != nil {
		return nil, fmt.Errorf("invalid ruleset content: %w", err)
	}
	now := time.Now().UTC()
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	r.CreatedAt = now
	r.UpdatedAt = now
	if err := c.store.CreateRuleset(ctx, r, prov); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) GetRuleset(ctx context.Context, id string) (*Ruleset, error) {
	if c.store == nil {
		return nil, ErrNoStore
	}
	return c.store.GetRuleset(ctx, id)
}

func (c *Client) ListRulesets(ctx context.Context) ([]Ruleset, error) {
	if c.store == nil {
		return nil, ErrNoStore
	}
	return c.store.ListRulesets(ctx)
}

// DisableRuleset soft-deletes a ruleset by marking it disabled.
func (c *Client) DisableRuleset(ctx context.Context, id string, reason DisabledReason, prov Provenance) (*Ruleset, error) {
	if c.store == nil {
		return nil, ErrNoStore
	}
	rs, err := c.store.GetRuleset(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get ruleset: %w", err)
	}
	rs.Disabled = true
	rs.DisabledReason = reason
	rs.UpdatedAt = time.Now().UTC()
	if err := c.store.PutRuleset(ctx, *rs, prov); err != nil {
		return nil, fmt.Errorf("put ruleset: %w", err)
	}
	return rs, nil
}

// EnableRuleset re-enables a disabled ruleset.
func (c *Client) EnableRuleset(ctx context.Context, id string, prov Provenance) (*Ruleset, error) {
	if c.store == nil {
		return nil, ErrNoStore
	}
	rs, err := c.store.GetRuleset(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get ruleset: %w", err)
	}
	rs.Disabled = false
	rs.DisabledReason = ""
	rs.UpdatedAt = time.Now().UTC()
	if err := c.store.PutRuleset(ctx, *rs, prov); err != nil {
		return nil, fmt.Errorf("put ruleset: %w", err)
	}
	return rs, nil
}

// ─── Convenience: Ruleset Management on Products ───

// AddRuleset appends a ruleset ID to a product's BaseRulesetIDs if not already present.
// Validates the ruleset exists and is not disabled. The store enforces this atomically
// via preconditions.
func (c *Client) AddRuleset(ctx context.Context, productID, rulesetID string, prov Provenance) (*Product, error) {
	p, err := c.GetProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}

	// Check if already linked.
	for _, id := range p.BaseRulesetIDs {
		if id == rulesetID {
			return p, nil
		}
	}

	// Client-side validation — the store also enforces this atomically via preconditions.
	rs, err := c.GetRuleset(ctx, rulesetID)
	if err != nil {
		return nil, fmt.Errorf("get ruleset: %w", err)
	}
	if rs.Disabled {
		return nil, fmt.Errorf("ruleset %q is disabled (%s): %w", rulesetID, rs.DisabledReason, ErrRulesetDisabled)
	}

	p.BaseRulesetIDs = append(p.BaseRulesetIDs, rulesetID)
	return c.UpdateProduct(ctx, *p, prov)
}

// RemoveRuleset removes a ruleset ID from a product's BaseRulesetIDs.
func (c *Client) RemoveRuleset(ctx context.Context, productID, rulesetID string, prov Provenance) (*Product, error) {
	p, err := c.GetProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	ids := make([]string, 0, len(p.BaseRulesetIDs))
	for _, id := range p.BaseRulesetIDs {
		if id != rulesetID {
			ids = append(ids, id)
		}
	}
	p.BaseRulesetIDs = ids
	return c.UpdateProduct(ctx, *p, prov)
}

// SetProductRuleset sets the product's inline ruleset content.
func (c *Client) SetProductRuleset(ctx context.Context, productID string, content []byte, prov Provenance) (*Product, error) {
	p, err := c.GetProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	p.Ruleset = content
	return c.UpdateProduct(ctx, *p, prov)
}

// ─── Ruleset Resolution ───

// ResolveRuleset merges base rulesets + product-specific ruleset into a single
// YAML document ready for evalengine. Disabled rulesets are skipped with a warning.
func (c *Client) ResolveRuleset(ctx context.Context, productID string) (*ResolvedRuleset, error) {
	product, err := c.store.GetProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}

	var merged rulesetYAML
	var layers []RulesetLayer

	for _, baseID := range product.BaseRulesetIDs {
		base, err := c.store.GetRuleset(ctx, baseID)
		if err != nil {
			return nil, fmt.Errorf("get base ruleset %s: %w", baseID, err)
		}
		if base.Disabled {
			c.logger.WarnContext(ctx, "skipping disabled base ruleset",
				"ruleset_id", baseID,
				"disabled_reason", base.DisabledReason,
				"product_id", productID,
			)
			continue
		}
		var baseRuleset rulesetYAML
		if err := yaml.Unmarshal(base.Content, &baseRuleset); err != nil {
			return nil, fmt.Errorf("parse base ruleset %s: %w", baseID, err)
		}
		merged.Evaluations = append(merged.Evaluations, baseRuleset.Evaluations...)
		layers = append(layers, RulesetLayer{Source: "base", SourceID: baseID})
	}

	if len(product.Ruleset) > 0 {
		var productRuleset rulesetYAML
		if err := yaml.Unmarshal(product.Ruleset, &productRuleset); err != nil {
			return nil, fmt.Errorf("parse product ruleset: %w", err)
		}
		merged.Evaluations = append(merged.Evaluations, productRuleset.Evaluations...)
		layers = append(layers, RulesetLayer{Source: "product", SourceID: productID})
	}

	mergedBytes, err := yaml.Marshal(&merged)
	if err != nil {
		return nil, fmt.Errorf("marshal merged ruleset: %w", err)
	}

	return &ResolvedRuleset{
		ProductID: productID,
		Merged:    mergedBytes,
		Layers:    layers,
	}, nil
}

// ─── Import ───

// upsertProduct is used by Import for idempotent product creation.
// It creates if not exists, updates if it does. Unlike RegisterProduct,
// it does not fail on duplicates.
func (c *Client) upsertProduct(ctx context.Context, p Product, prov Provenance) error {
	now := time.Now().UTC()
	p.UpdatedAt = now
	existing, err := c.store.GetProduct(ctx, p.ProductID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if existing == nil {
		p.CreatedAt = now
	} else {
		p.CreatedAt = existing.CreatedAt
	}
	return c.store.PutProduct(ctx, p, prov)
}

// ─── Internal: YAML types for ruleset round-tripping ───

type rulesetYAML struct {
	Evaluations []evaluationYAML `yaml:"evaluations"`
}

type evaluationYAML struct {
	Name               string             `yaml:"name"`
	Description        string             `yaml:"description,omitempty"`
	Expression         string             `yaml:"expression"`
	Reads              []string           `yaml:"reads,omitempty"`
	Writes             string             `yaml:"writes"`
	ResolutionWorkflow string             `yaml:"resolution_workflow,omitempty"`
	Resolution         string             `yaml:"resolution,omitempty"`
	Severity           string             `yaml:"severity,omitempty"`
	Category           string             `yaml:"category,omitempty"`
	CacheTTL           string             `yaml:"cache_ttl,omitempty"`
	FailureMode        string             `yaml:"failure_mode,omitempty"`
	Preconditions      []preconditionYAML `yaml:"preconditions,omitempty"`
}

type preconditionYAML struct {
	Expression  string `yaml:"expression"`
	Description string `yaml:"description,omitempty"`
}
