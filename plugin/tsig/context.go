package tsig

import "context"

type validatedKey struct{}

// Validated reports whether the tsig plugin successfully validated a TSIG
// record on the request.
func Validated(ctx context.Context) bool {
	validated, _ := ctx.Value(validatedKey{}).(bool)
	return validated
}

func withValidatedTSIG(ctx context.Context) context.Context {
	return context.WithValue(ctx, validatedKey{}, true)
}
