package shared

import (
	"fmt"
	"regexp"
)

var slugPattern = regexp.MustCompile(`^[a-z]+[a-z0-9]*(?:[-._][a-z0-9]+)*$`)

type Slug string

func NewSlug(s string) (Slug, error) {
	if !slugPattern.MatchString(s) {
		return "", fmt.Errorf("shared: invalid slug %q: must start with a lowercase letter and use only lowercase letters, digits, '-', '.' and '_'", s)
	}
	return Slug(s), nil
}

func (s Slug) String() string {
	return string(s)
}

func (s Slug) IsZero() bool {
	return s == ""
}

func (s Slug) Valid() bool {
	return slugPattern.MatchString(string(s))
}
