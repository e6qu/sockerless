package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// Amplify Hosting SSR (WEB_COMPUTE) execution, per the published Amplify
// Hosting deployment specification: a deployed bundle whose root carries
// `deploy-manifest.json` (version 1) routes requests through routes[] to
// "Static" targets (served from the bundle's `static/` directory) or
// "Compute" targets (proxied to the compute resource's HTTP server). Each
// computeResource is `compute/{name}/` in the bundle with a node entrypoint
// that listens on PORT=3000 (the spec's compute port convention).
//
// The sim runs one long-lived compute container per branch active
// deployment, started lazily on the first request that routes to a Compute
// target (deploys that are never browsed start no container). A new
// deployment replaces the container on its next request; branch/app deletes
// stop it. Persistent simulator restarts reclaim the same running compute
// container, published port, and extracted deployment bundle.

// ---------- deploy-manifest.json ----------

type amplifyDeployManifest struct {
	Version          int                           `json:"version"`
	Framework        *amplifyManifestFramework     `json:"framework,omitempty"`
	Routes           []amplifyManifestRoute        `json:"routes"`
	ComputeResources []amplifyManifestComputeEntry `json:"computeResources,omitempty"`
	ImageSettings    json.RawMessage               `json:"imageSettings,omitempty"`
}

type amplifyManifestFramework struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type amplifyManifestRoute struct {
	Path     string                 `json:"path"`
	Target   *amplifyManifestTarget `json:"target"`
	Fallback *amplifyManifestTarget `json:"fallback,omitempty"`
}

type amplifyManifestTarget struct {
	Kind         string `json:"kind"` // Static | Compute | ImageOptimization
	Src          string `json:"src,omitempty"`
	CacheControl string `json:"cacheControl,omitempty"`
}

type amplifyManifestComputeEntry struct {
	Name       string `json:"name"`
	Runtime    string `json:"runtime"` // e.g. nodejs18.x
	Entrypoint string `json:"entrypoint"`
}

func amplifyParseDeployManifest(data []byte) (*amplifyDeployManifest, error) {
	var manifest amplifyDeployManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid deploy-manifest.json: %w", err)
	}
	if manifest.Version != 1 {
		return nil, fmt.Errorf("deploy-manifest.json version %d not supported (spec version 1)", manifest.Version)
	}
	if len(manifest.Routes) == 0 {
		return nil, fmt.Errorf("deploy-manifest.json has no routes")
	}
	return &manifest, nil
}

func (m *amplifyDeployManifest) computeResource(name string) (amplifyManifestComputeEntry, bool) {
	for _, c := range m.ComputeResources {
		if c.Name == name || name == "" {
			return c, true
		}
	}
	return amplifyManifestComputeEntry{}, false
}

