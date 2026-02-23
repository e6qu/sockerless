package core

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/sockerless/api"
)

type copyInstruction struct {
	src string
	dst string
}

type parsedDockerfile struct {
	from   string
	config api.ContainerConfig
	copies []copyInstruction
}

// parseDockerfile parses a Dockerfile and returns the parsed result.
// Only the final stage is kept for multi-stage builds.
func parseDockerfile(content string, buildArgs map[string]string) (*parsedDockerfile, error) {
	// Join line continuations
	content = strings.ReplaceAll(content, "\\\n", " ")

	lines := strings.Split(content, "\n")
	result := &parsedDockerfile{
		config: api.ContainerConfig{
			Labels:       make(map[string]string),
			ExposedPorts: make(map[string]struct{}),
		},
	}
	args := make(map[string]string)

	// Seed args from buildArgs
	for k, v := range buildArgs {
		args[k] = v
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split into instruction and arguments
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		instruction := strings.ToUpper(parts[0])
		rest := strings.TrimSpace(parts[1])

		// Substitute ARG variables
		rest = substituteArgs(rest, args)

		switch instruction {
		case "FROM":
			// Reset for new stage (multi-stage)
			if result.from != "" {
				result.config = api.ContainerConfig{
					Labels:       make(map[string]string),
					ExposedPorts: make(map[string]struct{}),
				}
				result.copies = nil
			}

			// FROM image [AS name]
			fromParts := strings.Fields(rest)
			result.from = fromParts[0]

		case "COPY":
			fields := strings.Fields(rest)
			if len(fields) < 2 {
				continue
			}
			// Skip COPY --from=stage (multi-stage copy)
			if strings.HasPrefix(fields[0], "--from=") {
				continue
			}
			// Skip other flags like --chown, --chmod
			srcIdx := 0
			for srcIdx < len(fields)-1 && strings.HasPrefix(fields[srcIdx], "--") {
				srcIdx++
			}
			if srcIdx >= len(fields)-1 {
				continue
			}
			dst := fields[len(fields)-1]
			for i := srcIdx; i < len(fields)-1; i++ {
				result.copies = append(result.copies, copyInstruction{
					src: fields[i],
					dst: dst,
				})
			}

		case "ADD":
			fields := strings.Fields(rest)
			if len(fields) < 2 {
				continue
			}
			// Skip flags
			srcIdx := 0
			for srcIdx < len(fields)-1 && strings.HasPrefix(fields[srcIdx], "--") {
				srcIdx++
			}
			if srcIdx >= len(fields)-1 {
				continue
			}
			dst := fields[len(fields)-1]
			for i := srcIdx; i < len(fields)-1; i++ {
				result.copies = append(result.copies, copyInstruction{
					src: fields[i],
					dst: dst,
				})
			}

		case "ENV":
			key, value := parseEnv(rest)
			if key != "" {
				result.config.Env = append(result.config.Env, key+"="+value)
			}

		case "CMD":
			result.config.Cmd = parseShellOrExec(rest)

		case "ENTRYPOINT":
			result.config.Entrypoint = parseShellOrExec(rest)

		case "WORKDIR":
			result.config.WorkingDir = rest

		case "ARG":
			// ARG name[=default]
			if eqIdx := strings.Index(rest, "="); eqIdx >= 0 {
				name := rest[:eqIdx]
				def := rest[eqIdx+1:]
				// buildArgs override defaults
				if _, ok := args[name]; !ok {
					args[name] = def
				}
			}
			// ARG without default: just register (already in args if from buildArgs)

		case "LABEL":
			parseLabels(rest, result.config.Labels)

		case "EXPOSE":
			port := strings.Fields(rest)[0]
			if !strings.Contains(port, "/") {
				port += "/tcp"
			}
			result.config.ExposedPorts[port] = struct{}{}

		case "USER":
			result.config.User = rest

		case "HEALTHCHECK":
			hc := parseHealthcheckInstruction(rest)
			result.config.Healthcheck = hc

		case "SHELL":
			result.config.Shell = parseShellOrExec(rest)

		case "STOPSIGNAL":
			result.config.StopSignal = rest

		case "VOLUME":
			if result.config.Volumes == nil {
				result.config.Volumes = make(map[string]struct{})
			}
			// JSON array form: VOLUME ["/data", "/logs"]
			if strings.HasPrefix(strings.TrimSpace(rest), "[") {
				var arr []string
				if json.Unmarshal([]byte(rest), &arr) == nil {
					for _, v := range arr {
						result.config.Volumes[v] = struct{}{}
					}
				}
			} else {
				// Space-separated form: VOLUME /data /logs
				for _, v := range strings.Fields(rest) {
					result.config.Volumes[v] = struct{}{}
				}
			}

		case "RUN", "ONBUILD":
			// Ignored for our purposes
		}
	}

	if result.from == "" {
		return nil, fmt.Errorf("no FROM instruction found")
	}

	return result, nil
}

