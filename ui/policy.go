// Package ui provides reusable DSX fragments and templ components for the product catalogue.
//
// The consuming app mounts fragment handlers on its router and injects an
// [AccessPolicy] to control what each caller can see:
//
//	h := prodcatui.NewHandlers(client, policyFn)
//	h.RegisterRoutes(r)
package ui

import "github.com/laenen-partners/identity"

// AccessPolicy maps a caller's identity to the permissions they have
// within the product catalogue UI. The consuming app defines the policy.
type AccessPolicy func(identity.Context) AccessScope

// AccessScope controls what a caller can see and do.
type AccessScope struct {
	// ListTags are injected into every ListProducts query (AND-combined).
	ListTags []string

	// CanEdit controls whether the caller can modify products and rulesets.
	CanEdit bool

	// CanImport controls whether the caller can import catalogue definitions.
	CanImport bool
}
