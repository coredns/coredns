package auto

import (
	"bytes"
	"io"
	golog "log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var dbFiles = []string{"db.example.org", "aa.example.org"}

const zoneContent = `; testzone
@	IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2016082534 7200 3600 1209600 3600
		NS	a.iana-servers.net.
		NS	b.iana-servers.net.

www IN A 127.0.0.1
`

func TestWalk(t *testing.T) {
	t.Parallel()
	tempdir, err := createFiles(t)
	if err != nil {
		t.Fatal(err)
	}

	ldr := loader{
		directory: tempdir,
		re:        regexp.MustCompile(`db\.(.*)`),
		template:  `${1}`,
	}

	a := Auto{
		loader: ldr,
		Zones:  &Zones{},
	}

	a.Walk()

	// db.example.org and db.example.com should be here (created in createFiles)
	for _, name := range []string{"example.com.", "example.org."} {
		if _, ok := a.Z[name]; !ok {
			t.Errorf("%s should have been added", name)
		}
	}
}

func TestWalkSymlinkedDirectory(t *testing.T) {
	t.Parallel()
	tempdir, err := createFiles(t)
	if err != nil {
		t.Fatal(err)
	}

	// Create a symlink to the directory containing zone files,
	// simulating a Kubernetes ConfigMap mount where the directory
	// itself is a symlink.
	symlinkDir := filepath.Join(t.TempDir(), "zones")
	if err := os.Symlink(tempdir, symlinkDir); err != nil {
		t.Fatal(err)
	}

	ldr := loader{
		directory: symlinkDir,
		re:        regexp.MustCompile(`db\.(.*)`),
		template:  `${1}`,
	}

	a := Auto{
		loader: ldr,
		Zones:  &Zones{},
	}

	a.Walk()

	for _, name := range []string{"example.com.", "example.org."} {
		if _, ok := a.Z[name]; !ok {
			t.Errorf("%s should have been added when directory is a symlink", name)
		}
	}
}

func TestWalkNonExistent(t *testing.T) {
	t.Parallel()
	nonExistingDir := "highly_unlikely_to_exist_dir"

	ldr := loader{
		directory: nonExistingDir,
		re:        regexp.MustCompile(`db\.(.*)`),
		template:  `${1}`,
	}

	a := Auto{
		loader: ldr,
		Zones:  &Zones{},
	}

	a.Walk()
}

func TestWalkWarnsForDuplicateOrigin(t *testing.T) {
	dir := t.TempDir()
	zone := filepath.Join(dir, "example.org.zone")
	backup := filepath.Join(dir, "example.org.zone.bak-20260528")

	for _, path := range []string{zone, backup} {
		if err := os.WriteFile(path, []byte(zoneContent), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var logBuf bytes.Buffer
	golog.SetOutput(&logBuf)
	defer golog.SetOutput(io.Discard)

	a := Auto{
		loader: loader{
			directory: dir,
			re:        regexp.MustCompile(`(.*)\.zone`),
			template:  `${1}`,
		},
		Zones: &Zones{},
	}

	a.Walk()

	got := logBuf.String()
	if count := strings.Count(got, `[WARNING] plugin/auto: Multiple zone files match origin "example.org."`); count != 1 {
		t.Fatalf("Expected one duplicate origin warning, got %d in %q", count, got)
	}
	for _, want := range []string{
		`[WARNING] plugin/auto: Multiple zone files match origin "example.org."`,
		"example.org.zone",
		"example.org.zone.bak-20260528",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Expected log to contain %q, got %q", want, got)
		}
	}

	logBuf.Reset()
	a.Walk()
	if count := strings.Count(logBuf.String(), `[WARNING] plugin/auto: Multiple zone files match origin "example.org."`); count != 1 {
		t.Fatalf("Expected one duplicate origin warning after reload, got %d in %q", count, logBuf.String())
	}
}

func TestWalkKeepsFirstMatchingFileForOrigin(t *testing.T) {
	dir := t.TempDir()
	// Match Walk: it stores paths after EvalSymlinks.
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	zone := filepath.Join(dir, "example.org.zone")
	backup := filepath.Join(dir, "example.org.zone.bak-20260528")

	for _, path := range []string{zone, backup} {
		if err := os.WriteFile(path, []byte(zoneContent), 0644); err != nil {
			t.Fatal(err)
		}
	}

	a := Auto{
		loader: loader{
			directory: dir,
			re:        regexp.MustCompile(`(.*)\.zone`),
			template:  `${1}`,
		},
		Zones: &Zones{},
	}

	a.Walk()
	if got := a.Z["example.org."].File(); got != zone {
		t.Fatalf("zone file = %q, want %q", got, zone)
	}

	a.Walk()
	if got := a.Z["example.org."].File(); got != zone {
		t.Fatalf("zone file after reload = %q, want %q", got, zone)
	}
}

func createFiles(t *testing.T) (string, error) {
	t.Helper()
	dir := t.TempDir()

	for _, name := range dbFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(zoneContent), 0644); err != nil {
			return dir, err
		}
	}
	// symlinks
	if err := os.Symlink(filepath.Join(dir, "db.example.org"), filepath.Join(dir, "db.example.com")); err != nil {
		return dir, err
	}
	if err := os.Symlink(filepath.Join(dir, "db.example.org"), filepath.Join(dir, "aa.example.com")); err != nil {
		return dir, err
	}

	return dir, nil
}