// substituteArgs replaces $NAME and ${NAME} with values from the args map.
func substituteArgs(s string, args map[string]string) string {
	for k, v := range args {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
		s = strings.ReplaceAll(s, "$"+k, v)
	}
	return s
}

// parseEnv parses ENV key=value or ENV key value form.
func parseEnv(rest string) (string, string) {
	// Try key=value form first
	if eqIdx := strings.Index(rest, "="); eqIdx >= 0 {
		key := rest[:eqIdx]
		value := rest[eqIdx+1:]
		// Strip surrounding quotes
		value = strings.Trim(value, "\"'")
		return key, value
	}
	// Space-separated form: ENV key value with spaces
	parts := strings.SplitN(rest, " ", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

// parseShellOrExec parses JSON array form ["a","b"] or shell form "a b c".
func parseShellOrExec(rest string) []string {
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "[") {
		var arr []string
		if err := json.Unmarshal([]byte(rest), &arr); err == nil {
			return arr
		}
	}
	return strings.Fields(rest)
}

// parseHealthcheckInstruction parses the arguments of a HEALTHCHECK instruction.
// Supports: HEALTHCHECK NONE, HEALTHCHECK [options] CMD <command>, HEALTHCHECK [options] CMD ["cmd","arg"].
func parseHealthcheckInstruction(rest string) *api.HealthcheckConfig {
	rest = strings.TrimSpace(rest)

	if strings.EqualFold(rest, "NONE") {
		return &api.HealthcheckConfig{Test: []string{"NONE"}}
	}

	hc := &api.HealthcheckConfig{}
	// Parse options
	for {
		rest = strings.TrimSpace(rest)
		if !strings.HasPrefix(rest, "--") {
			break
		}
		// Find end of option (next space)
		endIdx := strings.IndexFunc(rest, unicode.IsSpace)
		if endIdx < 0 {
			break
		}
		opt := rest[:endIdx]
		rest = rest[endIdx:]

		if eqIdx := strings.Index(opt, "="); eqIdx >= 0 {
			key := opt[:eqIdx]
			val := opt[eqIdx+1:]
			dur := parseDuration(val)
			switch key {
			case "--interval":
				hc.Interval = int64(dur)
			case "--timeout":
				hc.Timeout = int64(dur)
			case "--start-period":
				hc.StartPeriod = int64(dur)
			case "--retries":
				n := 0
				for _, ch := range val {
					if ch >= '0' && ch <= '9' {
						n = n*10 + int(ch-'0')
					}
				}
				hc.Retries = n
			}
		}
	}

	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(strings.ToUpper(rest), "CMD") {
		return hc
	}
	rest = strings.TrimSpace(rest[3:])

	// Parse CMD — exec form ["cmd","arg"] or shell form
	if strings.HasPrefix(rest, "[") {
		var arr []string
		if err := json.Unmarshal([]byte(rest), &arr); err == nil {
			hc.Test = append([]string{"CMD"}, arr...)
			return hc
		}
	}
	// Shell form
	hc.Test = []string{"CMD-SHELL", rest}
	return hc
}

// parseDuration parses a Docker-style duration string like "5s", "1m30s", "500ms".
func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// parseLabels parses LABEL key=value [key2=value2 ...] into the labels map.
func parseLabels(rest string, labels map[string]string) {
	// Simple parser: split on spaces, but handle quoted values
	fields := strings.Fields(rest)
	for _, field := range fields {
		if eqIdx := strings.Index(field, "="); eqIdx >= 0 {
			key := field[:eqIdx]
			value := field[eqIdx+1:]
			value = strings.Trim(value, "\"'")
			labels[key] = value
		}
	}
}

// prepareBuildContext processes COPY instructions and creates a staging directory
// with files at their final container paths.
func prepareBuildContext(contextDir string, copies []copyInstruction) (string, error) {
	if len(copies) == 0 {
		return "", nil
	}

	stagingDir, err := os.MkdirTemp("", "build-ctx-")
	if err != nil {
		return "", err
	}

	for _, cp := range copies {
		srcPath := filepath.Join(contextDir, cp.src)
		// Determine destination path in the staging dir
		dstPath := filepath.Join(stagingDir, cp.dst)

		srcInfo, err := os.Stat(srcPath)
		if err != nil {
			continue // skip missing files
		}

		if srcInfo.IsDir() {
			// Copy directory contents
			_ = filepath.Walk(srcPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				rel, _ := filepath.Rel(srcPath, path)
				dest := filepath.Join(dstPath, rel)
				if info.IsDir() {
					_ = os.MkdirAll(dest, info.Mode())
					return nil
				}
				_ = os.MkdirAll(filepath.Dir(dest), 0755)
				data, err := os.ReadFile(path)
				if err != nil {
					return nil
				}
				_ = os.WriteFile(dest, data, info.Mode())
				return nil
			})
		} else {
			// Single file copy
			// If dst ends with /, it's a directory — put file inside
			if strings.HasSuffix(cp.dst, "/") {
				dstPath = filepath.Join(dstPath, filepath.Base(cp.src))
			}
			_ = os.MkdirAll(filepath.Dir(dstPath), 0755)
			data, err := os.ReadFile(srcPath)
			if err != nil {
				continue
			}
			_ = os.WriteFile(dstPath, data, srcInfo.Mode())
		}
	}

	return stagingDir, nil
}

