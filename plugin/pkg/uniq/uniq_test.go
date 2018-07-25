package uniq

import "testing"

func TestForEach(t *testing.T) {
	u, i := New(), 0
	u.Set("test", func() error { i++; return nil })

	if !u.HasTodo() {
		t.Errorf("Element %s - is not set as a 'todo'", "test")
	}
	u.ForEach()
	if i != 1 {
		t.Errorf("Failed to executed f for %s", "test")
	}
	if u.HasTodo() {
		t.Errorf("Element %s - is not change to 'done'", "test")
	}
	u.ForEach()
	if i != 1 {
		t.Errorf("Executed f twice instead of once")
	}
}
