package trace

import "github.com/ddh4r4m/saga/internal/schema"

func validatePins(v any) error { return schema.ValidateID(PinsSchema, v) }
