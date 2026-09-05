package utils

import (
	"encoding/base64"
	"fmt"
)

// Base64RawUrlDecodedByteLengthCompareWith compares the decoded base 64 length with compareWith parameter.
// Returns nil if comparison successful, otherwise error.
func Base64RawUrlDecodedByteLengthCompareWith(b64 string, compareWith int) error {
	decoded, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("base64 decoding failed")
	}
	if len(decoded) != compareWith {
		return fmt.Errorf("base64 decoded length mismatch, got %d, expected %d", len(decoded), compareWith)
	}
	return nil
}
