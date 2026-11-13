package models

import (
	"fmt"
	"strings"
)

const uuidLength = 36

// ValidateUUID checks that raw is a canonical 8-4-4-4-12 UUID and returns it
// lowercased. The field name is only used to build the error message, so the
// caller can point at the offending request parameter.
func ValidateUUID(field, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", Invalid(fmt.Sprintf("`%s` is required", field))
	}

	if len(value) != uuidLength {
		return "", Invalid(fmt.Sprintf("`%s` must be a UUID, got `%s`", field, raw))
	}

	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return "", Invalid(fmt.Sprintf("`%s` must be a UUID, got `%s`", field, raw))
			}
		default:
			isHex := (r >= '0' && r <= '9') ||
				(r >= 'a' && r <= 'f') ||
				(r >= 'A' && r <= 'F')
			if !isHex {
				return "", Invalid(fmt.Sprintf("`%s` must be a UUID, got `%s`", field, raw))
			}
		}
	}

	return strings.ToLower(value), nil
}
