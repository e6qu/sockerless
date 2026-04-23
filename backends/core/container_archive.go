package core

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/sockerless/api"
)

// RunContainerGetArchiveViaAgent implements `docker cp CONTAINER:/src /host/dst`
// by running `tar -cf - -C <parent> <name>` inside the container via
// the reverse-agent and returning the tar byte stream as a ReadCloser.
// Also runs stat(1) to populate the PathStat header Docker clients
// expect alongside the tarball.
//
// Phase 98 (BUG-751). The full tar is buffered in memory — this fits
// the typical `docker cp` workflow (copying single files/directories)
// but is not suitable for exporting TB-scale rootfs. For
// `docker export` use RunContainerExportViaAgent.
func RunContainerGetArchiveViaAgent(reg *ReverseAgentRegistry, containerID, srcPath string) (*api.ContainerArchiveResponse, error) {
	if reg == nil {
		return nil, ErrNoReverseAgent
	}
	stat, err := RunContainerStatPathViaAgent(reg, containerID, srcPath)
	if err != nil {
		return nil, err
	}
	parent := path.Dir(srcPath)
	name := path.Base(srcPath)
	argv := []string{"tar", "-cf", "-", "-C", parent, name}
	stdout, stderr, exit, err := reg.RunAndCapture(containerID, "archive-"+containerID, argv, nil, "")
	if err != nil {
		return nil, err
	}
	if exit != 0 {
		return nil, fmt.Errorf("tar failed (exit %d): %s", exit, strings.TrimSpace(string(stderr)))
	}
	return &api.ContainerArchiveResponse{
		Stat:   *stat,
		Reader: io.NopCloser(bytes.NewReader(stdout)),
	}, nil
}

// RunContainerPutArchiveViaAgent implements `docker cp /host/src CONTAINER:/dst`
// by reading the tar body into memory and streaming it as stdin to a
// `tar -xf - -C <dst>` exec running inside the container. Buffered in
// memory — same tradeoff as GetArchive. Phase 98.
func RunContainerPutArchiveViaAgent(reg *ReverseAgentRegistry, containerID, dstPath string, body io.Reader) error {
	if reg == nil {
		return ErrNoReverseAgent
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("read archive body: %w", err)
	}
	argv := []string{"tar", "-xf", "-", "-C", dstPath}
	stdout, stderr, exit, err := reg.RunAndCaptureWithStdin(containerID, "putarchive-"+containerID, argv, nil, "", data)
	if err != nil {
		return err
	}
	if exit != 0 {
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(stdout))
		}
		return fmt.Errorf("tar extract failed (exit %d): %s", exit, msg)
	}
	return nil
}

// RunContainerExportViaAgent returns a tar stream of the container's
// root filesystem (excluding Docker's typical mount points). Phase 98.
// Same in-memory caveat as GetArchive — for multi-GB rootfs, this
// should stream directly to the caller's conn rather than buffer. Fits
// most common `docker export` use (small containers, CI artifacts).
func RunContainerExportViaAgent(reg *ReverseAgentRegistry, containerID string) (io.ReadCloser, error) {
	if reg == nil {
		return nil, ErrNoReverseAgent
	}
	argv := []string{
		"tar", "-cf", "-",
		"--exclude=./proc",
		"--exclude=./sys",
		"--exclude=./dev",
		"--exclude=./tmp",
		"-C", "/", ".",
	}
	stdout, stderr, exit, err := reg.RunAndCapture(containerID, "export-"+containerID, argv, nil, "")
	if err != nil {
		return nil, err
	}
	// GNU tar returns 1 for "some files changed while reading" which is
	// common on a live rootfs; we surface that as soft-success. Anything
	// else is a real error.
	if exit != 0 && exit != 1 {
		return nil, fmt.Errorf("tar export failed (exit %d): %s", exit, strings.TrimSpace(string(stderr)))
	}
	return io.NopCloser(bytes.NewReader(stdout)), nil
}
