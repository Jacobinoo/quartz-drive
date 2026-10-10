package httputils

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-playground/validator/v10"
)

var Validate = validator.New(validator.WithRequiredStructEnabled())

func ParseAndValidate(r *http.Request, dst any) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		slog.DebugContext(r.Context(), "json payload parser error", "error", err)
		return err
	}

	if err := Validate.Struct(dst); err != nil {
		slog.DebugContext(r.Context(), "json payload validation error", "error", err)
		return err
	}

	return nil
}
