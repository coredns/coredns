package fanout

import "errors"

// Policy means specific policy
type Policy string

const (
	FirstPositive Policy = "first-positive"
	Any           Policy = "any"
)

func (p Policy) Validate() error {
	switch p {
	case Any, FirstPositive:
		return nil
	default:
		return errors.New("unknown policy")
	}
}
