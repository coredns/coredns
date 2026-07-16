package tsig

import "context"

type validatedKeyNameKey struct{}

// ValidatedKeyName returns the normalized name of the TSIG key validated by
// the tsig plugin. The boolean is false for unsigned requests and requests the
// plugin did not validate.
func ValidatedKeyName(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(validatedKeyNameKey{}).(string)
	return name, ok
}

func withValidatedKeyName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, validatedKeyNameKey{}, name)
}
