package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog"
	sim "github.com/sockerless/simulator"
)

// ACR Tasks quick-build slice. Sockerless's Azure backends (backends/aca,
// backends/azure-functions) build the reverse-agent bootstrap overlay
// image through the real ACR Tasks API — RegistriesClient.BeginScheduleRun
// with a DockerBuildRequest, then polling RunsClient.Get. This implements
// that slice at cloud-API fidelity: the build context is fetched from the
// sim's blob storage (where the backend's azblob upload landed), the
// docker build runs on the host engine — the sim's build infrastructure,
// exactly as the GCP Cloud Build slice (simulators/gcp/cloudbuild.go) does
// — and the resulting image lands in the local daemon tagged with the ACR
// image name, ready for sim.StartContainerSync to run. There is no
// sockerless-aware special-casing: any ACR Tasks client (SDK / az acr
// build / terraform) issuing a docker-build run reaches the same path.
//
// Real API: https://learn.microsoft.com/en-us/rest/api/containerregistry/registries/schedule-run

type acrRun struct {
	ID         string           `json:"id,omitempty"`
	Name       string           `json:"name,omitempty"`
	Type       string           `json:"type,omitempty"`
	Properties acrRunProperties `json:"properties"`
}

type acrRunProperties struct {
	RunID             string               `json:"runId"`
	Status            string               `json:"status"`
	ProvisioningState string               `json:"provisioningState"`
	RunType           string               `json:"runType,omitempty"`
	CreateTime        string               `json:"createTime,omitempty"`
	StartTime         string               `json:"startTime,omitempty"`
	FinishTime        string               `json:"finishTime,omitempty"`
	LastUpdatedTime   string               `json:"lastUpdatedTime,omitempty"`
	OutputImages      []acrImageDescriptor `json:"outputImages,omitempty"`
	Platform          *acrPlatform         `json:"platform,omitempty"`
}

type acrImageDescriptor struct {
	Registry   string `json:"registry,omitempty"`
	Repository string `json:"repository,omitempty"`
	Tag        string `json:"tag,omitempty"`
}

type acrPlatform struct {
	OS           string `json:"os,omitempty"`
	Architecture string `json:"architecture,omitempty"`
}

type acrArgument struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	IsSecret bool   `json:"isSecret"`
}

// acrDockerBuildRequest is the subset of the ACR Tasks DockerBuildRequest
// the sim honors. `type` discriminates the run-request union; sockerless
// only issues DockerBuildRequest.
type acrDockerBuildRequest struct {
	Type           string        `json:"type"`
	DockerFilePath string        `json:"dockerFilePath"`
	ImageNames     []string      `json:"imageNames"`
	SourceLocation string        `json:"sourceLocation"`
	IsPushEnabled  *bool         `json:"isPushEnabled"`
	NoCache        bool          `json:"noCache"`
	Arguments      []acrArgument `json:"arguments"`
	Target         string        `json:"target"`
	Platform       *acrPlatform  `json:"platform"`
}

var acrRuns sim.Store[acrRun]
var acrTasksLogger zerolog.Logger

func registerACRTasks(srv *sim.Server) {
	acrRuns = sim.MakeStore[acrRun](srv.DB(), "acr_runs")
	acrTasksLogger = srv.Logger()
	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerRegistry"
	// scheduleRun / runs are not in the path-normalization allowlist, so
	// they must be registered with the exact casing the SDK emits.
	srv.HandleFunc("POST "+armBase+"/registries/{registryName}/scheduleRun", handleACRScheduleRun)
	srv.HandleFunc("GET "+armBase+"/registries/{registryName}/runs/{runId}", handleACRGetRun)
}

