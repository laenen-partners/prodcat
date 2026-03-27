package connectrpc

import (
	"context"
	"fmt"
	"net/http"

	connect "connectrpc.com/connect"

	"github.com/laenen-partners/prodcat"
	prodcatrpcv1 "github.com/laenen-partners/prodcat/connectrpc/gen/prodcat/rpc/v1"
	"github.com/laenen-partners/prodcat/connectrpc/gen/prodcat/rpc/v1/prodcatrpcv1connect"
)

// Client wraps the generated ConnectRPC clients and exposes domain-level methods.
// It satisfies the [github.com/laenen-partners/prodcat/ui.CatalogService] interface.
type Client struct {
	query   prodcatrpcv1connect.ProductCatalogQueryServiceClient
	command prodcatrpcv1connect.ProductCatalogCommandServiceClient
}

// NewClient creates a ConnectRPC client targeting baseURL.
func NewClient(baseURL string, opts ...connect.ClientOption) *Client {
	return &Client{
		query:   prodcatrpcv1connect.NewProductCatalogQueryServiceClient(http.DefaultClient, baseURL, opts...),
		command: prodcatrpcv1connect.NewProductCatalogCommandServiceClient(http.DefaultClient, baseURL, opts...),
	}
}

// NewClientFromHTTP creates a ConnectRPC client using the given http.Client.
// Useful for wiring to an httptest.Server.
func NewClientFromHTTP(httpClient *http.Client, baseURL string, opts ...connect.ClientOption) *Client {
	return &Client{
		query:   prodcatrpcv1connect.NewProductCatalogQueryServiceClient(httpClient, baseURL, opts...),
		command: prodcatrpcv1connect.NewProductCatalogCommandServiceClient(httpClient, baseURL, opts...),
	}
}

// --- Query methods ---

func (c *Client) GetProduct(ctx context.Context, productID string) (*prodcat.Product, error) {
	resp, err := c.query.GetProduct(ctx, connect.NewRequest(&prodcatrpcv1.GetProductRequest{
		ProductId: productID,
	}))
	if err != nil {
		return nil, fmt.Errorf("prodcat rpc: get product: %w", err)
	}
	p := productFromProto(resp.Msg.Product)
	return &p, nil
}

func (c *Client) ListProducts(ctx context.Context, filter prodcat.ListFilter) ([]prodcat.Product, error) {
	resp, err := c.query.ListProducts(ctx, connect.NewRequest(&prodcatrpcv1.ListProductsRequest{
		Tags:        filter.Tags,
		CountryCode: filter.CountryCode,
	}))
	if err != nil {
		return nil, fmt.Errorf("prodcat rpc: list products: %w", err)
	}
	result := make([]prodcat.Product, len(resp.Msg.Products))
	for i, pp := range resp.Msg.Products {
		result[i] = productFromProto(pp)
	}
	return result, nil
}

func (c *Client) GetRuleset(ctx context.Context, id string) (*prodcat.Ruleset, error) {
	resp, err := c.query.GetRuleset(ctx, connect.NewRequest(&prodcatrpcv1.GetRulesetRequest{
		RulesetId: id,
	}))
	if err != nil {
		return nil, fmt.Errorf("prodcat rpc: get ruleset: %w", err)
	}
	r := rulesetFromProto(resp.Msg.Ruleset)
	return &r, nil
}

func (c *Client) ListRulesets(ctx context.Context) ([]prodcat.Ruleset, error) {
	resp, err := c.query.ListRulesets(ctx, connect.NewRequest(&prodcatrpcv1.ListRulesetsRequest{}))
	if err != nil {
		return nil, fmt.Errorf("prodcat rpc: list rulesets: %w", err)
	}
	result := make([]prodcat.Ruleset, len(resp.Msg.Rulesets))
	for i, pr := range resp.Msg.Rulesets {
		result[i] = rulesetFromProto(pr)
	}
	return result, nil
}

func (c *Client) ResolveRuleset(ctx context.Context, productID string) (*prodcat.ResolvedRuleset, error) {
	resp, err := c.query.ResolveRuleset(ctx, connect.NewRequest(&prodcatrpcv1.ResolveRulesetRequest{
		ProductId: productID,
	}))
	if err != nil {
		return nil, fmt.Errorf("prodcat rpc: resolve ruleset: %w", err)
	}
	r := resp.Msg.Resolved
	resolved := &prodcat.ResolvedRuleset{
		ProductID: r.ProductId,
		Merged:    r.Merged,
	}
	for _, l := range r.Layers {
		resolved.Layers = append(resolved.Layers, prodcat.RulesetLayer{
			Source:   l.Source,
			SourceID: l.SourceId,
		})
	}
	return resolved, nil
}

// --- Command methods ---

func (c *Client) DisableRuleset(ctx context.Context, id string, reason prodcat.DisabledReason, prov prodcat.Provenance) (*prodcat.Ruleset, error) {
	resp, err := c.command.DisableRuleset(ctx, connect.NewRequest(&prodcatrpcv1.DisableRulesetRequest{
		RulesetId:  id,
		Reason:     string(reason),
		Provenance: &prodcatrpcv1.Provenance{SourceUrn: prov.SourceURN, Reason: prov.Reason},
	}))
	if err != nil {
		return nil, fmt.Errorf("prodcat rpc: disable ruleset: %w", err)
	}
	r := rulesetFromProto(resp.Msg.Ruleset)
	return &r, nil
}

func (c *Client) EnableRuleset(ctx context.Context, id string, prov prodcat.Provenance) (*prodcat.Ruleset, error) {
	resp, err := c.command.EnableRuleset(ctx, connect.NewRequest(&prodcatrpcv1.EnableRulesetRequest{
		RulesetId:  id,
		Provenance: &prodcatrpcv1.Provenance{SourceUrn: prov.SourceURN, Reason: prov.Reason},
	}))
	if err != nil {
		return nil, fmt.Errorf("prodcat rpc: enable ruleset: %w", err)
	}
	r := rulesetFromProto(resp.Msg.Ruleset)
	return &r, nil
}
