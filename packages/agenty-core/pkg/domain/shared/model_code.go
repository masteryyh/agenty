package shared

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// ModelCode identifies a provider model. It is kept as an opaque upstream
// identifier because compatible APIs may use path separators and other
// characters that are unsafe or ambiguous in a file name.
type ModelCode string

func NewModelCode(value string) (ModelCode, error) {
	if value == "" || !utf8.ValidString(value) {
		return "", fmt.Errorf("shared: invalid model code %q: must be a non-empty valid UTF-8 value", value)
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", fmt.Errorf("shared: invalid model code %q: whitespace and control characters are not allowed", value)
		}
	}
	return ModelCode(value), nil
}

func (code ModelCode) String() string {
	return string(code)
}

func (code ModelCode) IsZero() bool {
	return code == ""
}

func (code ModelCode) Valid() bool {
	_, err := NewModelCode(code.String())
	return err == nil
}
