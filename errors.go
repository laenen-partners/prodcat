package prodcat

import "errors"

var (
	ErrNotFound        = errors.New("prodcat: not found")
	ErrAlreadyExists   = errors.New("prodcat: already exists")
	ErrRulesetDisabled = errors.New("prodcat: ruleset is disabled")
	ErrValidation      = errors.New("prodcat: validation error")
)
