package ecs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// resolveTaskARNForOps resolves the container's ECS task ARN. Returns
// "" if the container has no associated task (not yet started, or
// already stopped). Helpers below use this to decide between
// SSM-routed work and a NotImplementedError.
func (s *Server) resolveTaskARNForOps(containerID string) string {
	ecsState, ok := s.resolveTaskState(s.ctx(), containerID)
	if !ok {
		return ""
	}
	return ecsState.TaskARN
}

// runViaSSMOrNotImpl is shared boilerplate: resolve container, resolve
// task, run the command, return ssm output or a uniform error.
func (s *Server) runViaSSMOrNotImpl(containerID, cmd string, stdin []byte) (stdout, stderr []byte, exitCode int, _ error) {
	c, ok := s.ResolveContainerAuto(context.Background(), containerID)
	if !ok {
		return nil, nil, -1, &api.NotFoundError{Resource: "container", ID: containerID}
	}
	taskARN := s.resolveTaskARNForOps(c.ID)
	if taskARN == "" {
		return nil, nil, -1, &api.NotImplementedError{Message: "ECS operation requires a running task; container has no active execution"}
	}
	out, errOut, code, err := s.RunCommandViaSSM(taskARN, cmd, stdin)
	if err != nil {
		return nil, nil, -1, &api.ServerError{Message: fmt.Sprintf("SSM exec: %v", err)}
	}
	return out, errOut, code, nil
}

// ContainerTopViaSSM runs `ps <psArgs>` inside the running task via
// SSM and parses the output using the shared core helper.
func (s *Server) ContainerTopViaSSM(containerID, psArgs string) (*api.ContainerTopResponse, error) {
	if psArgs == "" {
		psArgs = "-ef"
	}
	stdout, stderr, exit, err := s.runViaSSMOrNotImpl(containerID, "ps "+psArgs, nil)
	if err != nil {
		return nil, err
	}
	if exit != 0 {
		return nil, &api.ServerError{Message: fmt.Sprintf("ps failed (exit %d): %s", exit, strings.TrimSpace(string(stderr)))}
	}
	return core.ParseTopOutput(string(stdout)), nil
}

// ContainerChangesViaSSM walks the container's rootfs via `find` and
// returns the post-boot diff. Same shape as the reverse-agent path
// (Phase 98 / BUG-753); the shell command is identical.
func (s *Server) ContainerChangesViaSSM(containerID string) ([]api.ContainerChangeItem, error) {
	cmd := `find / -xdev -newer /proc/1 -printf '%y\t%p\n'`
	stdout, stderr, exit, err := s.runViaSSMOrNotImpl(containerID, cmd, nil)
	if err != nil {
		return nil, err
	}
	if exit != 0 {
		return nil, &api.ServerError{Message: fmt.Sprintf("find failed (exit %d): %s", exit, strings.TrimSpace(string(stderr)))}
	}
	return core.ParseChangesOutput(string(stdout)), nil
}

// ContainerStatPathViaSSM runs `stat` on the given path inside the
// task and parses the result.
func (s *Server) ContainerStatPathViaSSM(containerID, path string) (*api.ContainerPathStat, error) {
	cmd := fmt.Sprintf(`stat -c '%%n\t%%s\t%%f\t%%Y\t%%N' %s`, shellQuote(path))
	stdout, stderr, exit, err := s.runViaSSMOrNotImpl(containerID, cmd, nil)
	if err != nil {
		return nil, err
	}
	if exit != 0 {
		return nil, &api.NotFoundError{Resource: "path", ID: path + ": " + strings.TrimSpace(string(stderr))}
	}
	return core.ParseStatOutput(string(stdout), path)
}

