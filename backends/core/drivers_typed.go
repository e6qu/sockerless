package core

import (
	"io"

	"github.com/sockerless/api"
)

// Cross-backend typed driver framework.
//
// Thirteen typed driver dimensions covering every "perform docker action
// X against the cloud" decision sockerless makes. Interfaces live here in
// `backends/core`; default implementations and per-cloud implementations
// live in `backends/<cloud>/drivers/` and `backends/<cloud>-common/drivers/`.
// Each backend constructs its `TypedDriverSet` at startup and operators
// override per-cloud-per-dimension via
// `SOCKERLESS_<BACKEND>_<DIMENSION>=<impl>` (resolved by the override
// registry in `driver_override.go`).
//
// The typed dimensions are kept distinct from the existing narrow
// `core.DriverSet` (Exec / Filesystem / Stream / Network) so the lift can
// run dimension-by-dimension with no behaviour change per commit. Each
// lift moves one dimension's existing implementation into the typed
// shape and switches the BaseServer dispatch site to call through the
// typed driver. The narrow `DriverSet` is removed once the last
// dimension is absorbed.

// ExecDriver lifts the narrow LegacyExecDriver into the typed shape.
// Implementations: docker→DockerExec; ECS→SSMExec;
// Lambda/CR/GCF/AZF→ReverseAgentExec; ACA→ACAConsoleExec ⇄
// ReverseAgentExec.
type ExecDriver interface {
	Driver
	// Exec runs a command in the container and streams I/O over the
	// caller-supplied io.ReadWriter (typically the hijacked HTTP
	// connection). Returns the exit code or an error if the
	// transport could not be established. Honour ctx for
	// cancellation.
	Exec(dctx DriverContext, opts ExecOptions, conn io.ReadWriter) (exitCode int, err error)
}

// ExecOptions is the per-call exec configuration.
type ExecOptions struct {
	ExecID  string
	Cmd     []string
	Env     []string
	WorkDir string
	TTY     bool
	User    string
}

// AttachDriver lifts the docker-attach path into the typed shape.
// Implementations: docker→DockerAttach; ECS→CloudWatchAttach;
// FaaS+CR+ACA→CloudLogsReadOnlyAttach (lifts `core.AttachViaCloudLogs`
// into a typed driver).
type AttachDriver interface {
	Driver
	Attach(dctx DriverContext, tty bool, conn io.ReadWriter) error
}

// FSReadDriver lifts the cp-from-container / stat / get-archive paths.
// Implementations: docker→DockerArchive; ECS→SSMTar;
// FaaS+CR+ACA→ReverseAgentTar.
//
// The overlay-rootfs alternate `OverlayUpperRead` ships under this
// dimension once the typed-driver framework lands fully.
type FSReadDriver interface {
	Driver
	GetArchive(dctx DriverContext, path string, w io.Writer) error
	StatPath(dctx DriverContext, path string) (*api.ContainerPathStat, error)
}

// FSWriteDriver lifts the cp-to-container / put-archive paths.
// Implementations: docker→DockerArchive; ECS→SSMTarExtract;
// FaaS+CR+ACA→ReverseAgentTarExtract; alternate `OverlayUpperWrite`.
type FSWriteDriver interface {
	Driver
	PutArchive(dctx DriverContext, path string, body io.Reader, noOverwriteDirNonDir bool) error
}

// FSDiffDriver lifts the docker-diff (find-newer) path.
// Implementations: docker→DockerChanges; ECS→SSMFindNewer;
// FaaS+CR+ACA→ReverseAgentFindNewer; alternate `OverlayUpperDiff`
// closes the deletion-not-captured limitation.
type FSDiffDriver interface {
	Driver
	Changes(dctx DriverContext) ([]api.ContainerChangeItem, error)
}

// FSExportDriver lifts the docker-export path.
// Implementations: docker→DockerExport; ECS→SSMTarRoot;
// FaaS+CR+ACA→ReverseAgentTarRoot; alternate `OverlayMergedExport`.
type FSExportDriver interface {
	Driver
	Export(dctx DriverContext, w io.Writer) error
}

