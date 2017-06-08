package fifo

import (
	"testing"
	"time"
)

func TestSize(t *testing.T) {
	f := NewFifo(3)

	sizeCheck(t, f, 0)

	f.Push(time.Now())
	sizeCheck(t, f, 1)

	f.Push(time.Now())
	f.Push(time.Now())
	sizeCheck(t, f, 3)

	f.Push(time.Now())
	sizeCheck(t, f, 3)
}

func TestFrequency(t *testing.T) {
	f := NewFifo(3)

	f.Push(time.Now())
	f.Push(time.Now())
	f.Push(time.Now())

	// 3 events added, frequence for the last *minute*, should return them all:
	start := time.Now().Add(-1 * time.Minute)
	freqCheck(t, f, 3, start)
}

func TestFrequencyOld(t *testing.T) {
	f := NewFifo(3)

	var history time.Time

	history = time.Now().Add(-15 * time.Minute)
	f.Push(history)
	history = time.Now().Add(-15 * time.Minute)
	f.Push(history)
	history = time.Now().Add(-15 * time.Minute)
	f.Push(history)

	start := time.Now().Add(-1 * time.Minute)
	freqCheck(t, f, 0, start)
}

func TestFrequencyFew(t *testing.T) {
	f := NewFifo(3)

	var history time.Time

	history = time.Now().Add(-18 * time.Minute)
	f.Push(history)
	history = time.Now().Add(-17 * time.Minute)
	f.Push(history)
	history = time.Now().Add(-12 * time.Minute)
	f.Push(history)

	start := time.Now().Add(-13 * time.Minute)
	freqCheck(t, f, 1, start)
}

func sizeCheck(t *testing.T, f *Fifo, expected int) {
	if x := f.Size(); x != expected {
		t.Fatalf("Expected size to be %d, got %d", expected, x)
	}
}

func freqCheck(t *testing.T, f *Fifo, expected int, start time.Time) {
	if x := f.Frequency(start, time.Now()); x != expected {
		t.Fatalf("Expected frequence to be %d, got %d", expected, x)
	}
}
