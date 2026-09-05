package gcpcommon

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// GCSSyncPreExec runs the gcs-sync storage backing's PreExec for every
// shared volume declared with that backing: the runner-side workspace
// (the volume's source path) is tarred to a per-exec Cloud Storage object
// and a SOCKERLESS_SYNC_VOLUMES hint is added to the exec's environment
// through addEnv, so the bootstrap restores it into the job container
// before running the command. The returned function pulls each volume's
// modifications back to the runner-side path once the exec has finished;
// it is nil when no gcs-sync volume is declared.
func GCSSyncPreExec(ctx context.Context, shared core.SharedVolumes, backings *core.StorageBackingRegistry, execID string, addEnv func(entry string), logger zerolog.Logger) (func(), error) {
	var vols core.SharedVolumes
	for _, sv := range shared {
		if sv.Backing == core.BackingGCSSync {
			vols = append(vols, sv)
		}
	}
	if len(vols) == 0 {
		return nil, nil
	}
	driver, err := backings.Resolve(core.BackingGCSSync)
	if err != nil {
		return nil, err
	}
	var pairs []string
	for _, sv := range vols {
		hints, err := driver.PreExec(ctx, sv, execID, sv.ContainerPath, "")
		if err != nil {
			return nil, fmt.Errorf("volume %q: %w", sv.Name, err)
		}
		pairs = append(pairs, hints["SOCKERLESS_SYNC_VOLUMES"]...)
	}
	if len(pairs) > 0 {
		addEnv("SOCKERLESS_SYNC_VOLUMES=" + strings.Join(pairs, ","))
	}
	return func() {
		for _, sv := range vols {
			if perr := driver.PostExec(ctx, sv, execID, sv.ContainerPath); perr != nil {
				logger.Warn().Err(perr).Str("volume", sv.Name).Str("exec", execID).
					Msg("gcs-sync PostExec failed — runner-side workspace may be stale for this step")
			}
		}
	}, nil
}