// CommitDriver lifts the docker-commit path.
// Implementations: docker→DockerCommit;
// FaaS+CR+ACA→ReverseAgentTarLayer+Push; ECS→accepted-gap NotImpl
// (no Fargate host fs); alternate `OverlayLayerCommit` closes the
// ECS gap.
type CommitDriver interface {
	Driver
	Commit(dctx DriverContext, opts CommitOptions) (imageID string, err error)
}

// CommitOptions matches `api.ContainerCommitRequest` minus the
// container reference (which is in DriverContext).
type CommitOptions struct {
	Author  string
	Comment string
	Repo    string
	Tag     string
	Pause   bool
	Changes []string
	Config  *api.ContainerConfig
}

// BuildDriver lifts `docker build` into the typed shape.
// Implementations: docker→LocalDockerBuild; ECS+Lambda→CodeBuild;
// CR+GCF→CloudBuild; ACA+AZF→ACRTasks. Alternate
// `KanikoInContainer`, `BuildKitRemote` plug in here.
//
// Build takes `api.ImageBuildOptions` directly — no projected subset —
// so no field is silently dropped between the handler and the impl.
type BuildDriver interface {
	Driver
	Build(dctx DriverContext, opts api.ImageBuildOptions, ctxReader io.Reader) (io.ReadCloser, error)
	// Available reports whether the underlying build service is
	// reachable / configured. Returning false routes to a
	// `NotImplementedError` with a clear missing-prerequisite
	// message; never falls back to a synthetic local-Dockerfile
	// parser unless the operator opts in via
	// `SOCKERLESS_LOCAL_DOCKERFILE_BUILD=1`.
	Available() bool
}

// StatsDriver lifts the docker-stats path.
// Implementations: docker→DockerStats; AWS→CloudWatchAggregate;
// GCP→CloudMonitoring; Azure→LogAnalytics. Alternate
// `CloudWatchInsightsRich` plugs in.
type StatsDriver interface {
	Driver
	Stats(dctx DriverContext, stream bool, w io.Writer) error
}

// ProcListDriver lifts the docker-top path.
// Implementations: docker→DockerTop; ECS→SSMPs;
// FaaS+CR+ACA→ReverseAgentPs.
type ProcListDriver interface {
	Driver
	Top(dctx DriverContext, psArgs string) (*api.ContainerTopResponse, error)
}

// LogsDriver lifts the docker-logs path.
// Implementations: docker→DockerLogs; AWS→CloudWatch;
// GCP→CloudLogging; Azure→LogAnalytics.
type LogsDriver interface {
	Driver
	Logs(dctx DriverContext, opts api.ContainerLogsOptions) (io.ReadCloser, error)
}

// SignalDriver lifts the docker-pause / unpause / kill paths.
// Implementations: docker→DockerKill; ECS→SSMKill;
// FaaS+CR+ACA→ReverseAgentKill.
type SignalDriver interface {
	Driver
	// Kill sends `signal` to the container's main process. For
	// pause/unpause backends translate to SIGSTOP/SIGCONT.
	Kill(dctx DriverContext, signal string) error
}

// RegistryDriver lifts the image push / pull paths.
// Implementations: per-cloud — ECRPullThrough+ECRPush;
// ARPullThrough+ARPush; ACRCacheRule+ACRPush.
//
// Both Push and Pull take a parsed `ImageRef` (the handler parses the
// raw `<domain>/<path>[:<tag>][@<digest>]` string at the dispatch
// boundary, exactly once) and return an io.ReadCloser streaming the
// OCI progress JSON the docker client expects.
type RegistryDriver interface {
	Driver
	Push(dctx DriverContext, ref ImageRef, auth string) (io.ReadCloser, error)
	Pull(dctx DriverContext, ref ImageRef, auth string) (io.ReadCloser, error)
}

// TypedDriverSet aggregates all 13 typed drivers. Backends construct
// this at startup; the BaseServer's dispatch sites call through
// these interfaces.
type TypedDriverSet struct {
	Exec     ExecDriver
	Attach   AttachDriver
	FSRead   FSReadDriver
	FSWrite  FSWriteDriver
	FSDiff   FSDiffDriver
	FSExport FSExportDriver
	Commit   CommitDriver
	Build    BuildDriver
	Stats    StatsDriver
	ProcList ProcListDriver
	Logs     LogsDriver
	Signal   SignalDriver
	Registry RegistryDriver
}
