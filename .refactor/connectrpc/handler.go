// Package connectrpc provides Connect-RPC handlers for the prodcat product catalogue.
//
// The handlers are thin translation layers that delegate to [prodcat.Client].
// Mount them on your HTTP mux alongside your own middleware:
//
//	h := prodcatrpc.NewHandler(client, tracker)
//	mux.Handle(prodcatrpcv1connect.NewProductCatalogQueryServiceHandler(h))
//	mux.Handle(prodcatrpcv1connect.NewProductCatalogCommandServiceHandler(h))
package connectrpc

import (
	"context"

	connect "connectrpc.com/connect"

	"github.com/laenen-partners/prodcat"
	prodcatrpcv1 "github.com/laenen-partners/prodcat/connectrpc/gen/prodcat/rpc/v1"
	"github.com/laenen-partners/prodcat/connectrpc/gen/prodcat/rpc/v1/prodcatrpcv1connect"
)

// Compile-time interface checks.
var (
	_ prodcatrpcv1connect.ProductCatalogQueryServiceHandler   = (*Handler)(nil)
	_ prodcatrpcv1connect.ProductCatalogCommandServiceHandler = (*Handler)(nil)
)

// Handler implements both ProductCatalogQueryServiceHandler and
// ProductCatalogCommandServiceHandler by delegating to a [prodcat.Client].
type Handler struct {
	client  *prodcat.Client
	tracker prodcat.ImportTracker
}

// NewHandler creates Connect-RPC handlers backed by the given prodcat Client.
// The tracker is used for Import idempotency — pass nil to disable tracking.
func NewHandler(client *prodcat.Client, tracker prodcat.ImportTracker) *Handler {
	return &Handler{client: client, tracker: tracker}
}

// --- Query service ---

func (h *Handler) GetProduct(ctx context.Context, req *connect.Request[prodcatrpcv1.GetProductRequest]) (*connect.Response[prodcatrpcv1.GetProductResponse], error) {
	p, err := h.client.GetProduct(ctx, req.Msg.ProductId)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&prodcatrpcv1.GetProductResponse{Product: productToProto(p)}), nil
}

func (h *Handler) ListProducts(ctx context.Context, req *connect.Request[prodcatrpcv1.ListProductsRequest]) (*connect.Response[prodcatrpcv1.ListProductsResponse], error) {
	result, err := h.client.ListProducts(ctx, prodcat.ListFilter{
		Tags:        req.Msg.Tags,
		CountryCode: req.Msg.CountryCode,
	})
	if err != nil {
		return nil, toConnectError(err)
	}
	out := make([]*prodcatrpcv1.Product, len(result))
	for i := range result {
		out[i] = productToProto(&result[i])
	}
	return connect.NewResponse(&prodcatrpcv1.ListProductsResponse{Products: out}), nil
}

func (h *Handler) GetRuleset(ctx context.Context, req *connect.Request[prodcatrpcv1.GetRulesetRequest]) (*connect.Response[prodcatrpcv1.GetRulesetResponse], error) {
	r, err := h.client.GetRuleset(ctx, req.Msg.RulesetId)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&prodcatrpcv1.GetRulesetResponse{Ruleset: rulesetToProto(r)}), nil
}

func (h *Handler) ListRulesets(ctx context.Context, req *connect.Request[prodcatrpcv1.ListRulesetsRequest]) (*connect.Response[prodcatrpcv1.ListRulesetsResponse], error) {
	result, err := h.client.ListRulesets(ctx)
	if err != nil {
		return nil, toConnectError(err)
	}
	out := make([]*prodcatrpcv1.Ruleset, len(result))
	for i := range result {
		out[i] = rulesetToProto(&result[i])
	}
	return connect.NewResponse(&prodcatrpcv1.ListRulesetsResponse{Rulesets: out}), nil
}

func (h *Handler) ResolveRuleset(ctx context.Context, req *connect.Request[prodcatrpcv1.ResolveRulesetRequest]) (*connect.Response[prodcatrpcv1.ResolveRulesetResponse], error) {
	resolved, err := h.client.ResolveRuleset(ctx, req.Msg.ProductId)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&prodcatrpcv1.ResolveRulesetResponse{Resolved: resolvedToProto(resolved)}), nil
}

// --- Command service ---