// ContainerGetArchiveViaSSM tars the requested path inside the task
// and returns the tarball + stat header. Buffered in memory — same
// tradeoff as the reverse-agent path.
func (s *Server) ContainerGetArchiveViaSSM(containerID, path string) (*api.ContainerArchiveResponse, error) {
	stat, err := s.ContainerStatPathViaSSM(containerID, path)
	if err != nil {
		return nil, err
	}
	parent := dirOf(path)
	name := baseOf(path)
	cmd := fmt.Sprintf(`tar -cf - -C %s %s`, shellQuote(parent), shellQuote(name))
	stdout, stderr, exit, ferr := s.runViaSSMOrNotImpl(containerID, cmd, nil)
	if ferr != nil {
		return nil, ferr
	}
	if exit != 0 {
		return nil, &api.ServerError{Message: fmt.Sprintf("tar failed (exit %d): %s", exit, strings.TrimSpace(string(stderr)))}
	}
	return &api.ContainerArchiveResponse{
		Stat:   *stat,
		Reader: io.NopCloser(bytes.NewReader(stdout)),
	}, nil
}

// ContainerPutArchiveViaSSM extracts the incoming tar body into <path>
// inside the task by piping it as stdin to `tar -xf - -C <path>`.
func (s *Server) ContainerPutArchiveViaSSM(containerID, path string, body io.Reader) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return &api.ServerError{Message: fmt.Sprintf("read archive body: %v", err)}
	}
	cmd := fmt.Sprintf(`tar -xf - -C %s`, shellQuote(path))
	stdout, stderr, exit, ferr := s.runViaSSMOrNotImpl(containerID, cmd, data)
	if ferr != nil {
		return ferr
	}
	if exit != 0 {
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(stdout))
		}
		return &api.ServerError{Message: fmt.Sprintf("tar extract failed (exit %d): %s", exit, msg)}
	}
	return nil
}

// ContainerExportViaSSM streams the task's rootfs as tar via SSM.
// Buffered in memory — same caveat as ContainerGetArchive.
func (s *Server) ContainerExportViaSSM(containerID string) (io.ReadCloser, error) {
	cmd := `tar -cf - --exclude=./proc --exclude=./sys --exclude=./dev --exclude=./tmp -C / .`
	stdout, stderr, exit, err := s.runViaSSMOrNotImpl(containerID, cmd, nil)
	if err != nil {
		return nil, err
	}
	if exit != 0 {
		return nil, &api.ServerError{Message: fmt.Sprintf("tar export failed (exit %d): %s", exit, strings.TrimSpace(string(stderr)))}
	}
	return io.NopCloser(bytes.NewReader(stdout)), nil
}

// ContainerSignalViaSSM sends a Unix signal to the user subprocess
// PID written by the bootstrap to /tmp/.sockerless-mainpid (Phase 99
// convention). Used by ContainerPause/Unpause.
func (s *Server) ContainerSignalViaSSM(containerID, signal string) error {
	cmd := fmt.Sprintf(
		`sh -c 'test -r %s || { echo "sockerless bootstrap PID file not found at %s" >&2; exit 64; }; kill -%s $(cat %s)'`,
		core.MainPIDConventionPath, core.MainPIDConventionPath, signal, core.MainPIDConventionPath,
	)
	stdout, stderr, exit, err := s.runViaSSMOrNotImpl(containerID, cmd, nil)
	if err != nil {
		return err
	}
	if exit == 0 {
		return nil
	}
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		msg = strings.TrimSpace(string(stdout))
	}
	if exit == 64 {
		return &api.NotImplementedError{Message: "docker pause/unpause requires a bootstrap that writes " + core.MainPIDConventionPath + " (the user-process PID)"}
	}
	return &api.ServerError{Message: fmt.Sprintf("kill -%s failed (exit %d): %s", signal, exit, msg)}
}

// shellQuote wraps a string in single quotes, escaping embedded
// single quotes the POSIX way.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// dirOf / baseOf are the path.Dir / path.Base equivalents that work
// for the in-container POSIX paths we send to tar.
func dirOf(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		if i == 0 {
			return "/"
		}
		return p[:i]
	}
	return "."
}

func baseOf(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
