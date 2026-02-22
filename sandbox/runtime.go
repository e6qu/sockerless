// Package sandbox provides WASM-based command execution for containers.
// It uses wazero to run go-busybox applets inside a WASI sandbox with
// per-container virtual filesystems and mvdan.cc/sh for shell parsing.
package sandbox

import (
	"context"
	_ "embed"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed busybox.wasm
var busyboxWasm []byte

// Runtime manages the WASM runtime and compiled busybox module.
// Create once and share across all containers.
type Runtime struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
}

// NewRuntime creates a new WASM runtime and compiles the busybox module.
func NewRuntime(ctx context.Context) (*Runtime, error) {
	rt := wazero.NewRuntimeWithConfig(ctx,
		wazero.NewRuntimeConfig().WithCloseOnContextDone(true),
	)

	// Instantiate WASI preview1 (provides fd_write, args_get, etc.)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, err
	}

	// Compile busybox once — instantiate per command
	compiled, err := rt.CompileModule(ctx, busyboxWasm)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, err
	}

	return &Runtime{
		runtime:  rt,
		compiled: compiled,
	}, nil
}

// Close releases the WASM runtime and compiled module.
func (r *Runtime) Close(ctx context.Context) error {
	return r.runtime.Close(ctx)
}
