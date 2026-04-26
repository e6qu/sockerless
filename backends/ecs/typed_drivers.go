package ecs

import (
	"errors"
	"io"
	"strings"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// Cloud-native typed drivers for ECS — direct dispatch via SSM
// ExecuteCommand instead of the legacy s.self.ContainerXxx →
// ContainerXxxViaSSM hop. Wired in NewServer below the existing
// Logs + Attach overrides.

type ssmProcListDriver struct{ s *Server }

func (d *ssmProcListDriver) Describe() string { return "ecs SSMPs" }
func (d *ssmProcListDriver) Top(dctx core.DriverContext, psArgs string) (*api.ContainerTopResponse, error) {
	return d.s.ContainerTopViaSSM(dctx.Container.ID, psArgs)
}

type ssmFSDiffDriver struct{ s *Server }

func (d *ssmFSDiffDriver) Describe() string { return "ecs SSMFindNewer" }
func (d *ssmFSDiffDriver) Changes(dctx core.DriverContext) ([]api.ContainerChangeItem, error) {
	return d.s.ContainerChangesViaSSM(dctx.Container.ID)
}

type ssmFSReadDriver struct{ s *Server }

func (d *ssmFSReadDriver) Describe() string { return "ecs SSMTar" }
func (d *ssmFSReadDriver) StatPath(dctx core.DriverContext, path string) (*api.ContainerPathStat, error) {
	return d.s.ContainerStatPathViaSSM(dctx.Container.ID, path)
}
func (d *ssmFSReadDriver) GetArchive(dctx core.DriverContext, path string, w io.Writer) error {
	resp, err := d.s.ContainerGetArchiveViaSSM(dctx.Container.ID, path)
	if err != nil {
		return err
	}
	if resp == nil || resp.Reader == nil {
		return nil
	}
	defer resp.Reader.Close()
	_, err = io.Copy(w, resp.Reader)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

type ssmFSWriteDriver struct{ s *Server }

func (d *ssmFSWriteDriver) Describe() string { return "ecs SSMTarExtract" }
func (d *ssmFSWriteDriver) PutArchive(dctx core.DriverContext, path string, body io.Reader, _ bool) error {
	return d.s.ContainerPutArchiveViaSSM(dctx.Container.ID, path, body)
}

type ssmFSExportDriver struct{ s *Server }

func (d *ssmFSExportDriver) Describe() string { return "ecs SSMTarRoot" }
func (d *ssmFSExportDriver) Export(dctx core.DriverContext, w io.Writer) error {
	rc, err := d.s.ContainerExportViaSSM(dctx.Container.ID)
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(w, rc)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

type ssmSignalDriver struct{ s *Server }

func (d *ssmSignalDriver) Describe() string { return "ecs SSMKill" }
func (d *ssmSignalDriver) Kill(dctx core.DriverContext, signal string) error {
	// SIGSTOP / SIGCONT (`docker pause` / `docker unpause`) require
	// signaling the user PID inside the running task — only SSM can do
	// that on Fargate. Every other signal (SIGTERM/SIGKILL/SIGINT/etc.
	// from `docker kill -s`) is a docker-stop semantic; route it to the
	// legacy ContainerKill which calls ECS StopTask. This matches docker
	// daemon behavior on a swarm/managed runtime where signal delivery
	// to PID 1 is by definition "stop the container".
	switch strings.ToUpper(strings.TrimPrefix(signal, "SIG")) {
	case "STOP", "CONT":
		return d.s.ContainerSignalViaSSM(dctx.Container.ID, signal)
	}
	return d.s.ContainerKill(dctx.Container.ID, signal)
}
