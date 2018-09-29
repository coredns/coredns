package sarama

import (
	"reflect"
	"testing"
)

func TestBalanceStrategyRange(t *testing.T) {
	tests := []struct {
		members  map[string][]string
		topics   map[string][]int32
		expected BalanceStrategyPlan
	}{
		{
			members: map[string][]string{"M1": {"T1", "T2"}, "M2": {"T1", "T2"}},
			topics:  map[string][]int32{"T1": {0, 1, 2, 3}, "T2": {0, 1, 2, 3}},
			expected: BalanceStrategyPlan{
				"M1": map[string][]int32{"T1": {0, 1}, "T2": {2, 3}},
				"M2": map[string][]int32{"T1": {2, 3}, "T2": {0, 1}},
			},
		},
		{
			members: map[string][]string{"M1": {"T1", "T2"}, "M2": {"T1", "T2"}},
			topics:  map[string][]int32{"T1": {0, 1, 2}, "T2": {0, 1, 2}},
			expected: BalanceStrategyPlan{
				"M1": map[string][]int32{"T1": {0, 1}, "T2": {2}},
				"M2": map[string][]int32{"T1": {2}, "T2": {0, 1}},
			},
		},
		{
			members: map[string][]string{"M1": {"T1"}, "M2": {"T1", "T2"}},
			topics:  map[string][]int32{"T1": {0, 1}, "T2": {0, 1}},
			expected: BalanceStrategyPlan{
				"M1": map[string][]int32{"T1": {0}},
				"M2": map[string][]int32{"T1": {1}, "T2": {0, 1}},
			},
		},
	}

	strategy := BalanceStrategyRange
	if strategy.Name() != "range" {
		t.Errorf("Unexpected stategy name\nexpected: range\nactual: %v", strategy.Name())
	}

	for _, test := range tests {
		members := make(map[string]ConsumerGroupMemberMetadata)
		for memberID, topics := range test.members {
			members[memberID] = ConsumerGroupMemberMetadata{Topics: topics}
		}

		actual, err := strategy.Plan(members, test.topics)
		if err != nil {
			t.Errorf("Unexpected error %v", err)
		} else if !reflect.DeepEqual(actual, test.expected) {
			t.Errorf("Plan does not match expectation\nexpected: %#v\nactual: %#v", test.expected, actual)
		}
	}
}

func TestBalanceStrategyRoundRobin(t *testing.T) {
	tests := []struct {
		members  map[string][]string
		topics   map[string][]int32
		expected BalanceStrategyPlan
	}{
		{
			members: map[string][]string{"M1": {"T1", "T2"}, "M2": {"T1", "T2"}},
			topics:  map[string][]int32{"T1": {0, 1, 2, 3}, "T2": {0, 1, 2, 3}},
			expected: BalanceStrategyPlan{
				"M1": map[string][]int32{"T1": {0, 2}, "T2": {1, 3}},
				"M2": map[string][]int32{"T1": {1, 3}, "T2": {0, 2}},
			},
		},
		{
			members: map[string][]string{"M1": {"T1", "T2"}, "M2": {"T1", "T2"}},
			topics:  map[string][]int32{"T1": {0, 1, 2}, "T2": {0, 1, 2}},
			expected: BalanceStrategyPlan{
				"M1": map[string][]int32{"T1": {0, 2}, "T2": {1}},
				"M2": map[string][]int32{"T1": {1}, "T2": {0, 2}},
			},
		},
	}

	strategy := BalanceStrategyRoundRobin
	if strategy.Name() != "roundrobin" {
		t.Errorf("Unexpected stategy name\nexpected: range\nactual: %v", strategy.Name())
	}

	for _, test := range tests {
		members := make(map[string]ConsumerGroupMemberMetadata)
		for memberID, topics := range test.members {
			members[memberID] = ConsumerGroupMemberMetadata{Topics: topics}
		}

		actual, err := strategy.Plan(members, test.topics)
		if err != nil {
			t.Errorf("Unexpected error %v", err)
		} else if !reflect.DeepEqual(actual, test.expected) {
			t.Errorf("Plan does not match expectation\nexpected: %#v\nactual: %#v", test.expected, actual)
		}
	}
}
