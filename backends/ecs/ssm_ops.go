package ecs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
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
// task, run the command, return ssm output or a uniform error. The
// command is wrapped in `sh -c '<cmd>; printf "<marker>%d<marker>" $?'`
// because AWS ECS ExecuteCommand sessions are interactive and never
// send the SSM `output_stream_data` frame with `payloadType=exit_code`
// — channel_closed is the only terminal signal — so the only reliable
// way to recover the command's exit code is to have the in-container
// shell print it. The wrapper strips the marker (and the preceding
// bytes that constituted the exit-code text) before returning the
// caller's stdout.
func (s *Server) runViaSSMOrNotImpl(containerID, cmd string, stdin []byte) (stdout, stderr []byte, exitCode int, _ error) {
	c, ok := s.ResolveContainerAuto(context.Background(), containerID)
	if !ok {
		return nil, nil, -1, &api.NotFoundError{Resource: "container", ID: containerID}
	}
	taskARN := s.resolveTaskARNForOps(c.ID)
	if taskARN == "" {
		return nil, nil, -1, &api.NotImplementedError{Message: "ECS operation requires a running task; container has no active execution"}
	}
	wrapped := "sh -c " + shellQuote(cmd+`; printf "__SOCKEXIT:%d:__" $?`)
	out, errOut, _, err := s.RunCommandViaSSM(taskARN, wrapped, stdin)
	if err != nil {
		return nil, nil, -1, &api.ServerError{Message: fmt.Sprintf("SSM exec: %v", err)}
	}
	cleanOut, code, ok := extractSSMExitMarker(out)
	if !ok {
		return out, errOut, -1, &api.ServerError{Message: fmt.Sprintf("SSM exec: command produced no exit marker (stdout=%q stderr=%q)", string(out), string(errOut))}
	}
	return cleanOut, errOut, code, nil
}

// extractSSMExitMarker strips the `__SOCKEXIT:N:__` suffix from the
// command's stdout and returns the cleaned bytes plus the parsed exit
// code. ok=false when the marker is absent (which means the command
// crashed before the wrapping printf ran — caller treats as an SSM
// error rather than masking exit=0).
func extractSSMExitMarker(out []byte) ([]byte, int, bool) {
	const prefix = "__SOCKEXIT:"
	const suffix = ":__"
	idx := strings.LastIndex(string(out), prefix)
	if idx < 0 {
		return out, 0, false
	}
	rest := string(out[idx+len(prefix):])
	end := strings.Index(rest, suffix)
	if end < 0 {
		return out, 0, false
	}
	code, err := strconv.Atoi(strings.TrimSpace(rest[:end]))
	if err != nil {
		return out, 0, false
	}
	clean := out[:idx]
	// Drop a trailing CR/LF the in-container shell may have emitted
	// between the user command's last line and our printf.
	for len(clean) > 0 && (clean[len(clean)-1] == '\n' || clean[len(clean)-1] == '\r') {
		clean = clean[:len(clean)-1]
	}
	return clean, code, true
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
// returns the post-boot diff in the `<type>\t<path>` format
// `core.ParseChangesOutput` expects. Uses three `find -type` passes
// (busybox-compatible — GNU find's `-printf` is not available on
// alpine's busybox build) plus a sed prefix to emit the type tag.
// `runViaSSMOrNotImpl` wraps the script in `sh -c` for us.
func (s *Server) ContainerChangesViaSSM(containerID string) ([]api.ContainerChangeItem, error) {
	const cmd = `for t in d f l; do find / -xdev -newer /proc/1 -type "$t" | sed "s|^|$t	|"; done`
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
// task and parses the result. The format string uses literal tab
// characters because busybox `stat -c` does not interpret backslash
// escapes inside single-quoted format args, so `\t` would land in
// the output verbatim and ParseStatOutput would reject it.
func (s *Server) ContainerStatPathViaSSM(containerID, path string) (*api.ContainerPathStat, error) {
	cmd := fmt.Sprintf("stat -c '%%n\t%%s\t%%f\t%%Y\t%%N' %s", shellQuote(path))
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
