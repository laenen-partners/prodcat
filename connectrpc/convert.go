package connectrpc

import (
	"errors"
	"time"

	connect "connectrpc.com/connect"

	"github.com/laenen-partners/prodcat"
	prodcatrpcv1 "github.com/laenen-partners/prodcat/connectrpc/gen/prodcat/rpc/v1"
)

// --- Domain → Proto ---

func productToProto(p *prodcat.Product) *prodcatrpcv1.Product {
	pp := &prodcatrpcv1.Product{
		ProductId:       p.ProductID,
		Name:            p.Name,
		Description:     p.Description,
		Tags:            p.Tags,
		CurrencyCode:    p.CurrencyCode,
		ParentProductId: p.ParentProductID,
		BaseRulesetIds:  p.BaseRulesetIDs,
		Ruleset:         p.Ruleset,
		CreatedAt:       p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       p.UpdatedAt.Format(time.RFC3339),
	}
	if p.Disabled {
		pp.Status = prodcatrpcv1.ProductStatus_PRODUCT_STATUS_SUSPENDED
	} else {
		pp.Status = prodcatrpcv1.ProductStatus_PRODUCT_STATUS_ACTIVE
	}
	if p.Availability.Mode != "" {
		pp.Availability = &prodcatrpcv1.GeoAvailability{
			Mode:         string(p.Availability.Mode),
			CountryCodes: p.Availability.CountryCodes,
		}
	}
	return pp
}

func rulesetToProto(r *prodcat.Ruleset) *prodcatrpcv1.Ruleset {
	return &prodcatrpcv1.Ruleset{
		RulesetId:      r.ID,
		Name:           r.Name,
		Description:    r.Description,
		Content:        r.Content,
		Version:        r.Version,
		Disabled:       r.Disabled,
		DisabledReason: string(r.DisabledReason),
		CreatedAt:      r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      r.UpdatedAt.Format(time.RFC3339),
	}
}

func resolvedToProto(r *prodcat.ResolvedRuleset) *prodcatrpcv1.ResolvedRuleset {
	layers := make([]*prodcatrpcv1.RulesetLayer, len(r.Layers))
	for i, l := range r.Layers {
		layers[i] = &prodcatrpcv1.RulesetLayer{Source: l.Source, SourceId: l.SourceID}
	}
	return &prodcatrpcv1.ResolvedRuleset{
		ProductId: r.ProductID,
		Merged:    r.Merged,
		Layers:    layers,
	}
}

// --- Proto → Domain ---

func productFromProto(pp *prodcatrpcv1.Product) prodcat.Product {
	p := prodcat.Product{
		ProductID:       pp.ProductId,
		Name:            pp.Name,
		Description:     pp.Description,
		Tags:            pp.Tags,
		Disabled:        pp.Status == prodcatrpcv1.ProductStatus_PRODUCT_STATUS_SUSPENDED,
		CurrencyCode:    pp.CurrencyCode,
		ParentProductID: pp.ParentProductId,
		BaseRulesetIDs:  pp.BaseRulesetIds,
		Ruleset:         pp.Ruleset,
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, pp.CreatedAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, pp.UpdatedAt)
	if pp.Availability != nil {
		p.Availability = prodcat.GeoAvailability{
			Mode:         prodcat.AvailabilityMode(pp.Availability.Mode),
			CountryCodes: pp.Availability.CountryCodes,
		}
	}
	return p
}

func rulesetFromProto(pr *prodcatrpcv1.Ruleset) prodcat.Ruleset {
	r := prodcat.Ruleset{
		ID:             pr.RulesetId,
		Name:           pr.Name,
		Description:    pr.Description,
		Content:        pr.Content,
		Version:        pr.Version,
		Disabled:       pr.Disabled,
		DisabledReason: prodcat.DisabledReason(pr.DisabledReason),
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339, pr.CreatedAt)
	r.UpdatedAt, _ = time.Parse(time.RFC3339, pr.UpdatedAt)
	return r
}

func provenanceFromProto(pp *prodcatrpcv1.Provenance) prodcat.Provenance {
	if pp == nil {
		return prodcat.Provenance{}
	}
	return prodcat.Provenance{
		SourceURN: pp.SourceUrn,
		Reason:    pp.Reason,
	}
}

// --- Error mapping ---

func toConnectError(err error) *connect.Error {
	switch {
	case errors.Is(err, prodcat.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, prodcat.ErrAlreadyExists):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, prodcat.ErrRulesetDisabled):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, prodcat.ErrValidation):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, prodcat.ErrNoStore):
		return connect.NewError(connect.CodeInternal, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
