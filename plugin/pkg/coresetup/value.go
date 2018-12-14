package coresetup

import "time"

type Value interface {
	IntValue() int
	StringValue() string
	DurationValue() time.Duration
}

type v struct {
	Int      *int
	String   *string
	Duration *time.Duration
}

func (v v) IntValue() int {
	if v.Int != nil {
		return *v.Int
	}
	return 0
}

func (v v) StringValue() string {
	if v.String != nil {
		return *v.String
	}
	return ""
}

func (v v) DurationValue() time.Duration {
	if v.Duration != nil {
		return *v.Duration
	}
	return 0
}
