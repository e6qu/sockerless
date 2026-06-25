//go:build iamscratch

package main

import sim "github.com/sockerless/simulator"

// TEMPORARY local build aid for verifying iam.go in isolation while a parallel
// agent finishes firestore admin. Guarded by the iamscratch build tag so it
// never participates in the real build, and deleted before finishing.
func registerFirestoreAdmin(srv *sim.Server) { _ = srv }
