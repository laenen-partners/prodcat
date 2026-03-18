package gen

import (
	"context"
	"encoding/json"
)

func scanJSON(src any, dst *map[string]string) error {
	if src == nil {
		*dst = map[string]string{}
		return nil
	}
	b, ok := src.([]byte)
	if !ok {
		*dst = map[string]string{}
		return nil
	}
	return json.Unmarshal(b, dst)
}

func jsonBytes(m map[string]string) []byte {
	if m == nil {
		return []byte("{}")
	}
	b, _ := json.Marshal(m)
	return b
}

func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ─── Families ───

func (q *Queries) CreateFamily(ctx context.Context, arg CreateFamilyParams) (Family, error) {
	row := q.db.QueryRow(ctx,
		`INSERT INTO families (id, family, name, description, ruleset, base_ruleset_ids)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING *`,
		arg.ID, arg.Family, jsonBytes(arg.Name), jsonBytes(arg.Description), arg.Ruleset, emptyIfNil(arg.BaseRulesetIds))
	return scanFamily(row)
}

func (q *Queries) GetFamily(ctx context.Context, id string) (Family, error) {
	row := q.db.QueryRow(ctx, `SELECT * FROM families WHERE id = $1`, id)
	return scanFamily(row)
}

func (q *Queries) ListFamilies(ctx context.Context) ([]Family, error) {
	rows, err := q.db.Query(ctx, `SELECT * FROM families ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Family
	for rows.Next() {
		f, err := scanFamilyRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

func (q *Queries) UpdateFamily(ctx context.Context, arg UpdateFamilyParams) (Family, error) {
	row := q.db.QueryRow(ctx,
		`UPDATE families SET name=$2, description=$3, ruleset=$4, base_ruleset_ids=$5, updated_at=now()
		 WHERE id=$1 RETURNING *`,
		arg.ID, jsonBytes(arg.Name), jsonBytes(arg.Description), arg.Ruleset, emptyIfNil(arg.BaseRulesetIds))
	return scanFamily(row)
}

func scanFamily(row interface{ Scan(...any) error }) (Family, error) {
	var f Family
	var nameB, descB []byte
	err := row.Scan(&f.ID, &f.Family, &nameB, &descB, &f.Ruleset, &f.BaseRulesetIds, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return f, err
	}
	scanJSON(nameB, &f.Name)
	scanJSON(descB, &f.Description)
	return f, nil
}

func scanFamilyRows(rows interface{ Scan(...any) error }) (Family, error) {
	return scanFamily(rows)
}

// ─── Archetypes ───

func (q *Queries) CreateArchetype(ctx context.Context, arg CreateArchetypeParams) (Archetype, error) {
	row := q.db.QueryRow(ctx,
		`INSERT INTO archetypes (id, family_id, name, description, ruleset, base_ruleset_ids)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING *`,
		arg.ID, arg.FamilyID, jsonBytes(arg.Name), jsonBytes(arg.Description), arg.Ruleset, emptyIfNil(arg.BaseRulesetIds))
	return scanArchetype(row)
}

func (q *Queries) GetArchetype(ctx context.Context, id string) (Archetype, error) {
	row := q.db.QueryRow(ctx, `SELECT * FROM archetypes WHERE id = $1`, id)
	return scanArchetype(row)
}

func (q *Queries) ListArchetypes(ctx context.Context, familyID string) ([]Archetype, error) {
	rows, err := q.db.Query(ctx, `SELECT * FROM archetypes WHERE family_id = $1 ORDER BY created_at`, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Archetype
	for rows.Next() {
		a, err := scanArchetype(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (q *Queries) UpdateArchetype(ctx context.Context, arg UpdateArchetypeParams) (Archetype, error) {
	row := q.db.QueryRow(ctx,
		`UPDATE archetypes SET name=$2, description=$3, ruleset=$4, base_ruleset_ids=$5, updated_at=now()
		 WHERE id=$1 RETURNING *`,
		arg.ID, jsonBytes(arg.Name), jsonBytes(arg.Description), arg.Ruleset, emptyIfNil(arg.BaseRulesetIds))
	return scanArchetype(row)
}

func scanArchetype(row interface{ Scan(...any) error }) (Archetype, error) {
	var a Archetype
	var nameB, descB []byte
	err := row.Scan(&a.ID, &a.FamilyID, &nameB, &descB, &a.Ruleset, &a.BaseRulesetIds, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return a, err
	}
	scanJSON(nameB, &a.Name)
	scanJSON(descB, &a.Description)
	return a, nil
}

// ─── Products ───

func (q *Queries) CreateProduct(ctx context.Context, arg CreateProductParams) (Product, error) {
	row := q.db.QueryRow(ctx,
		`INSERT INTO products (
			id, archetype_id, name, description, tagline,
			status, product_type, currency_code, parent_product_id,
			provider_id, provider_name, regulator, license_number, regulatory_country,
			sharia_compliant, availability_mode, country_codes,
			ruleset, base_ruleset_ids, effective_from, effective_to, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
		RETURNING *`,
		arg.ID, arg.ArchetypeID, jsonBytes(arg.Name), jsonBytes(arg.Description), jsonBytes(arg.Tagline),
		arg.Status, arg.ProductType, arg.CurrencyCode, arg.ParentProductID,
		arg.ProviderID, arg.ProviderName, arg.Regulator, arg.LicenseNumber, arg.RegulatoryCountry,
		arg.ShariaCompliant, arg.AvailabilityMode, emptyIfNil(arg.CountryCodes),
		arg.Ruleset, emptyIfNil(arg.BaseRulesetIds), arg.EffectiveFrom, arg.EffectiveTo, arg.CreatedBy)
	return scanProduct(row)
}

func (q *Queries) GetProduct(ctx context.Context, id string) (Product, error) {
	row := q.db.QueryRow(ctx, `SELECT * FROM products WHERE id = $1`, id)
	return scanProduct(row)
}

func (q *Queries) ListProducts(ctx context.Context, arg ListProductsParams) ([]Product, error) {
	limit := arg.ResultLimit
	if limit <= 0 {
		limit = 100
	}
	rows, err := q.db.Query(ctx,
		`SELECT * FROM products
		 WHERE ($1::text IS NULL OR archetype_id = $1)
		   AND ($2::text IS NULL OR archetype_id IN (SELECT id FROM archetypes WHERE family_id = $2))
		   AND ($3::text IS NULL OR status = $3::product_status)
		   AND ($4::text IS NULL OR currency_code = $4)
		 ORDER BY created_at LIMIT $5 OFFSET $6`,
		arg.ArchetypeID, arg.FamilyID, arg.Status, arg.CurrencyCode, limit, arg.ResultOffset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (q *Queries) UpdateProduct(ctx context.Context, arg UpdateProductParams) (Product, error) {
	row := q.db.QueryRow(ctx,
		`UPDATE products SET name=$2, description=$3, tagline=$4, currency_code=$5,
		 sharia_compliant=$6, availability_mode=$7, country_codes=$8,
		 ruleset=$9, base_ruleset_ids=$10, effective_from=$11, effective_to=$12, updated_at=now()
		 WHERE id=$1 RETURNING *`,
		arg.ID, jsonBytes(arg.Name), jsonBytes(arg.Description), jsonBytes(arg.Tagline),
		arg.CurrencyCode, arg.ShariaCompliant, arg.AvailabilityMode, emptyIfNil(arg.CountryCodes),
		arg.Ruleset, emptyIfNil(arg.BaseRulesetIds), arg.EffectiveFrom, arg.EffectiveTo)
	return scanProduct(row)
}

func (q *Queries) TransitionProductStatus(ctx context.Context, arg TransitionProductStatusParams) (Product, error) {
	row := q.db.QueryRow(ctx,
		`UPDATE products SET status=$2, updated_at=now() WHERE id=$1 RETURNING *`,
		arg.ID, arg.Status)
	return scanProduct(row)
}

func (q *Queries) ListAvailableProducts(ctx context.Context, arg ListAvailableProductsParams) ([]Product, error) {
	rows, err := q.db.Query(ctx,
		`SELECT * FROM products WHERE status = 'active'
		 AND (availability_mode = 'global'
		   OR (availability_mode = 'specific_countries' AND $1 = ANY(country_codes))
		   OR (availability_mode = 'global_except' AND NOT $1 = ANY(country_codes)))
		 ORDER BY created_at`, arg.CountryCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func scanProduct(row interface{ Scan(...any) error }) (Product, error) {
	var p Product
	var nameB, descB, tagB []byte
	err := row.Scan(
		&p.ID, &p.ArchetypeID, &nameB, &descB, &tagB,
		&p.Status, &p.ProductType, &p.CurrencyCode, &p.ParentProductID,
		&p.ProviderID, &p.ProviderName, &p.Regulator, &p.LicenseNumber, &p.RegulatoryCountry,
		&p.ShariaCompliant, &p.AvailabilityMode, &p.CountryCodes,
		&p.Ruleset, &p.BaseRulesetIds, &p.EffectiveFrom, &p.EffectiveTo,
		&p.CreatedAt, &p.UpdatedAt, &p.CreatedBy)
	if err != nil {
		return p, err
	}
	scanJSON(nameB, &p.Name)
	scanJSON(descB, &p.Description)
	scanJSON(tagB, &p.Tagline)
	return p, nil
}

// ─── Base Rulesets ───

func (q *Queries) CreateBaseRuleset(ctx context.Context, arg CreateBaseRulesetParams) (BaseRuleset, error) {
	row := q.db.QueryRow(ctx,
		`INSERT INTO base_rulesets (id, name, description, content, version) VALUES ($1,$2,$3,$4,$5) RETURNING *`,
		arg.ID, arg.Name, arg.Description, arg.Content, arg.Version)
	return scanBaseRuleset(row)
}

func (q *Queries) GetBaseRuleset(ctx context.Context, id string) (BaseRuleset, error) {
	row := q.db.QueryRow(ctx, `SELECT * FROM base_rulesets WHERE id = $1`, id)
	return scanBaseRuleset(row)
}

func (q *Queries) ListBaseRulesets(ctx context.Context) ([]BaseRuleset, error) {
	rows, err := q.db.Query(ctx, `SELECT * FROM base_rulesets ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []BaseRuleset
	for rows.Next() {
		r, err := scanBaseRuleset(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (q *Queries) UpdateBaseRuleset(ctx context.Context, arg UpdateBaseRulesetParams) (BaseRuleset, error) {
	row := q.db.QueryRow(ctx,
		`UPDATE base_rulesets SET name=$2, description=$3, content=$4, version=$5, updated_at=now() WHERE id=$1 RETURNING *`,
		arg.ID, arg.Name, arg.Description, arg.Content, arg.Version)
	return scanBaseRuleset(row)
}

func scanBaseRuleset(row interface{ Scan(...any) error }) (BaseRuleset, error) {
	var r BaseRuleset
	return r, row.Scan(&r.ID, &r.Name, &r.Description, &r.Content, &r.Version, &r.CreatedAt, &r.UpdatedAt)
}

// ─── Subscriptions ───

func (q *Queries) CreateSubscription(ctx context.Context, arg CreateSubscriptionParams) (Subscription, error) {
	row := q.db.QueryRow(ctx,
		`INSERT INTO subscriptions (entity_id, entity_type, product_id, signing_rule, required_count)
		 VALUES ($1,$2,$3,$4,$5) RETURNING *`,
		arg.EntityID, arg.EntityType, arg.ProductID, arg.SigningRule, arg.RequiredCount)
	return scanSubscription(row)
}

func (q *Queries) GetSubscription(ctx context.Context, id string) (Subscription, error) {
	row := q.db.QueryRow(ctx, `SELECT * FROM subscriptions WHERE id = $1`, id)
	return scanSubscription(row)
}

func (q *Queries) ListSubscriptions(ctx context.Context, arg ListSubscriptionsParams) ([]Subscription, error) {
	limit := arg.ResultLimit
	if limit <= 0 {
		limit = 100
	}
	rows, err := q.db.Query(ctx,
		`SELECT * FROM subscriptions
		 WHERE ($1::text IS NULL OR entity_id = $1)
		   AND ($2::text IS NULL OR id IN (SELECT subscription_id FROM parties WHERE customer_id = $2 AND removed_at IS NULL))
		   AND ($3::text IS NULL OR status = $3::subscription_status)
		 ORDER BY created_at DESC LIMIT $4 OFFSET $5`,
		arg.EntityID, arg.CustomerID, arg.Status, limit, arg.ResultOffset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Subscription
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (q *Queries) ActivateSubscription(ctx context.Context, arg ActivateSubscriptionParams) error {
	_, err := q.db.Exec(ctx,
		`UPDATE subscriptions SET status='active', external_ref=$2, activated_at=now() WHERE id=$1`,
		arg.ID, arg.ExternalRef)
	return err
}

func (q *Queries) CancelSubscription(ctx context.Context, id string) error {
	_, err := q.db.Exec(ctx, `UPDATE subscriptions SET status='canceled', canceled_at=now() WHERE id=$1`, id)
	return err
}

func (q *Queries) DisableSubscription(ctx context.Context, arg DisableSubscriptionParams) error {
	_, err := q.db.Exec(ctx,
		`UPDATE subscriptions SET disabled=true, disabled_reason=$2, disabled_message=$3, disabled_at=now() WHERE id=$1`,
		arg.ID, arg.DisabledReason, arg.DisabledMessage)
	return err
}

func (q *Queries) EnableSubscription(ctx context.Context, id string) error {
	_, err := q.db.Exec(ctx,
		`UPDATE subscriptions SET disabled=false, disabled_reason=NULL, disabled_message='', disabled_at=NULL WHERE id=$1`, id)
	return err
}

func (q *Queries) UpdateSigningAuthority(ctx context.Context, arg UpdateSigningAuthorityParams) error {
	_, err := q.db.Exec(ctx,
		`UPDATE subscriptions SET signing_rule=$2, required_count=$3 WHERE id=$1`,
		arg.ID, arg.SigningRule, arg.RequiredCount)
	return err
}

func scanSubscription(row interface{ Scan(...any) error }) (Subscription, error) {
	var s Subscription
	var disabledReason *string
	err := row.Scan(
		&s.ID, &s.ProductID, &s.EntityID, &s.EntityType, &s.Status,
		&s.SigningRule, &s.RequiredCount, &s.ParentSubscriptionID, &s.ExternalRef,
		&s.Disabled, &disabledReason, &s.DisabledMessage, &s.DisabledAt,
		&s.EvalState, &s.CreatedAt, &s.ActivatedAt, &s.CanceledAt)
	if disabledReason != nil {
		s.DisabledReason = DisabledReason(*disabledReason)
	}
	return s, err
}

// ─── Parties ───

func (q *Queries) AddParty(ctx context.Context, arg AddPartyParams) (Party, error) {
	row := q.db.QueryRow(ctx,
		`INSERT INTO parties (subscription_id, customer_id, role) VALUES ($1,$2,$3) RETURNING *`,
		arg.SubscriptionID, arg.CustomerID, arg.Role)
	return scanParty(row)
}

func (q *Queries) ListParties(ctx context.Context, subscriptionID string) ([]Party, error) {
	rows, err := q.db.Query(ctx,
		`SELECT * FROM parties WHERE subscription_id=$1 AND removed_at IS NULL ORDER BY added_at`, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Party
	for rows.Next() {
		p, err := scanParty(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (q *Queries) RemoveParty(ctx context.Context, id string) error {
	_, err := q.db.Exec(ctx, `UPDATE parties SET removed_at=now() WHERE id=$1`, id)
	return err
}

func scanParty(row interface{ Scan(...any) error }) (Party, error) {
	var p Party
	var disabledReason *string
	err := row.Scan(
		&p.ID, &p.SubscriptionID, &p.CustomerID, &p.Role, &p.RequirementsMet,
		&p.Disabled, &disabledReason, &p.DisabledMessage, &p.DisabledAt,
		&p.EvalState, &p.AddedAt, &p.RemovedAt)
	if disabledReason != nil {
		p.DisabledReason = DisabledReason(*disabledReason)
	}
	return p, err
}

// ─── Capabilities ───

func (q *Queries) CreateCapability(ctx context.Context, arg CreateCapabilityParams) (Capability, error) {
	row := q.db.QueryRow(ctx,
		`INSERT INTO capabilities (subscription_id, capability_type, status) VALUES ($1,$2,$3) RETURNING *`,
		arg.SubscriptionID, arg.CapabilityType, arg.Status)
	return scanCapability(row)
}

func (q *Queries) ListCapabilities(ctx context.Context, subscriptionID string) ([]Capability, error) {
	rows, err := q.db.Query(ctx,
		`SELECT * FROM capabilities WHERE subscription_id=$1 ORDER BY created_at`, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Capability
	for rows.Next() {
		c, err := scanCapability(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (q *Queries) DisableCapability(ctx context.Context, arg DisableCapabilityParams) error {
	_, err := q.db.Exec(ctx,
		`UPDATE capabilities SET status='disabled', disabled=true, disabled_reason=$2, disabled_message=$3, disabled_at=now() WHERE id=$1`,
		arg.ID, arg.DisabledReason, arg.DisabledMessage)
	return err
}

func (q *Queries) EnableCapability(ctx context.Context, id string) error {
	_, err := q.db.Exec(ctx,
		`UPDATE capabilities SET status='active', disabled=false, disabled_reason=NULL, disabled_message='', disabled_at=NULL WHERE id=$1`, id)
	return err
}

func scanCapability(row interface{ Scan(...any) error }) (Capability, error) {
	var c Capability
	var disabledReason *string
	err := row.Scan(
		&c.ID, &c.SubscriptionID, &c.CapabilityType, &c.Status,
		&c.Disabled, &disabledReason, &c.DisabledMessage, &c.DisabledAt, &c.CreatedAt)
	if disabledReason != nil {
		c.DisabledReason = DisabledReason(*disabledReason)
	}
	return c, err
}
