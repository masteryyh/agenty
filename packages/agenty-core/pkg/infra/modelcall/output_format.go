package modelcall

import "strings"

func validateOutputFormat(format OutputFormat) error {
	if len(format.Name) == 0 || len(format.Name) > 64 {
		return invalidRequest("output format name must contain 1 to 64 ASCII letters, digits, underscores or hyphens")
	}
	for _, char := range format.Name {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-", char) {
			return invalidRequest("output format name contains an unsupported character")
		}
	}
	if format.Schema.Type != JSONSchemaTypeObject || len(format.Schema.AnyOf) > 0 {
		return invalidRequest("output format schema must have an object root without anyOf")
	}
	_, err := format.Schema.toMap()
	return err
}
