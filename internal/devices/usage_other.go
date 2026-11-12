//go:build !linux

package devices

import "errors"

// Usage is only available on Linux (the target); other systems build so
// that the pure logic can be tested there.
func Usage(dir string) (total, free int64, err error) {
	return 0, 0, errors.ErrUnsupported
}
