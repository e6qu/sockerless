package core

import (
	"encoding/json"
	"fmt"
	"io"
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
			// Handle multi-value ENV k1=v1 k2=v2
			for _, entry := range parseEnvMulti(rest) {
				result.config.Env = append(result.config.Env, entry)
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

// parseEnvMulti parses ENV instructions that may contain multiple key=value pairs.
// ENV k1=v1 k2=v2 should produce two env entries.
func parseEnvMulti(rest string) []string {
	// If it contains "=", it might be multi-value form
	if !strings.Contains(rest, "=") {
		// Single legacy form: ENV key value
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) == 2 {
			return []string{parts[0] + "=" + parts[1]}
		}
		return []string{parts[0] + "="}
	}
	tokens := splitRespectingQuotes(rest)
	var result []string
	for _, token := range tokens {
		if eqIdx := strings.Index(token, "="); eqIdx >= 0 {
			key := token[:eqIdx]
			value := token[eqIdx+1:]
			value = strings.Trim(value, "\"'")
			result = append(result, key+"="+value)
		}
	}
	return result
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
// Handle quoted values with spaces (e.g. LABEL foo="bar baz").
func parseLabels(rest string, labels map[string]string) {
	tokens := splitRespectingQuotes(rest)
	for _, token := range tokens {
		if eqIdx := strings.Index(token, "="); eqIdx >= 0 {
			key := token[:eqIdx]
			value := token[eqIdx+1:]
			value = strings.Trim(value, "\"'")
			labels[key] = value
		}
	}
}

// splitRespectingQuotes splits a string on spaces while keeping quoted substrings together.
// Used by parseLabels and parseEnvMulti.
func splitRespectingQuotes(s string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case inQuote != 0:
			if ch == inQuote {
				current.WriteByte(ch)
				inQuote = 0
			} else {
				current.WriteByte(ch)
			}
		case ch == '"' || ch == '\'':
			current.WriteByte(ch)
			inQuote = ch
		case ch == ' ' || ch == '\t':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
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
	q := r.URL.Query()
	opts := api.ImageBuildOptions{
		Dockerfile:   q.Get("dockerfile"),
		NoCache:      q.Get("nocache") == "1" || q.Get("nocache") == "true",
		Remove:       q.Get("rm") != "0" && q.Get("rm") != "false",
		Quiet:        q.Get("q") == "1" || q.Get("q") == "true",
		Target:       q.Get("target"),
		Platform:     q.Get("platform"),
		NetworkMode:  q.Get("networkmode"),
		ExtraHosts:   q.Get("extrahosts"),
		Squash:       q.Get("squash") == "1" || q.Get("squash") == "true",
		Pull:         q.Get("pull"),
		Remote:       q.Get("remote"),
		CgroupParent: q.Get("cgroupparent"),
		CPUSetCPUs:   q.Get("cpusetcpus"),
		Outputs:      q.Get("outputs"),
		Version:      q.Get("version"),
	}
	if t := q.Get("t"); t != "" {
		opts.Tags = []string{t}
	}
	if v := q.Get("shmsize"); v != "" {
		fmt.Sscanf(v, "%d", &opts.ShmSize)
	}
	if v := q.Get("memory"); v != "" {
		fmt.Sscanf(v, "%d", &opts.Memory)
	}
	if v := q.Get("memswap"); v != "" {
		fmt.Sscanf(v, "%d", &opts.MemorySwap)
	}
	if v := q.Get("cpushares"); v != "" {
		fmt.Sscanf(v, "%d", &opts.CPUShares)
	}
	if v := q.Get("cpuperiod"); v != "" {
		fmt.Sscanf(v, "%d", &opts.CPUPeriod)
	}
	if v := q.Get("cpuquota"); v != "" {
		fmt.Sscanf(v, "%d", &opts.CPUQuota)
	}
	if ba := q.Get("buildargs"); ba != "" {
		var args map[string]*string
		if err := json.Unmarshal([]byte(ba), &args); err != nil {
			WriteError(w, &api.InvalidParameterError{Message: "invalid buildargs: " + err.Error()})
			return
		}
		opts.BuildArgs = args
	}
	if v := q.Get("labels"); v != "" {
		var labels map[string]string
		if json.Unmarshal([]byte(v), &labels) == nil {
			opts.Labels = labels
		}
	}
	if v := q.Get("cachefrom"); v != "" {
		var cacheFrom []string
		if json.Unmarshal([]byte(v), &cacheFrom) == nil {
			opts.CacheFrom = cacheFrom
		}
	}
	if v := q.Get("cacheto"); v != "" {
		opts.CacheTo = []string{v}
	}

	dctx := DriverContext{
		Ctx:     r.Context(),
		Backend: s.Desc.Driver,
		Logger:  s.Logger,
	}
	rc, err := s.Typed.Build.Build(dctx, opts, r.Body)
	if err != nil {
		WriteError(w, err)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
