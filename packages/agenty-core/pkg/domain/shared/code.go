package shared

import (
	"fmt"
	"regexp"
)

var codePattern = regexp.MustCompile(`^[a-z]+[a-z0-9]*(?:[-._][a-z0-9]+)*$`)

type Code string

func NewCode(s string) (Code, error) {
	if !codePattern.MatchString(s) {
		return "", fmt.Errorf("shared: invalid code %q: must start with a lowercase letter and use only lowercase letters, digits, '-', '.' and '_'", s)
	}
	return Code(s), nil
}

func (s Code) String() string {
	return string(s)
}

func (s Code) IsZero() bool {
	return s == ""
}

func (s Code) Valid() bool {
	return codePattern.MatchString(string(s))
}
