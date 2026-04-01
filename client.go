package prodcat

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	es "github.com/laenen-partners/entitystore"
	"gopkg.in/yaml.v3"
)

// Client provides product/ruleset management, ruleset resolution,
// and catalogue import backed by an entitystore.
type Client struct {
	es     es.EntityStorer
	logger *slog.Logger
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithLogger sets the logger for the client.
func WithLogger(l *slog.Logger) ClientOption {
	return func(c *Client) { c.logger = l }
}

// NewClient creates a new client backed by the given entitystore.
func NewClient(e es.EntityStorer, opts ...ClientOption) *Client {
	c := &Client{es: e, logger: slog.Default()}
	for _, o := range opts {
		o(c)
	}
	return c
}

// EntityStore returns the underlying entitystore.
func (c *Client) EntityStore() es.EntityStorer {
	return c.es
}

// ─── Product Management ───

func (c *Client) RegisterProduct(ctx context.Context, p Product) (*Product, error) {
	if p.ProductID == "" {
		return nil, fmt.Errorf("product_id is required: %w", ErrValidation)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("name is required: %w", ErrValidation)
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	actor := actorFromContext(ctx)
	if err := c.createProduct(ctx, p, &ProductCreatedEvent{
		ProductID: p.ProductID, Actor: actor, Name: p.Name,
		Description: p.Description, Tags: p.Tags,
		CurrencyCode: p.CurrencyCode, BaseRulesetIDs: p.BaseRulesetIDs,
	}); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) GetProduct(ctx context.Context, productID string) (*Product, error) {
	return c.getProduct(ctx, productID)
}

func (c *Client) ListProducts(ctx context.Context, filter ListFilter) ([]Product, error) {
	return c.listProducts(ctx, filter)
}

func (c *Client) UpdateProduct(ctx context.Context, p Product) (*Product, error) {
	p.UpdatedAt = time.Now().UTC()
	actor := actorFromContext(ctx)
	if err := c.putProduct(ctx, p, &ProductUpdatedEvent{
		ProductID: p.ProductID, Actor: actor, Name: p.Name,
		Description: p.Description, Tags: p.Tags,
		CurrencyCode: p.CurrencyCode, BaseRulesetIDs: p.BaseRulesetIDs,
	}); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) DisableProduct(ctx context.Context, productID string, reason DisabledReason) (*Product, error) {
	p, err := c.getProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	p.Disabled = true
	p.DisabledReason = reason
	p.UpdatedAt = time.Now().UTC()
	actor := actorFromContext(ctx)
	if err := c.putProduct(ctx, *p, &ProductDisabledEvent{
		ProductID: productID, Actor: actor, Reason: string(reason), Name: p.Name,
	}); err != nil {
		return nil, fmt.Errorf("put product: %w", err)
	}
	return p, nil
}

func (c *Client) EnableProduct(ctx context.Context, productID string) (*Product, error) {
	p, err := c.getProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	p.Disabled = false
	p.DisabledReason = ""
	p.UpdatedAt = time.Now().UTC()
	actor := actorFromContext(ctx)
	if err := c.putProduct(ctx, *p, &ProductEnabledEvent{
		ProductID: productID, Actor: actor, Name: p.Name,
	}); err != nil {
		return nil, fmt.Errorf("put product: %w", err)
	}
	return p, nil
}

// DeleteProduct soft-deletes a product.
func (c *Client) DeleteProduct(ctx context.Context, productID string) error {
	p, err := c.getProduct(ctx, productID)
	if err != nil {
		return fmt.Errorf("get product: %w", err)
	}
	actor := actorFromContext(ctx)
	p.Disabled = true
	p.DisabledReason = DisabledReasonDeleted
	p.UpdatedAt = time.Now().UTC()
	if err := c.putProduct(ctx, *p, &ProductDeletedEvent{
		ProductID: productID, Actor: actor, Name: p.Name,
	}); err != nil {
		return fmt.Errorf("mark deleted: %w", err)
	}
	return c.deleteProduct(ctx, productID)
}

// ─── Ruleset Management ───

func (c *Client) CreateRuleset(ctx context.Context, r Ruleset) (*Ruleset, error) {
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
	r.ContentHash = ContentHashOf(r.Content)
	r.CreatedAt = now
	r.UpdatedAt = now
	actor := actorFromContext(ctx)
	if err := c.createRuleset(ctx, r, &RulesetCreatedEvent{
		RulesetID: r.ID, Actor: actor, Name: r.Name,
		Description: r.Description, Version: r.Version,
	}); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) GetRuleset(ctx context.Context, id string) (*Ruleset, error) {
	return c.getRuleset(ctx, id)
}

func (c *Client) ListRulesets(ctx context.Context) ([]Ruleset, error) {
	return c.listRulesets(ctx)
}

func (c *Client) DisableRuleset(ctx context.Context, id string, reason DisabledReason) (*Ruleset, error) {
	rs, err := c.getRuleset(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get ruleset: %w", err)
	}
	rs.Disabled = true
	rs.DisabledReason = reason
	rs.UpdatedAt = time.Now().UTC()
	actor := actorFromContext(ctx)
	if err := c.putRuleset(ctx, *rs, &RulesetDisabledEvent{
		RulesetID: id, Actor: actor, Reason: string(reason), Name: rs.Name,
	}); err != nil {
		return nil, fmt.Errorf("put ruleset: %w", err)
	}
	return rs, nil
}

func (c *Client) EnableRuleset(ctx context.Context, id string) (*Ruleset, error) {
	rs, err := c.getRuleset(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get ruleset: %w", err)
	}
	rs.Disabled = false
	rs.DisabledReason = ""
	rs.UpdatedAt = time.Now().UTC()
	actor := actorFromContext(ctx)
	if err := c.putRuleset(ctx, *rs, &RulesetEnabledEvent{
		RulesetID: id, Actor: actor, Name: rs.Name,
	}); err != nil {
		return nil, fmt.Errorf("put ruleset: %w", err)
	}
	return rs, nil
}

// DeleteRuleset soft-deletes a ruleset.
func (c *Client) DeleteRuleset(ctx context.Context, id string) error {
	rs, err := c.getRuleset(ctx, id)
	if err != nil {
		return fmt.Errorf("get ruleset: %w", err)
	}
	actor := actorFromContext(ctx)
	rs.Disabled = true
	rs.DisabledReason = DisabledReasonDeleted
	rs.UpdatedAt = time.Now().UTC()
	if err := c.putRuleset(ctx, *rs, &RulesetDeletedEvent{
		RulesetID: id, Actor: actor, Name: rs.Name,
	}); err != nil {
		return fmt.Errorf("mark deleted: %w", err)
	}
	return c.deleteRuleset(ctx, id)
}

// ─── Convenience: Ruleset Management on Products ───

func (c *Client) AddRuleset(ctx context.Context, productID, rulesetID string) (*Product, error) {
	p, err := c.GetProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	for _, id := range p.BaseRulesetIDs {
		if id == rulesetID {
			return p, nil
		}
	}
	rs, err := c.GetRuleset(ctx, rulesetID)
	if err != nil {
		return nil, fmt.Errorf("get ruleset: %w", err)
	}
	if rs.Disabled {
		return nil, fmt.Errorf("ruleset %q is disabled (%s): %w", rulesetID, rs.DisabledReason, ErrRulesetDisabled)
	}

	p.BaseRulesetIDs = append(p.BaseRulesetIDs, rulesetID)
	p.UpdatedAt = time.Now().UTC()
	actor := actorFromContext(ctx)
	if err := c.putProduct(ctx, *p, &RulesetLinkedToProductEvent{
		ProductID: productID, RulesetID: rulesetID, Actor: actor,
		ProductName: p.Name, RulesetName: rs.Name,
	}); err != nil {
		return nil, err
	}
	if err := c.linkRulesetToProduct(ctx, productID, rulesetID); err != nil {
		c.logger.WarnContext(ctx, "failed to create has_ruleset relation",
			"product_id", productID, "ruleset_id", rulesetID, "error", err)
	}
	return p, nil
}

func (c *Client) RemoveRuleset(ctx context.Context, productID, rulesetID string) (*Product, error) {
	p, err := c.GetProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	rs, _ := c.GetRuleset(ctx, rulesetID) // best-effort for event context
	rulesetName := ""
	if rs != nil {
		rulesetName = rs.Name
	}

	ids := make([]string, 0, len(p.BaseRulesetIDs))
	for _, id := range p.BaseRulesetIDs {
		if id != rulesetID {
			ids = append(ids, id)
		}
	}
	p.BaseRulesetIDs = ids
	p.UpdatedAt = time.Now().UTC()
	actor := actorFromContext(ctx)
	if err := c.putProduct(ctx, *p, &RulesetUnlinkedFromProductEvent{
		ProductID: productID, RulesetID: rulesetID, Actor: actor,
		ProductName: p.Name, RulesetName: rulesetName,
	}); err != nil {
		return nil, err
	}
	if err := c.unlinkRulesetFromProduct(ctx, productID, rulesetID); err != nil {
		c.logger.WarnContext(ctx, "failed to delete has_ruleset relation",
			"product_id", productID, "ruleset_id", rulesetID, "error", err)
	}
	return p, nil
}

func (c *Client) SetProductRuleset(ctx context.Context, productID string, content []byte) (*Product, error) {
	p, err := c.GetProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	p.Ruleset = content
	return c.UpdateProduct(ctx, *p)
}

// ─── Routing ───

// SetRouting replaces the entire routing map for a product.
func (c *Client) SetRouting(ctx context.Context, productID string, routing map[string]string) (*Product, error) {
	p, err := c.GetProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	p.Routing = routing
	return c.UpdateProduct(ctx, *p)
}

// SetRoute sets a single capability→provider route on a product.
func (c *Client) SetRoute(ctx context.Context, productID, capability, providerID string) (*Product, error) {
	p, err := c.GetProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	if p.Routing == nil {
		p.Routing = make(map[string]string)
	}
	p.Routing[capability] = providerID
	return c.UpdateProduct(ctx, *p)
}

// RemoveRoute removes a single capability route from a product.
func (c *Client) RemoveRoute(ctx context.Context, productID, capability string) (*Product, error) {
	p, err := c.GetProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	if p.Routing != nil {
		delete(p.Routing, capability)
	}
	return c.UpdateProduct(ctx, *p)
}

// ─── Ruleset Resolution ───

func (c *Client) ResolveRuleset(ctx context.Context, productID string) (*ResolvedRuleset, error) {
	product, err := c.getProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}

	var merged rulesetYAML
	var layers []RulesetLayer

	for _, baseID := range product.BaseRulesetIDs {
		base, err := c.getRuleset(ctx, baseID)
		if err != nil {
			return nil, fmt.Errorf("get base ruleset %s: %w", baseID, err)
		}
		if base.Disabled {
			c.logger.WarnContext(ctx, "skipping disabled base ruleset",
				"ruleset_id", baseID, "disabled_reason", base.DisabledReason,
				"product_id", productID)
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
