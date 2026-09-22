package test

import (
	"os"
	"testing"
)

func RunTestFile(t *testing.T, filename string) {
	t.Helper()

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	t.Logf("Test file opened successfully: %s", filename)
}

func TestTXTRecordSplit(t *testing.T) {
	RunTestFile(t, "testdata/txtrecordsplit.test")
}
