// Package readfile reads a file named from a Corefile with a size cap, so a
// malformed or unexpectedly large configuration or credential file is rejected
// instead of being buffered and parsed in full.
package readfile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DefaultLimit is the largest configuration or credential file a plugin reads
// during setup or reload: 8 MiB. Files that plugins document as a zone-like
// table (the loadbalance weights file) fit well below it, and a credential
// JSON document is a few KiB at most.
const DefaultLimit int64 = 8 << 20

// ErrTooLarge is returned by Read when the file is larger than the limit.
var ErrTooLarge = errors.New("file exceeds the size limit")

// Read returns the contents of name. It returns an error wrapping ErrTooLarge
// when the file is larger than limit, and never buffers more than limit+1
// bytes even when the size reported by the file system is stale or the file
// grows while it is read.
func Read(name string, limit int64) ([]byte, error) {
	path := filepath.Clean(name)
	file, err := os.Open(path) // #nosec G304 -- the path comes from the Corefile, which is operator-controlled.
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if info, err := file.Stat(); err == nil && info.Size() > limit {
		return nil, fmt.Errorf("%s: %w (%d bytes, limit %d)", path, ErrTooLarge, info.Size(), limit)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s: %w (more than %d bytes)", path, ErrTooLarge, limit)
	}
	return data, nil
}
