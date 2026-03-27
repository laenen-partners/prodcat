package ui

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/laenen-partners/dsx/ds"
	"github.com/laenen-partners/identity"
	"github.com/laenen-partners/prodcat"
	"github.com/starfederation/datastar-go/datastar"
	"gopkg.in/yaml.v3"
)

// ProductListSignals holds the client-side filter state.
type ProductListSignals struct {
	ShowDisabled bool `json:"showDisabled"`
}

// RulesetListSignals holds the client-side filter state for rulesets.
type RulesetListSignals struct {
	ShowDisabled bool `json:"showDisabled"`
}

// CatalogService is the interface the UI needs from a prodcat backend.
// Both [prodcat.Client] and [connectrpc.Client] satisfy this.
type CatalogService interface {
	GetProduct(ctx context.Context, productID string) (*prodcat.Product, error)
	ListProducts(ctx context.Context, filter prodcat.ListFilter) ([]prodcat.Product, error)
	GetRuleset(ctx context.Context, id string) (*prodcat.Ruleset, error)
	ListRulesets(ctx context.Context) ([]prodcat.Ruleset, error)
	ResolveRuleset(ctx context.Context, productID string) (*prodcat.ResolvedRuleset, error)
	DisableRuleset(ctx context.Context, id string, reason prodcat.DisabledReason, prov prodcat.Provenance) (*prodcat.Ruleset, error)
	EnableRuleset(ctx context.Context, id string, prov prodcat.Provenance) (*prodcat.Ruleset, error)
}

// Handlers provides HTTP handlers for product catalogue UI fragments.
type Handlers struct {
	client CatalogService
	policy AccessPolicy
}

// NewHandlers creates fragment handlers backed by the given CatalogService.
func NewHandlers(client CatalogService, policy AccessPolicy) *Handlers {
	return &Handlers{client: client, policy: policy}
}

// RegisterRoutes mounts all catalogue UI fragment routes on the given chi router.
func (h *Handlers) RegisterRoutes(r chi.Router) {
	r.Get("/fragments/products", h.ProductList())
	r.Get("/fragments/products/{id}", h.ProductDetail())
	r.Get("/fragments/rulesets", h.RulesetList())
	r.Get("/fragments/rulesets/{id}", h.RulesetDetail())
	r.Post("/fragments/rulesets/{id}/disable", h.DisableRuleset())
	r.Post("/fragments/rulesets/{id}/enable", h.EnableRuleset())
}

// ProductList returns an HTTP handler that renders the product list table.
func (h *Handlers) ProductList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var signals ProductListSignals
		_ = ds.ReadSignals("catalog", r, &signals)
		sse := datastar.NewSSE(w, r)

		scope := h.scopeFromRequest(r)
		filter := prodcat.ListFilter{
			Tags: append([]string{}, scope.ListTags...),
		}

		products, err := h.client.ListProducts(r.Context(), filter)
		if err != nil {
			slog.ErrorContext(r.Context(), "prodcat ui: list products", "error", err)
			ds.Send.Toast(sse, ds.ToastError, "Failed to load products")
			return
		}

		// Filter disabled if not requested.
		if !signals.ShowDisabled {
			filtered := make([]prodcat.Product, 0, len(products))
			for _, p := range products {
				if !p.Disabled {
					filtered = append(filtered, p)
				}
			}
			products = filtered
		}

		ds.Send.Patch(sse, ProductListTable(products, scope.CanEdit, signals))
	}
}

// ProductDetail returns an HTTP handler that renders a product detail drawer.
func (h *Handlers) ProductDetail() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		productID := r.PathValue("id")
		if productID == "" {
			http.Error(w, "missing product id", http.StatusBadRequest)
			return
		}

		product, err := h.client.GetProduct(r.Context(), productID)
		if err != nil {
			slog.ErrorContext(r.Context(), "prodcat ui: get product", "product_id", productID, "error", err)
			http.Error(w, "product not found", http.StatusNotFound)
			return
		}

		sse := datastar.NewSSE(w, r)
		ds.Send.Drawer(sse, ProductDetailContent(product), ds.WithDrawerMaxWidth("max-w-xl"))
	}
}

