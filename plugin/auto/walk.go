package auto

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/coredns/coredns/plugin/file"

	"github.com/miekg/dns"
)

// Walk will recursively walk of the file under l.directory and adds the one that match l.re.
func (a Auto) Walk() error {
	// TODO(miek): should add something so that we don't stomp on each other.

	toDelete := make(map[string]bool)
	for _, n := range a.Names() {
		toDelete[n] = true
	}

	filepath.Walk(a.directory, func(path string, info os.FileInfo, e error) error {
		if e != nil {
			log.Warningf("error reading %v: %v", path, e)
		}
		if info == nil || info.IsDir() {
			return nil
		}

		match, origin := matches(a.re, info.Name(), a.template)
		if !match {
			return nil
		}

		if z, ok := a.Z[origin]; ok {
			// we already have this zone
			toDelete[origin] = false
			z.SetFile(path)
			return nil
		}

		reader, err := os.Open(filepath.Clean(path))
		if err != nil {
			log.Warningf("Opening %s failed: %s", path, err)
			return nil
		}
		defer reader.Close()

		// Serial for loading a zone is 0, because it is a new zone.
		zo, err := file.Parse(reader, origin, path, 0)
		if err != nil {
			log.Warningf("Parse zone `%s': %v", origin, err)
			return nil
		}

		zo.ReloadInterval = a.ReloadInterval
		zo.Upstream = a.upstream

		a.Add(zo, origin, a.transfer)

		if a.metrics != nil {
			a.metrics.AddZone(origin)
		}

		log.Infof("Inserting zone `%s' from: %s", origin, path)

		toDelete[origin] = false

		return nil
	})

	for origin, ok := range toDelete {
		if !ok {
			continue
		}

		if a.metrics != nil {
			a.metrics.RemoveZone(origin)
		}

		a.Remove(origin)

		log.Infof("Deleting zone `%s'", origin)
	}

	return nil
}

// matches re to filename, if it is a match, the subexpression will be used to expand
// template to an origin. When match is true that origin is returned. Origin is fully qualified.
func matches(re *regexp.Regexp, filename, template string) (match bool, origin string) {
	base := filepath.Base(filename)

	matches := re.FindStringSubmatchIndex(base)
	if matches == nil {
		return false, ""
	}

	by := re.ExpandString(nil, template, base, matches)
	if by == nil {
		return false, ""
	}

	origin = dns.Fqdn(string(by))

	return true, origin
}