// amplifyManifestRouteMatch matches a route path pattern against a request
// path. The spec's patterns are simple globs ("/api/*", "/*.*", "/*") where
// `*` matches any characters, including separators — the catch-all "/*"
// matches every path.
func amplifyManifestRouteMatch(pattern, reqPath string) bool {
	var b strings.Builder
	b.WriteString("^")
	for i, part := range strings.Split(pattern, "*") {
		if i > 0 {
			b.WriteString(".*")
		}
		b.WriteString(regexp.QuoteMeta(part))
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(reqPath)
}

// ---------- compute container lifecycle ----------

// amplifyComputePort is the container-side port the deployment spec requires
// compute entrypoints to listen on.
const amplifyComputePort = 3000

type amplifyComputeInstance struct {
	jobID     string
	hostPort  int
	bundleDir string
	handle    *sim.ContainerHandle
}

var (
	amplifyComputeMu        sync.Mutex
	amplifyComputeInstances = map[string]*amplifyComputeInstance{} // appID/branch
)

func recoverAmplifyComputeInstances() error {
	if !sim.HasPersistentWorkloadIdentity() {
		return nil
	}
	existing, err := sim.FindExistingContainers(map[string]string{"sockerless-amplify-compute": ""})
	if err != nil {
		return fmt.Errorf("find AWS Amplify Hosting compute containers: %w", err)
	}
	amplifyComputeMu.Lock()
	defer amplifyComputeMu.Unlock()
	for _, workload := range existing {
		key := workload.Labels["sockerless-amplify-compute"]
		if key == "" {
			return fmt.Errorf("AWS Amplify Hosting compute container %s has no deployment identity", workload.ID)
		}
		if _, duplicate := amplifyComputeInstances[key]; duplicate {
			return fmt.Errorf("AWS Amplify Hosting deployment %s has multiple compute containers", key)
		}
		jobID := workload.Labels["sockerless-amplify-job-id"]
		if jobID == "" {
			parts := strings.SplitN(key, "/", 2)
			if len(parts) != 2 {
				return fmt.Errorf("AWS Amplify Hosting compute identity %q is invalid", key)
			}
			job, ok := amplifyLatestSucceededJob(parts[0], parts[1])
			if !ok {
				return fmt.Errorf("AWS Amplify Hosting compute container %s has no active deployment", workload.ID)
			}
			jobID = job.Job.Summary.JobId
		}
		hostPort := workload.PublishedPorts[amplifyComputePort]
		if hostPort == 0 {
			return fmt.Errorf("AWS Amplify Hosting compute container %s has no published port %d", workload.ID, amplifyComputePort)
		}
		if !workload.Running {
			if err := sim.StartExistingContainer(workload.ID); err != nil {
				return fmt.Errorf("restart AWS Amplify Hosting compute container %s: %w", workload.ID, err)
			}
		}
		handle, err := sim.AdoptContainer(workload.ID, sim.ContainerConfig{}, sim.NoopSink{})
		if err != nil {
			return fmt.Errorf("adopt AWS Amplify Hosting compute container %s: %w", workload.ID, err)
		}
		if err := amplifyWaitForPort(hostPort, 60*time.Second); err != nil {
			handle.Cancel()
			return fmt.Errorf("AWS Amplify Hosting compute container %s did not resume: %w", workload.ID, err)
		}
		amplifyComputeInstances[key] = &amplifyComputeInstance{
			jobID: jobID, hostPort: hostPort,
			bundleDir: workload.Labels["sockerless-amplify-bundle-dir"],
			handle:    handle,
		}
	}
	return nil
}

// amplifyStopCompute stops and forgets a branch's compute container. Called
// on branch/app delete; simulator shutdown is covered by CleanupContainers.
func amplifyStopCompute(appID, branch string) {
	amplifyComputeMu.Lock()
	defer amplifyComputeMu.Unlock()
	amplifyStopComputeLocked(appID + "/" + branch)
}

func amplifyStopComputeLocked(key string) {
	instance := amplifyComputeInstances[key]
	if instance == nil {
		return
	}
	delete(amplifyComputeInstances, key)
	if instance.handle != nil {
		instance.handle.Cancel()
	}
	if instance.bundleDir != "" {
		_ = os.RemoveAll(instance.bundleDir)
	}
}

// amplifyComputeRuntimeImage maps the manifest's runtime ("nodejs18.x") to
// the node container image (via the ECR Public Docker Hub mirror — repo
// policy: no direct Docker Hub pulls).
func amplifyComputeRuntimeImage(runtimeName string) (string, error) {
	m := regexp.MustCompile(`^nodejs(\d+)\.x$`).FindStringSubmatch(runtimeName)
	if m == nil {
		return "", fmt.Errorf("compute runtime %q not supported (expected nodejsNN.x)", runtimeName)
	}
	return "public.ecr.aws/docker/library/node:" + m[1] + "-alpine", nil
}

// amplifyEnsureCompute returns the host port of the running compute
// container for the branch's active deployment, starting (or replacing) it
// if needed.
func amplifyEnsureCompute(app AmplifyApp, br AmplifyBranch, content *amplifyHostedContent, compute amplifyManifestComputeEntry) (int, error) {
	key := app.AppId + "/" + br.BranchName
	amplifyComputeMu.Lock()
	defer amplifyComputeMu.Unlock()
	if instance := amplifyComputeInstances[key]; instance != nil {
		if instance.jobID == content.JobID {
			return instance.hostPort, nil
		}
		// New active deployment: replace the container.
		amplifyStopComputeLocked(key)
	}

	image, err := amplifyComputeRuntimeImage(compute.Runtime)
	if err != nil {
		return 0, err
	}
	bundleDir, err := amplifyExtractBundle(content.Files)
	if err != nil {
		return 0, err
	}
	hostPort, err := amplifyFreeLoopbackPort()
	if err != nil {
		_ = os.RemoveAll(bundleDir)
		return 0, err
	}

	env := map[string]string{}
	for k, v := range app.EnvironmentVariables {
		env[k] = v
	}
	for k, v := range br.EnvironmentVariables {
		env[k] = v
	}
	env["PORT"] = strconv.Itoa(amplifyComputePort)

	handle, err := sim.StartContainerSync(sim.ContainerConfig{
		Image:        image,
		Architecture: "linux/" + runtime.GOARCH,
		Command:      []string{"node", compute.Entrypoint},
		WorkingDir:   "/bundle/compute/" + compute.Name,
		// The deployed bundle is immutable to compute. The shared SELinux
		// relabel lets the confined workload read the real host directory on
		// enforcing hosts and is accepted as a no-op by Docker elsewhere.
		Binds: []string{bundleDir + ":/bundle:ro,z"},
		Env:   env,
		Labels: map[string]string{
			"sockerless-amplify-compute":    key,
			"sockerless-amplify-job-id":     content.JobID,
			"sockerless-amplify-bundle-dir": bundleDir,
		},
		PublishPorts: map[int]int{amplifyComputePort: hostPort},
		Sandbox:      sim.SandboxFargate,
	}, sim.NoopSink{})
	if err != nil {
		_ = os.RemoveAll(bundleDir)
		return 0, fmt.Errorf("start compute container: %w", err)
	}
	if err := amplifyWaitForPort(hostPort, 60*time.Second); err != nil {
		handle.Cancel()
		_ = os.RemoveAll(bundleDir)
		return 0, fmt.Errorf("compute entrypoint %s never listened on PORT %d: %w", compute.Entrypoint, amplifyComputePort, err)
	}
	amplifyComputeInstances[key] = &amplifyComputeInstance{
		jobID:     content.JobID,
		hostPort:  hostPort,
		bundleDir: bundleDir,
		handle:    handle,
	}
	return hostPort, nil
}

// amplifyExtractBundle writes the unpacked deployment to a temp dir so the
// compute container can bind-mount it.
func amplifyExtractBundle(files map[string][]byte) (string, error) {
	dir, err := os.MkdirTemp("", "sockerless-amplify-compute-*")
	if err != nil {
		return "", err
	}
	for name, data := range files {
		dest := filepath.Join(dir, filepath.FromSlash(name))
		if !strings.HasPrefix(dest, dir+string(os.PathSeparator)) {
			_ = os.RemoveAll(dir)
			return "", fmt.Errorf("bundle path escapes extraction dir: %s", name)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			_ = os.RemoveAll(dir)
			return "", err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			_ = os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}

func amplifyFreeLoopbackPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return 0, fmt.Errorf("unexpected listener address type %T", ln.Addr())
	}
	port := addr.Port
	_ = ln.Close()
	return port, nil
}

// amplifyWaitForPort blocks until the compute server answers an HTTP
// request (bounded cold-start health wait before the first proxy). A bare
// TCP accept is not enough: Podman's gvproxy binds the published host port
// immediately and resets connections until the container backend listens,
// so the wait requires actual response bytes from the entrypoint.
func amplifyWaitForPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	probe := func() error {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err != nil {
			return err
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := conn.Write([]byte("HEAD / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n")); err != nil {
			return err
		}
		buf := make([]byte, 1)
		if _, err := conn.Read(buf); err != nil {
			return err
		}
		return nil
	}
	for {
		err := probe()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// ---------- request routing ----------

// amplifyServeManifestRoutes serves a WEB_COMPUTE request through the deploy
// manifest's routes[]: first matching route wins; a Static miss falls back
// to the route's fallback target.
func amplifyServeManifestRoutes(w http.ResponseWriter, r *http.Request, app AmplifyApp, br AmplifyBranch, content *amplifyHostedContent) {
	for _, route := range content.Manifest.Routes {
		if route.Target == nil || !amplifyManifestRouteMatch(route.Path, r.URL.Path) {
			continue
		}
		if amplifyServeManifestTarget(w, r, app, br, content, route.Target) {
			return
		}
		if route.Fallback != nil && amplifyServeManifestTarget(w, r, app, br, content, route.Fallback) {
			return
		}
		http.NotFound(w, r)
		return
	}
	http.NotFound(w, r)
}

// amplifyServeManifestTarget attempts one route target; reports whether the
// request was handled (a Static miss is unhandled so fallbacks apply).
func amplifyServeManifestTarget(w http.ResponseWriter, r *http.Request, app AmplifyApp, br AmplifyBranch, content *amplifyHostedContent, target *amplifyManifestTarget) bool {
	switch target.Kind {
	case "Static":
		// The spec places compute-bundle static assets under static/.
		key, ok := amplifyResolveFile(content.Files, "static"+path.Clean("/"+r.URL.Path))
		if !ok {
			return false
		}
		if target.CacheControl != "" {
			w.Header().Set("Cache-Control", target.CacheControl)
		}
		amplifyWriteFile(w, key, content.Files[key], http.StatusOK)
		return true
	case "Compute":
		compute, ok := content.Manifest.computeResource(target.Src)
		if !ok {
			http.Error(w, "deploy-manifest names no compute resource "+target.Src, http.StatusBadGateway)
			return true
		}
		port, err := amplifyEnsureCompute(app, br, content, compute)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		if err := amplifyProxyToCompute(w, r, port); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return true
	case "ImageOptimization":
		// Real Amplify runs an image-optimization service for this kind;
		// the sim has none and won't pretend it transformed anything.
		http.Error(w, "ImageOptimization routes are not supported by this simulator", http.StatusNotImplemented)
		return true
	default:
		http.Error(w, "deploy-manifest target kind "+target.Kind+" not supported", http.StatusBadGateway)
		return true
	}
}

func amplifyProxyToCompute(w http.ResponseWriter, r *http.Request, port int) error {
	upstreamURL := url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		Path:     r.URL.EscapedPath(),
		RawQuery: r.URL.RawQuery,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, r.Method, upstreamURL.String(), r.Body)
	if err != nil {
		return err
	}
	req.Header = r.Header.Clone()
	req.Host = r.Host
	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("forward to compute %s: %w", upstreamURL.Host, err)
	}
	defer resp.Body.Close()
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}
