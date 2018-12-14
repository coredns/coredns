package coresetup

import (
	"fmt"
	"strconv"
	"time"
)

type Type interface {
	Parse(string) (Value, error)
}

type Int struct {
	Min     int
	Max     int
	Default *int
}

func DefaultInt(i int) *int                          { return &i }
func DefaultString(s string) *string                 { return &s }
func DefaultDuration(t time.Duration) *time.Duration { return &t }

func (i Int) Parse(x string) (Value, error) {
	j, err := strconv.ParseInt(x, 10, 32)
	if err != nil {
		return nil, err
	}

	if i.Min >= 0 && j <= int64(i.Min) {
		return nil, fmt.Errorf("integer value lower than minimal %d: %q", i.Min, x)
	}
	if i.Max >= 0 && j > int64(i.Max) {
		return nil, fmt.Errorf("integer value larger than maximum %d: %q", i.Max, x)
	}
	ji := int(j)
	return v{Int: &ji}, nil
}

type Duration struct {
	Default *time.Duration
}

func (d Duration) Parse(x string) (Value, error) {
	duration, err := time.ParseDuration(x)
	if err != nil {
		return nil, err
	}
	return v{Duration: &duration}, nil
}

type String struct {
	Default *string
}

func (s String) Parse(x string) (Value, error) {
	return v{String: &x}, nil
}
