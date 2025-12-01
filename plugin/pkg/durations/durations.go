package durations

import (
	"fmt"
	"strconv"
	"time"
)

// NewDurationFromArg returns a time.Duration from a configuration argument
// (string) which has come from the Corefile. The argument has some basic
// validation applied before returning a time.Duration. If the argument has no
// time unit specified and is numeric the argument will be treated as seconds
// rather than GO's default of nanoseconds.
func NewDurationFromArg(arg string) (time.Duration, error) {
	_, err := strconv.Atoi(arg)
	if err == nil {
		arg = arg + "s"
	}

	d, err := time.ParseDuration(arg)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration '%s'", arg)
	}

	return d, nil
}