// handleImageBuild handles POST /internal/v1/images/build.
func (s *BaseServer) handleImageBuild(w http.ResponseWriter, r *http.Request) {
	tag := r.URL.Query().Get("t")
	dockerfileName := r.URL.Query().Get("dockerfile")
	if dockerfileName == "" {
		dockerfileName = "Dockerfile"
	}

	// Parse buildargs from query param (JSON string)
	var buildArgs map[string]string
	if ba := r.URL.Query().Get("buildargs"); ba != "" {
		_ = json.Unmarshal([]byte(ba), &buildArgs)
	}

	// Extract tar body to temp dir
	contextDir, err := os.MkdirTemp("", "docker-build-")
	if err != nil {
		WriteError(w, &api.ServerError{Message: "failed to create temp dir: " + err.Error()})
		return
	}
	defer os.RemoveAll(contextDir)

	if err := extractTar(r.Body, contextDir); err != nil {
		WriteError(w, &api.ServerError{Message: "failed to extract build context: " + err.Error()})
		return
	}

	// Read the Dockerfile
	dfContent, err := os.ReadFile(filepath.Join(contextDir, dockerfileName))
	if err != nil {
		WriteError(w, &api.ServerError{Message: "failed to read Dockerfile: " + err.Error()})
		return
	}

	parsed, err := parseDockerfile(string(dfContent), buildArgs)
	if err != nil {
		WriteError(w, &api.ServerError{Message: "failed to parse Dockerfile: " + err.Error()})
		return
	}

	// Resolve base image config
	baseConfig := api.ContainerConfig{
		Env: []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
	}
	if baseImg, ok := s.Store.ResolveImage(parsed.from); ok {
		baseConfig = baseImg.Config
	}

	// Merge: base image config + parsed Dockerfile overrides
	finalConfig := baseConfig
	if len(parsed.config.Env) > 0 {
		finalConfig.Env = append(finalConfig.Env, parsed.config.Env...)
	}
	if len(parsed.config.Cmd) > 0 {
		finalConfig.Cmd = parsed.config.Cmd
	}
	if len(parsed.config.Entrypoint) > 0 {
		finalConfig.Entrypoint = parsed.config.Entrypoint
	}
	if parsed.config.WorkingDir != "" {
		finalConfig.WorkingDir = parsed.config.WorkingDir
	}
	if parsed.config.User != "" {
		finalConfig.User = parsed.config.User
	}
	if finalConfig.Labels == nil {
		finalConfig.Labels = make(map[string]string)
	}
	for k, v := range parsed.config.Labels {
		finalConfig.Labels[k] = v
	}
	if finalConfig.ExposedPorts == nil {
		finalConfig.ExposedPorts = make(map[string]struct{})
	}
	for k, v := range parsed.config.ExposedPorts {
		finalConfig.ExposedPorts[k] = v
	}
	if parsed.config.Healthcheck != nil {
		finalConfig.Healthcheck = parsed.config.Healthcheck
	}
	if len(parsed.config.Shell) > 0 {
		finalConfig.Shell = parsed.config.Shell
	}
	if parsed.config.StopSignal != "" {
		finalConfig.StopSignal = parsed.config.StopSignal
	}
	if len(parsed.config.Volumes) > 0 {
		if finalConfig.Volumes == nil {
			finalConfig.Volumes = make(map[string]struct{})
		}
		for k, v := range parsed.config.Volumes {
			finalConfig.Volumes[k] = v
		}
	}

	// Generate image ID
	hash := sha256.Sum256([]byte(tag + time.Now().String()))
	imageID := fmt.Sprintf("sha256:%x", hash)
	shortID := fmt.Sprintf("%x", hash)[:12]

	ref := tag
	if ref == "" {
		ref = imageID
	}
	if !strings.Contains(ref, ":") {
		ref += ":latest"
	}

	img := api.Image{
		ID:           imageID,
		RepoTags:     []string{ref},
		Created:      time.Now().UTC().Format(time.RFC3339Nano),
		Size:         0,
		Architecture: "amd64",
		Os:           "linux",
		Config:       finalConfig,
		RootFS:       api.RootFS{Type: "layers", Layers: []string{"sha256:" + GenerateID()}},
	}
	StoreImageWithAliases(s.Store, ref, img)

	// Process COPY instructions: stage files for container injection
	if len(parsed.copies) > 0 {
		stagingDir, err := prepareBuildContext(contextDir, parsed.copies)
		if err == nil && stagingDir != "" {
			s.Store.BuildContexts.Store(imageID, stagingDir)
		}
	}

	// Stream Docker build-format JSON progress
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	flusher, _ := w.(http.Flusher)

	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}

	// Reconstruct steps from Dockerfile for output
	dfLines := strings.Split(strings.ReplaceAll(string(dfContent), "\\\n", " "), "\n")
	step := 0
	totalSteps := 0
	for _, line := range dfLines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			totalSteps++
		}
	}

	for _, line := range dfLines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		step++
		_ = enc.Encode(map[string]any{"stream": fmt.Sprintf("Step %d/%d : %s\n", step, totalSteps, line)})
		flush()
		_ = enc.Encode(map[string]any{"stream": fmt.Sprintf(" ---> %s\n", shortID)})
		flush()
	}

	_ = enc.Encode(map[string]any{"aux": map[string]string{"ID": imageID}})
	flush()
	_ = enc.Encode(map[string]any{"stream": fmt.Sprintf("Successfully built %s\n", shortID)})
	flush()
	if tag != "" {
		_ = enc.Encode(map[string]any{"stream": fmt.Sprintf("Successfully tagged %s\n", ref)})
		flush()
	}
}
