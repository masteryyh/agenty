package shared

import (
	"fmt"
	"regexp"
)

// ModelID identifies a provider model. Unlike Slug, it may contain provider
// namespace separators and model variant markers used by compatible APIs.
type ModelID string

var modelIDPattern = regexp.MustCompile(`^[a-z][a-z0-9._\[\]-]*(?:/[a-z][a-z0-9._\[\]-]*)*$`)

func NewModelID(value string) (ModelID, error) {
	if !modelIDPattern.MatchString(value) {
		return "", fmt.Errorf("shared: invalid model id %q: must start with a lowercase letter and use only lowercase letters, digits, '-', '.', '_', '/', '[' and ']'", value)
	}
	return ModelID(value), nil
}

func (id ModelID) String() string {
	return string(id)
}

func (id ModelID) IsZero() bool {
	return id == ""
}

func (id ModelID) Valid() bool {
	return modelIDPattern.MatchString(string(id))
}
