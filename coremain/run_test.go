package coremain

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/coredns/caddy"
)

func TestConfLoader(t *testing.T) {
	tests := []struct {
		name          string
		conf          string
		contents      []byte
		serverType    string
		expectedError bool
		expectedInput bool
		checkContents bool
	}{
		{
			name:          "empty conf",
			conf:          "",
			serverType:    "dns",
			expectedError: false,
			expectedInput: false,
		},
		{
			name:          "non-existent file",
			conf:          "non-existent-file",
			serverType:    "dns",
			expectedError: true,
			expectedInput: false,
		},
		{
			name:          "stdin input",
			conf:          "stdin",
			serverType:    "dns",
			expectedError: false,
			expectedInput: false,
		},
		{
			name:          "valid config file",
			contents:      []byte("example.org:53 {\n    whoami\n}\n"),
			serverType:    "dns",
			expectedError: false,
			expectedInput: true,
			checkContents: true,
		},
		{
			name:          "empty config file",
			conf:          "",
			contents:      []byte(""),
			serverType:    "dns",
			expectedError: false,
			expectedInput: false,
			checkContents: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.contents) > 0 {
				tmpDir := t.TempDir()
				tmpFile := filepath.Join(tmpDir, "corefile-test")
				if err := os.WriteFile(tmpFile, tc.contents, 0o644); err != nil {
					t.Fatalf("Failed to write to temp file: %v", err)
				}
				conf = tmpFile
			} else {
				conf = tc.conf
			}

			input, err := confLoader(tc.serverType)

			// Check error
			if tc.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tc.expectedError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			// Check input
			if !tc.expectedInput && input != nil {
				t.Errorf("Expected nil input but got: %v", input)
			}
			if tc.expectedInput && input == nil {
				t.Errorf("Expected non-nil input but got nil")
			}

			// Check contents if needed
			if tc.checkContents && input != nil {
				caddyInput, ok := input.(caddy.CaddyfileInput)
				if !ok {
					t.Errorf("Expected input to be caddy.CaddyfileInput")
				}
				if string(caddyInput.Contents) != string(tc.contents) {
					t.Errorf("Expected contents %q, got %q", tc.contents, caddyInput.Contents)
				}
				if caddyInput.ServerTypeName != tc.serverType {
					t.Errorf("Expected ServerTypeName to be %q, got %q", tc.serverType, caddyInput.ServerTypeName)
				}
			}
		})
	}
}

func TestDefaultLoader(t *testing.T) {
	// The working directory matters because defaultLoader() looks for "Corefile" in the current directory
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Test without Corefile
	input, err := defaultLoader("dns")
	if err != nil {
		t.Errorf("Expected no error for missing Corefile, got: %v", err)
	}
	if input != nil {
		t.Errorf("Expected nil input for missing Corefile, got: %v", input)
	}

	// Test with Corefile
	testContents := []byte("example.org:53 {\n    whoami\n}\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "Corefile"), testContents, 0o644); err != nil {
		t.Fatalf("Failed to write Corefile: %v", err)
	}

	input, err = defaultLoader("dns")
	if err != nil {
		t.Errorf("Expected no error for valid Corefile, got: %v", err)
	}
	if input == nil {
		t.Errorf("Expected non-nil input for valid Corefile")
	} else {
		caddyInput, ok := input.(caddy.CaddyfileInput)
		if !ok {
			t.Errorf("Expected input to be caddy.CaddyfileInput")
		}
		if string(caddyInput.Contents) != string(testContents) {
			t.Errorf("Expected contents %q, got %q", testContents, caddyInput.Contents)
		}
		if caddyInput.ServerTypeName != "dns" {
			t.Errorf("Expected ServerTypeName to be %q, got %q", "dns", caddyInput.ServerTypeName)
		}
	}

	if runtime.GOOS != "windows" {
		// Create a file but make it unreadable
		tmpFile := filepath.Join(tmpDir, "Corefile")
		if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		if err := os.Chmod(tmpFile, 0000); err != nil {
			t.Fatalf("Failed to change permissions: %v", err)
		}

		input, err = defaultLoader("dns")
		if err == nil {
			t.Error("Expected error for unreadable Corefile but got none")
		}
		if input != nil {
			t.Error("Expected nil input for unreadable Corefile")
		}
	}
}

