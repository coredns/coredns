package request

// An optionalBool is a bool value that may not be set and is a replacement for
// using a *bool pointer.
type optionalBool int

const (
	boolUnset optionalBool = iota
	boolFalse
	boolTrue
)

func (o *optionalBool) Set(b bool) {
	if b {
		*o = boolTrue
	} else {
		*o = boolFalse
	}
}

func (o optionalBool) IsSet() bool { return o != boolUnset }
func (o optionalBool) Value() bool { return o == boolTrue }