func handleACRScheduleRun(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	registry := sim.PathParam(r, "registryName")

	var req acrDockerBuildRequest
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureError(w, "InvalidRequestContent", "failed to parse run request: "+err.Error(), http.StatusBadRequest)
		return
	}

	runID := "cb" + generateUUID()[:8]
	now := time.Now().UTC().Format(time.RFC3339)
	run := acrRun{
		ID:   fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerRegistry/registries/%s/runs/%s", sub, rg, registry, runID),
		Name: runID,
		Type: "Microsoft.ContainerRegistry/registries/runs",
		Properties: acrRunProperties{
			RunID:      runID,
			RunType:    "QuickBuild",
			CreateTime: now,
			StartTime:  now,
			Platform:   req.Platform,
		},
	}

	// Execute the docker build synchronously. Real ACR Tasks is async
	// (QUEUED → RUNNING → SUCCEEDED) with status surfaced via the Run
	// resource; the sim compresses that into the one call, exactly as the
	// GCP Cloud Build slice does, so the SDK poller resolves on the first
	// body read.
	buildErr := executeACRBuild(r.Context(), req)
	run.Properties.FinishTime = time.Now().UTC().Format(time.RFC3339)
	run.Properties.LastUpdatedTime = run.Properties.FinishTime
	if buildErr != nil {
		run.Properties.Status = "Failed"
		run.Properties.ProvisioningState = "Failed"
		acrRuns.Put(runID, run)
		// Surface the build failure in the sim log for harness diagnosis.
		// The response is still a 200 with the Run body (status=Failed) —
		// real ACR returns the run resource and reports failure through
		// its status, which the SDK body poller reads to fail the caller;
		// an ARM error envelope here would break the poller's body read.
		acrTasksLogger.Error().Str("runId", runID).Strs("images", req.ImageNames).
			Err(buildErr).Msg("ACR Task build failed")
		sim.WriteJSON(w, http.StatusOK, run)
		return
	}

	run.Properties.Status = "Succeeded"
	run.Properties.ProvisioningState = "Succeeded"
	for _, img := range req.ImageNames {
		run.Properties.OutputImages = append(run.Properties.OutputImages, parseACRImage(img))
	}
	acrRuns.Put(runID, run)
	sim.WriteJSON(w, http.StatusOK, run)
}

func handleACRGetRun(w http.ResponseWriter, r *http.Request) {
	runID := sim.PathParam(r, "runId")
	run, ok := acrRuns.Get(runID)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "Run %q not found.", runID)
		return
	}
	sim.WriteJSON(w, http.StatusOK, run)
}

// executeACRBuild fetches the build context the backend uploaded to sim
// blob storage and runs `docker build` against the host engine, tagging
// the image with each requested ACR image name. The context is a gzipped
// tar (Dockerfile + COPY'd files), streamed to `docker build -` on stdin,
// so no extraction is needed. The built image lands in the local daemon —
// the sim's registry — where StartContainerSync resolves it by tag.
func executeACRBuild(ctx context.Context, req acrDockerBuildRequest) error {
	if len(req.ImageNames) == 0 {
		return fmt.Errorf("imageNames is required")
	}
	if req.SourceLocation == "" {
		return fmt.Errorf("sourceLocation is required (only source-based DockerBuildRequest is supported)")
	}

	account, container, blob, err := parseACRBlobURL(req.SourceLocation)
	if err != nil {
		return fmt.Errorf("parse sourceLocation: %w", err)
	}
	obj, ok := blobObjects.Get(blobObjectKey(account, container, blob))
	if !ok {
		return fmt.Errorf("source context blob %s/%s not found in storage account %s", container, blob, account)
	}

	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not available for ACR Tasks build: %w", err)
	}

	dockerfile := req.DockerFilePath
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}

	args := []string{"build", "-f", dockerfile}
	for _, img := range req.ImageNames {
		args = append(args, "-t", img)
	}
	if req.Platform != nil && req.Platform.OS != "" {
		platform := req.Platform.OS
		if req.Platform.Architecture != "" {
			platform += "/" + req.Platform.Architecture
		}
		args = append(args, "--platform", platform)
	}
	if req.NoCache {
		args = append(args, "--no-cache")
	}
	for _, a := range req.Arguments {
		args = append(args, "--build-arg", a.Name+"="+a.Value)
	}
	if req.Target != "" {
		args = append(args, "--target", req.Target)
	}
	args = append(args, "-") // build context from stdin (gzipped tar)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = bytes.NewReader(obj.Data)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker build %v failed: %w: %s", req.ImageNames, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// parseACRBlobURL splits an Azure blob URL
// (https://<account>.blob.core.windows.net/<container>/<blob>) into its
// account / container / blob-name parts. The account is the host's first
// subdomain label; the first path segment is the container and the rest is
// the blob name (which may itself contain slashes).
func parseACRBlobURL(raw string) (account, container, blob string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "", err
	}
	host := u.Host
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	account, _, _ = strings.Cut(host, ".")
	if account == "" {
		return "", "", "", fmt.Errorf("no storage account in host %q", u.Host)
	}
	p := strings.TrimPrefix(u.Path, "/")
	container, blob, ok := strings.Cut(p, "/")
	if !ok || container == "" || blob == "" {
		return "", "", "", fmt.Errorf("expected /<container>/<blob> path, got %q", u.Path)
	}
	return account, container, blob, nil
}

// parseACRImage splits <registry>/<repository>:<tag> into its parts for
// the Run's outputImages list.
func parseACRImage(img string) acrImageDescriptor {
	d := acrImageDescriptor{}
	rest := img
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		d.Registry = rest[:i]
		rest = rest[i+1:]
	}
	if i := strings.LastIndexByte(rest, ':'); i >= 0 {
		d.Repository = rest[:i]
		d.Tag = rest[i+1:]
	} else {
		d.Repository = rest
	}
	return d
}