// RulesetList returns an HTTP handler that renders the ruleset list table.
func (h *Handlers) RulesetList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var signals RulesetListSignals
		_ = ds.ReadSignals("catalog", r, &signals)
		sse := datastar.NewSSE(w, r)

		scope := h.scopeFromRequest(r)
		rulesets, err := h.client.ListRulesets(r.Context())
		if err != nil {
			slog.ErrorContext(r.Context(), "prodcat ui: list rulesets", "error", err)
			ds.Send.Toast(sse, ds.ToastError, "Failed to load rulesets")
			return
		}

		// Filter disabled if not requested.
		if !signals.ShowDisabled {
			filtered := make([]prodcat.Ruleset, 0, len(rulesets))
			for _, rs := range rulesets {
				if !rs.Disabled {
					filtered = append(filtered, rs)
				}
			}
			rulesets = filtered
		}

		ds.Send.Patch(sse, RulesetListTable(rulesets, scope.CanEdit, signals))
	}
}

// RulesetDetail returns an HTTP handler that renders a ruleset detail drawer.
func (h *Handlers) RulesetDetail() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rulesetID := r.PathValue("id")
		if rulesetID == "" {
			http.Error(w, "missing ruleset id", http.StatusBadRequest)
			return
		}

		ruleset, err := h.client.GetRuleset(r.Context(), rulesetID)
		if err != nil {
			slog.ErrorContext(r.Context(), "prodcat ui: get ruleset", "ruleset_id", rulesetID, "error", err)
			http.Error(w, "ruleset not found", http.StatusNotFound)
			return
		}

		scope := h.scopeFromRequest(r)
		sse := datastar.NewSSE(w, r)
		ds.Send.Drawer(sse, RulesetDetailContent(ruleset, scope.CanEdit), ds.WithDrawerMaxWidth("max-w-2xl"))
	}
}

// DisableRuleset handles POST /fragments/rulesets/{id}/disable.
func (h *Handlers) DisableRuleset() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rulesetID := r.PathValue("id")
		sse := datastar.NewSSE(w, r)

		scope := h.scopeFromRequest(r)
		if !scope.CanEdit {
			ds.Send.Toast(sse, ds.ToastError, "You do not have permission to disable rulesets")
			return
		}

		prov := provenanceFromRequest(r)
		rs, err := h.client.DisableRuleset(r.Context(), rulesetID, prodcat.DisabledReasonDeleted, prov)
		if err != nil {
			slog.ErrorContext(r.Context(), "prodcat ui: disable ruleset", "ruleset_id", rulesetID, "error", err)
			ds.Send.Toast(sse, ds.ToastError, "Failed to disable ruleset")
			return
		}

		ds.Send.Drawer(sse, RulesetDetailContent(rs, scope.CanEdit), ds.WithDrawerMaxWidth("max-w-2xl"))
		ds.Send.Toast(sse, ds.ToastSuccess, "Ruleset disabled")
	}
}

// EnableRuleset handles POST /fragments/rulesets/{id}/enable.
func (h *Handlers) EnableRuleset() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rulesetID := r.PathValue("id")
		sse := datastar.NewSSE(w, r)

		scope := h.scopeFromRequest(r)
		if !scope.CanEdit {
			ds.Send.Toast(sse, ds.ToastError, "You do not have permission to enable rulesets")
			return
		}

		prov := provenanceFromRequest(r)
		rs, err := h.client.EnableRuleset(r.Context(), rulesetID, prov)
		if err != nil {
			slog.ErrorContext(r.Context(), "prodcat ui: enable ruleset", "ruleset_id", rulesetID, "error", err)
			ds.Send.Toast(sse, ds.ToastError, "Failed to enable ruleset")
			return
		}

		ds.Send.Drawer(sse, RulesetDetailContent(rs, scope.CanEdit), ds.WithDrawerMaxWidth("max-w-2xl"))
		ds.Send.Toast(sse, ds.ToastSuccess, "Ruleset enabled")
	}
}

func (h *Handlers) scopeFromRequest(r *http.Request) AccessScope {
	ident, ok := identity.FromContext(r.Context())
	if !ok {
		return AccessScope{}
	}
	return h.policy(ident)
}

// parseYAML unmarshals YAML bytes into a generic structure for the yamltree component.
func parseYAML(data []byte) any {
	var out any
	_ = yaml.Unmarshal(data, &out)
	return out
}

func provenanceFromRequest(r *http.Request) prodcat.Provenance {
	ident, ok := identity.FromContext(r.Context())
	if !ok {
		return prodcat.Provenance{SourceURN: "ui:anonymous"}
	}
	return prodcat.Provenance{SourceURN: "ui:" + ident.PrincipalID()}
}
