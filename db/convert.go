package db

import (
	"github.com/laenen-partners/prodcat"
	gen "github.com/laenen-partners/prodcat/db/gen"
)

// Row-to-domain converters. These map SQLC-generated types back to prodcat types.

func familyFromRow(row gen.Family) prodcat.FamilyDefinition {
	return prodcat.FamilyDefinition{
		ID:             row.ID,
		Family:         prodcat.ProductFamily(row.Family),
		Name:           row.Name,
		Description:    row.Description,
		Ruleset:        row.Ruleset,
		BaseRulesetIDs: row.BaseRulesetIds,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func archetypeFromRow(row gen.Archetype) prodcat.Archetype {
	return prodcat.Archetype{
		ID:             row.ID,
		FamilyID:       row.FamilyID,
		Name:           row.Name,
		Description:    row.Description,
		Ruleset:        row.Ruleset,
		BaseRulesetIDs: row.BaseRulesetIds,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func productFromRow(row gen.Product) prodcat.Product {
	return prodcat.Product{
		ID:          row.ID,
		ArchetypeID: row.ArchetypeID,
		Name:        row.Name,
		Description: row.Description,
		Tagline:     row.Tagline,
		Status:      prodcat.ProductStatus(row.Status),
		ProductType: prodcat.ProductType(row.ProductType),
		CurrencyCode: row.CurrencyCode,
		ParentProductID: row.ParentProductID,
		Provider: prodcat.RegulatoryProvider{
			ProviderID:        row.ProviderID,
			Name:              row.ProviderName,
			Regulator:         row.Regulator,
			LicenseNumber:     row.LicenseNumber,
			RegulatoryCountry: row.RegulatoryCountry,
		},
		EffectiveFrom: row.EffectiveFrom,
		EffectiveTo:   row.EffectiveTo,
		Compliance: prodcat.ComplianceConfig{
			ShariaCompliant: row.ShariaCompliant,
		},
		Eligibility: prodcat.EligibilityConfig{
			Geographic: prodcat.GeographicAvailability{
				Mode:         prodcat.AvailabilityMode(row.AvailabilityMode),
				CountryCodes: row.CountryCodes,
			},
			Ruleset:        row.Ruleset,
			BaseRulesetIDs: row.BaseRulesetIds,
		},
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		CreatedBy: row.CreatedBy,
	}
}

func baseRulesetFromRow(row gen.BaseRuleset) prodcat.BaseRuleset {
	return prodcat.BaseRuleset{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		Content:     row.Content,
		Version:     row.Version,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func subscriptionFromRow(row gen.Subscription) prodcat.Subscription {
	sub := prodcat.Subscription{
		ID:                   row.ID,
		ProductID:            row.ProductID,
		EntityID:             row.EntityID,
		EntityType:           prodcat.EntityType(row.EntityType),
		Status:               prodcat.SubscriptionStatus(row.Status),
		ParentSubscriptionID: row.ParentSubscriptionID,
		ExternalRef:          row.ExternalRef,
		CreatedAt:            row.CreatedAt,
		ActivatedAt:          row.ActivatedAt,
		CanceledAt:           row.CanceledAt,
		SigningAuthority: prodcat.SigningAuthority{
			Rule:          prodcat.SigningRule(row.SigningRule),
			RequiredCount: int(row.RequiredCount),
		},
	}
	if row.Disabled {
		sub.Disabled = &prodcat.DisabledState{
			Disabled:  true,
			Reason:    prodcat.DisabledReason(row.DisabledReason),
			Message:   row.DisabledMessage,
			DisabledAt: row.DisabledAt,
		}
	}
	return sub
}

func partyFromRow(row gen.Party) prodcat.Party {
	p := prodcat.Party{
		ID:              row.ID,
		SubscriptionID:  row.SubscriptionID,
		CustomerID:      row.CustomerID,
		Role:            prodcat.PartyRole(row.Role),
		RequirementsMet: row.RequirementsMet,
		AddedAt:         row.AddedAt,
		RemovedAt:       row.RemovedAt,
	}
	if row.Disabled {
		p.Disabled = &prodcat.DisabledState{
			Disabled:  true,
			Reason:    prodcat.DisabledReason(row.DisabledReason),
			Message:   row.DisabledMessage,
			DisabledAt: row.DisabledAt,
		}
	}
	return p
}

func capabilityFromRow(row gen.Capability) prodcat.Capability {
	c := prodcat.Capability{
		ID:             row.ID,
		SubscriptionID: row.SubscriptionID,
		CapabilityType: prodcat.CapabilityType(row.CapabilityType),
		Status:         prodcat.CapabilityStatus(row.Status),
	}
	if row.Disabled {
		c.Disabled = &prodcat.DisabledState{
			Disabled:  true,
			Reason:    prodcat.DisabledReason(row.DisabledReason),
			Message:   row.DisabledMessage,
			DisabledAt: row.DisabledAt,
		}
	}
	return c
}