func (h *Handler) RegisterProduct(ctx context.Context, req *connect.Request[prodcatrpcv1.RegisterProductRequest]) (*connect.Response[prodcatrpcv1.RegisterProductResponse], error) {
	p := productFromProto(req.Msg.Product)
	prov := provenanceFromProto(req.Msg.Provenance)
	result, err := h.client.RegisterProduct(ctx, p, prov)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&prodcatrpcv1.RegisterProductResponse{Product: productToProto(result)}), nil
}

func (h *Handler) UpdateProduct(ctx context.Context, req *connect.Request[prodcatrpcv1.UpdateProductRequest]) (*connect.Response[prodcatrpcv1.UpdateProductResponse], error) {
	p := productFromProto(req.Msg.Product)
	prov := provenanceFromProto(req.Msg.Provenance)
	result, err := h.client.UpdateProduct(ctx, p, prov)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&prodcatrpcv1.UpdateProductResponse{Product: productToProto(result)}), nil
}

func (h *Handler) CreateRuleset(ctx context.Context, req *connect.Request[prodcatrpcv1.CreateRulesetRequest]) (*connect.Response[prodcatrpcv1.CreateRulesetResponse], error) {
	r := rulesetFromProto(req.Msg.Ruleset)
	prov := provenanceFromProto(req.Msg.Provenance)
	result, err := h.client.CreateRuleset(ctx, r, prov)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&prodcatrpcv1.CreateRulesetResponse{Ruleset: rulesetToProto(result)}), nil
}

func (h *Handler) DisableRuleset(ctx context.Context, req *connect.Request[prodcatrpcv1.DisableRulesetRequest]) (*connect.Response[prodcatrpcv1.DisableRulesetResponse], error) {
	prov := provenanceFromProto(req.Msg.Provenance)
	result, err := h.client.DisableRuleset(ctx, req.Msg.RulesetId, prodcat.DisabledReason(req.Msg.Reason), prov)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&prodcatrpcv1.DisableRulesetResponse{Ruleset: rulesetToProto(result)}), nil
}

func (h *Handler) EnableRuleset(ctx context.Context, req *connect.Request[prodcatrpcv1.EnableRulesetRequest]) (*connect.Response[prodcatrpcv1.EnableRulesetResponse], error) {
	prov := provenanceFromProto(req.Msg.Provenance)
	result, err := h.client.EnableRuleset(ctx, req.Msg.RulesetId, prov)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&prodcatrpcv1.EnableRulesetResponse{Ruleset: rulesetToProto(result)}), nil
}

func (h *Handler) AddRuleset(ctx context.Context, req *connect.Request[prodcatrpcv1.AddRulesetRequest]) (*connect.Response[prodcatrpcv1.AddRulesetResponse], error) {
	prov := provenanceFromProto(req.Msg.Provenance)
	result, err := h.client.AddRuleset(ctx, req.Msg.ProductId, req.Msg.RulesetId, prov)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&prodcatrpcv1.AddRulesetResponse{Product: productToProto(result)}), nil
}

func (h *Handler) RemoveRuleset(ctx context.Context, req *connect.Request[prodcatrpcv1.RemoveRulesetRequest]) (*connect.Response[prodcatrpcv1.RemoveRulesetResponse], error) {
	prov := provenanceFromProto(req.Msg.Provenance)
	result, err := h.client.RemoveRuleset(ctx, req.Msg.ProductId, req.Msg.RulesetId, prov)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&prodcatrpcv1.RemoveRulesetResponse{Product: productToProto(result)}), nil
}

func (h *Handler) SetProductRuleset(ctx context.Context, req *connect.Request[prodcatrpcv1.SetProductRulesetRequest]) (*connect.Response[prodcatrpcv1.SetProductRulesetResponse], error) {
	prov := provenanceFromProto(req.Msg.Provenance)
	result, err := h.client.SetProductRuleset(ctx, req.Msg.ProductId, req.Msg.Content, prov)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&prodcatrpcv1.SetProductRulesetResponse{Product: productToProto(result)}), nil
}

func (h *Handler) Import(ctx context.Context, req *connect.Request[prodcatrpcv1.ImportRequest]) (*connect.Response[prodcatrpcv1.ImportResponse], error) {
	if err := h.client.Import(ctx, req.Msg.Filename, req.Msg.Data, h.tracker); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&prodcatrpcv1.ImportResponse{}), nil
}
