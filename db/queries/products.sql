-- name: CreateProduct :one
INSERT INTO products (
    id, archetype_id, name, description, tagline,
    status, product_type, currency_code, parent_product_id,
    provider_id, provider_name, regulator, license_number, regulatory_country,
    sharia_compliant, availability_mode, country_codes,
    ruleset, base_ruleset_ids, effective_from, effective_to, created_by
) VALUES (
    @id, @archetype_id, @name, @description, @tagline,
    @status, @product_type, @currency_code, @parent_product_id,
    @provider_id, @provider_name, @regulator, @license_number, @regulatory_country,
    @sharia_compliant, @availability_mode, @country_codes,
    @ruleset, @base_ruleset_ids, @effective_from, @effective_to, @created_by
) RETURNING *;

-- name: GetProduct :one
SELECT * FROM products WHERE id = @id;

-- name: ListProducts :many
SELECT * FROM products
WHERE (@archetype_id::text IS NULL OR archetype_id = @archetype_id)
  AND (@family_id::text IS NULL OR archetype_id IN (SELECT id FROM archetypes WHERE family_id = @family_id))
  AND (@status::text IS NULL OR status = @status::product_status)
  AND (@currency_code::text IS NULL OR currency_code = @currency_code)
  AND (@country_code::text IS NULL OR availability_mode = 'global' OR (availability_mode = 'specific_countries' AND @country_code = ANY(country_codes)) OR (availability_mode = 'global_except' AND NOT @country_code = ANY(country_codes)))
ORDER BY created_at
LIMIT CASE WHEN @result_limit::int > 0 THEN @result_limit ELSE 100 END
OFFSET @result_offset;

-- name: UpdateProduct :one
UPDATE products
SET name = @name,
    description = @description,
    tagline = @tagline,
    currency_code = @currency_code,
    sharia_compliant = @sharia_compliant,
    availability_mode = @availability_mode,
    country_codes = @country_codes,
    ruleset = @ruleset,
    base_ruleset_ids = @base_ruleset_ids,
    effective_from = @effective_from,
    effective_to = @effective_to,
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: TransitionProductStatus :one
UPDATE products SET status = @status, updated_at = now() WHERE id = @id RETURNING *;

-- name: ListAvailableProducts :many
SELECT * FROM products
WHERE status = 'active'
  AND (
    availability_mode = 'global'
    OR (availability_mode = 'specific_countries' AND @country_code::text = ANY(country_codes))
    OR (availability_mode = 'global_except' AND NOT @country_code::text = ANY(country_codes))
  )
  AND (@customer_type::text IS NULL OR id IN (SELECT product_id FROM customer_segments WHERE customer_type = @customer_type))
  AND (@family::text IS NULL OR archetype_id IN (SELECT a.id FROM archetypes a JOIN families f ON a.family_id = f.id WHERE f.family = @family::product_family))
ORDER BY created_at;
