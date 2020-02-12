package fanout

import "errors"

// Policy means specific policy
type Policy string

const (
	// FirstPositive means that fanout plugin will try to return first non-negative response from proxies.
	FirstPositive Policy = "first-positive"
	// Any means that fanout plugin will return any first response from proxies.
	Any Policy = "any"
)

// Validate checks that the policy is supported
func (p Policy) Validate() error {
	switch p {
	case Any, FirstPositive:
		return nil
	default:
		return errors.New("unknown policy")
	}
}
