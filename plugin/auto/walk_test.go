package auto

import (
	"os"
	"path/filepath"
	"regexp"
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
