//go:build !linux

package core

import (
	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

// NewPlatformNetworkDriver returns nil on non-Linux platforms,
// causing the server to use the synthetic driver directly.
func NewPlatformNetworkDriver(_ *SyntheticNetworkDriver, _ zerolog.Logger) api.NetworkDriver {
	return nil
}