func TestVersionString(t *testing.T) {
	caddy.AppName = "TestApp"
	caddy.AppVersion = "1.0.0"

	expected := "TestApp-1.0.0\n"
	result := versionString()

	if result != expected {
		t.Errorf("Expected version string %q, got %q", expected, result)
	}
}

func TestReleaseString(t *testing.T) {
	GitCommit = "a6d2d7b5"

	expected := runtime.GOOS + "/" + runtime.GOARCH + ", " + runtime.Version() + ", " + GitCommit + "\n"
	result := releaseString()

	if result != expected {
		t.Errorf("Expected release string %q, got %q", expected, result)
	}
}

func TestSetVersion(t *testing.T) {
	// Test case 1: Development build with nearest tag
	gitTag = ""
	gitNearestTag = "v1.2.3"
	gitShortStat = "1 file changed"
	GitCommit = "abcdef"
	buildDate = "2023-05-01"

	setVersion()

	if !devBuild {
		t.Errorf("Expected devBuild to be true with empty gitTag and non-empty gitShortStat")
	}

	expectedAppVersion := "1.2.3 (+abcdef 2023-05-01)"
	if appVersion != expectedAppVersion {
		t.Errorf("Expected appVersion to be %q, got %q", expectedAppVersion, appVersion)
	}

	// Test case 2: Release build with tag
	gitTag = "v2.0.0"
	gitNearestTag = "v1.9.0"
	gitShortStat = ""

	setVersion()

	if devBuild {
		t.Errorf("Expected devBuild to be false with non-empty gitTag and empty gitShortStat")
	}

	expectedAppVersion = "2.0.0"
	if appVersion != expectedAppVersion {
		t.Errorf("Expected appVersion to be %q, got %q", expectedAppVersion, appVersion)
	}

	// Test case 3: No tags available
	gitTag = ""
	gitNearestTag = ""
	gitShortStat = ""
	appVersion = "(untracked dev build)" // Reset to default

	setVersion()

	if !devBuild {
		t.Errorf("Expected devBuild to be true with empty gitTag")
	}

	expectedAppVersion = "(untracked dev build)"
	if appVersion != expectedAppVersion {
		t.Errorf("Expected appVersion to be %q, got %q", expectedAppVersion, appVersion)
	}
}

func TestShowVersion(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Set test values
	caddy.AppName = "TestApp"
	caddy.AppVersion = "1.0.0"
	GitCommit = "abc123"

	// Test case 1: Non-dev build
	devBuild = false
	gitShortStat = ""
	gitFilesModified = ""

	showVersion()

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	result := buf.String()

	expected := fmt.Sprintf("TestApp-1.0.0\n%s/%s, %s, abc123\n", runtime.GOOS, runtime.GOARCH, runtime.Version())
	if result != expected {
		t.Errorf("Expected version output %q, got %q", expected, result)
	}

	// Test case 2: Dev build with modified files
	oldStdout = os.Stdout
	r, w, _ = os.Pipe()
	os.Stdout = w

	devBuild = true
	gitShortStat = "1 file changed, 10 insertions(+), 5 deletions(-)"
	gitFilesModified = "path/to/modified/file.go"

	showVersion()

	w.Close()
	os.Stdout = oldStdout

	buf.Reset()
	io.Copy(&buf, r)
	result = buf.String()

	expected = fmt.Sprintf("TestApp-1.0.0\n%s/%s, %s, abc123\n1 file changed, 10 insertions(+), 5 deletions(-)\npath/to/modified/file.go\n",
		runtime.GOOS, runtime.GOARCH, runtime.Version())
	if result != expected {
		t.Errorf("Expected version output with dev build info:\n%q\ngot:\n%q", expected, result)
	}
}
