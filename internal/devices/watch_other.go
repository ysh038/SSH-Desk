//go:build !linux

package devices

import (
	"context"
	"errors"
)

// Watch is only available on Linux.
func Watch(ctx context.Context, onChange func()) error { return errors.ErrUnsupported }
