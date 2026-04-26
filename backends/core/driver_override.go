package core

import (
	"os"
	"strings"

	"github.com/sockerless/api"
)

// Per-cloud-per-dimension driver overrides
//
// Operators select an alternate driver for a dimension via the env
// var `SOCKERLESS_<BACKEND>_<DIMENSION>=<impl>`. Examples:
//
//   SOCKERLESS_ECS_BUILD=Kaniko             # ECS Build → KanikoInContainer
//   SOCKERLESS_LAMBDA_FSDIFF=OverlayUpper   # Lambda FSDiff → overlay rootfs
//
// `<DIMENSION>` is the typed dimension name without the trailing
// `Driver` ("EXEC", "ATTACH", "FSREAD", "FSWRITE", "FSDIFF",
// "FSEXPORT", "COMMIT", "BUILD", "STATS", "PROCLIST", "LOGS",
// "SIGNAL", "REGISTRY"). `<impl>` is the implementation key
// registered by the backend's driver-init code.
//
// Backends register their alternates with `RegisterDriverImpl`
// during init; the `ResolveDriver*` helpers look up the env override
// and return either the chosen alternate or the default.

// driverImplKey is the identity for a registered driver implementation.
type driverImplKey struct {
	Backend   string // "ecs", "lambda", "aca", …
	Dimension string // "EXEC", "BUILD", "FSDIFF", …
	Impl      string // operator-facing key, e.g. "Kaniko", "OverlayUpper"
}

// driverImplRegistry is the per-process registry of (backend,
// dimension, impl) → factory. Initialized lazily; backends call
// RegisterDriverImpl during init.
var driverImplRegistry = map[driverImplKey]func() Driver{}

// RegisterDriverImpl records a driver alternate for a (backend,
// dimension, impl) triple. Called by per-backend driver-init code:
//
//	core.RegisterDriverImpl("ecs", "BUILD", "Kaniko", func() core.Driver { return &kanikoBuildDriver{...} })
//
// At construction time the backend calls
// `core.ResolveDriverFor("BUILD", defaultBuild)` to get either the
// operator-selected alternate or the default driver.
func RegisterDriverImpl(backend, dimension, impl string, factory func() Driver) {
	driverImplRegistry[driverImplKey{
		Backend:   backend,
		Dimension: strings.ToUpper(dimension),
		Impl:      impl,
	}] = factory
}

// ResolveDriverFor returns the operator-overridden driver for a
// (backend, dimension) pair if an override is registered and the
// SOCKERLESS_<BACKEND>_<DIMENSION> env var names a known impl.
// Otherwise returns the supplied default. If the env var names an
// UNKNOWN impl, returns nil — backend init code surfaces a startup
// error via the second return value.
//
// Example:
//
//	var build BuildDriver = &codeBuildDriver{...}     // default
//	if alt, ok, err := core.ResolveDriverFor("ecs", "BUILD"); err != nil {
//	    return err
//	} else if ok {
//	    build = alt.(BuildDriver)
//	}
func ResolveDriverFor(backend, dimension string) (Driver, bool, error) {
	dim := strings.ToUpper(dimension)
	envVar := "SOCKERLESS_" + strings.ToUpper(backend) + "_" + dim
	impl := os.Getenv(envVar)
	if impl == "" {
		return nil, false, nil
	}
	factory, ok := driverImplRegistry[driverImplKey{
		Backend:   backend,
		Dimension: dim,
		Impl:      impl,
	}]
	if !ok {
		return nil, false, &api.InvalidParameterError{Message: envVar + "=" + impl + ": unknown driver impl; see backend init for the registered keys"}
	}
	return factory(), true, nil
}

// NotImplDriverError formats the canonical error returned when a
// dimension's driver is missing or returns a NotImpl. Format:
//
//	docker <action> requires <Describe()>; configured driver: <name>
//
// Example output:
//
//	"docker exec requires a reverse-agent bootstrap inside the
//	 Lambda container (SOCKERLESS_CALLBACK_URL); no session
//	 registered. Configured driver: lambda ReverseAgentExec"
//
// Centralising the message shape keeps every NotImpl response
// shaped the same way across all backends + dimensions, with no
// phase or bug references leaking into the operator-visible error.
// Pass the typed driver's `Describe()` value as `description`.
func NotImplDriverError(action, description string) *api.NotImplementedError {
	return &api.NotImplementedError{Message: "docker " + action + " requires " + description}
}
