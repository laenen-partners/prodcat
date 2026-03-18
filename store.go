package prodcat

import "context"

// Store is the persistence interface for the eligibility engine.
// Backed by entitystore (PostgreSQL).
type Store interface {
	// Products
	PutProduct(ctx context.Context, p ProductEligibility) error
	GetProduct(ctx context.Context, productID string) (ProductEligibility, error)
	ListProducts(ctx context.Context, filter TagFilter) ([]ProductEligibility, error)

	// Base rulesets
	PutRuleset(ctx context.Context, r BaseRuleset) error
	GetRuleset(ctx context.Context, id string) (BaseRuleset, error)
	ListRulesets(ctx context.Context) ([]BaseRuleset, error)

	// Subscriptions
	PutSubscription(ctx context.Context, s Subscription) error
	GetSubscription(ctx context.Context, id string) (Subscription, error)
	ListSubscriptions(ctx context.Context, filter SubscriptionFilter) ([]Subscription, error)
}

func hasAllTags(tags, required []string) bool {
	set := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		set[t] = struct{}{}
	}
	for _, r := range required {
		if _, ok := set[r]; !ok {
			return false
		}
	}
	return true
}
