// Command gen reads api/openapi.yaml and generates types_gen.go and backend_gen.go.
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// ── OpenAPI structures ──────────────────────────────────────────────────

type OpenAPI struct {
	Paths      map[string]map[string]Operation `yaml:"paths"`
	Components struct {
		Schemas map[string]Schema `yaml:"schemas"`
	} `yaml:"components"`
}

type Operation struct {
	OperationID string   `yaml:"operationId"`
	GoMethod    string   `yaml:"x-sockerless-go-method"`
	GoArgs      []string `yaml:"x-sockerless-go-args"`
	GoReturns   string   `yaml:"x-sockerless-go-returns"`
	Streaming   bool     `yaml:"x-sockerless-streaming"`
	Upgrade     bool     `yaml:"x-sockerless-upgrade"`
	Summary     string   `yaml:"summary"`
	Parameters  []Param  `yaml:"parameters"`
	RequestBody *struct {
		Content map[string]struct {
			Schema SchemaRef `yaml:"schema"`
		} `yaml:"content"`
	} `yaml:"requestBody"`
	Responses map[string]struct {
		Content map[string]struct {
			Schema SchemaRef `yaml:"schema"`
		} `yaml:"content"`
	} `yaml:"responses"`
}

type Param struct {
	Name     string    `yaml:"name"`
	In       string    `yaml:"in"`
	Required bool      `yaml:"required"`
	Schema   SchemaRef `yaml:"schema"`
}

type SchemaRef struct {
	Ref                  string     `yaml:"$ref"`
	Type                 string     `yaml:"type"`
	Format               string     `yaml:"format"`
	Items                *SchemaRef `yaml:"items"`
	AdditionalProperties *SchemaRef `yaml:"additionalProperties"`
}

type Schema struct {
	Type                 string             `yaml:"type"`
	Properties           map[string]PropDef `yaml:"properties"`
	DockerName           string             `yaml:"x-sockerless-docker-name"`
	GoEmbeds             []string           `yaml:"x-sockerless-go-embeds"`
	Extensions           []string           `yaml:"x-sockerless-extensions"`
	CodegenSkip          bool               `yaml:"x-sockerless-codegen-skip"`
}

type PropDef struct {
	Type                 string   `yaml:"type"`
	Format               string   `yaml:"format"`
	Ref                  string   `yaml:"$ref"`
	GoType               string   `yaml:"x-sockerless-go-type"`
	GoPointer            bool     `yaml:"x-sockerless-go-pointer"`
	JSONTag              string   `yaml:"x-sockerless-json-tag"`
	Items                *PropDef `yaml:"items"`
	AdditionalProperties *PropDef `yaml:"additionalProperties"`
}

// ── Go field representation ─────────────────────────────────────────────

type GoField struct {
	Name    string // Go field name
	Type    string // Go type string
	JSONTag string // JSON tag value
	Omit    bool   // whether to add omitempty
}

type GoStruct struct {
	Name    string
	Comment string
	Embeds  []string // embedded type names (e.g. "*ContainerConfig")
	Fields  []GoField
}

// ── Backend method representation ───────────────────────────────────────

type BackendMethod struct {
	Name    string
	Comment string
	Args    string // e.g. "id string, req *ContainerCreateRequest"
	Returns string // e.g. "(*ContainerCreateResponse, error)"
}

func main() {
	// Usage: gen [specPath [outDir]]
	// When run via go generate, cwd is the package dir (api/), so defaults work.
	// When run directly from api/gen/, we detect and adjust.
	specPath := "openapi.yaml"
	outDir := "."
	explicitOutDir := false

	if len(os.Args) > 1 {
		specPath = os.Args[1]
	} else if _, err := os.Stat(specPath); err != nil {
		specPath = filepath.Join("..", "openapi.yaml")
		outDir = ".."
	}
	if len(os.Args) > 2 {
		outDir = os.Args[2]
		explicitOutDir = true
	}
	_ = explicitOutDir

	data, err := os.ReadFile(specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read spec: %v\n", err)
		os.Exit(1)
	}

	var spec OpenAPI
	if err := yaml.Unmarshal(data, &spec); err != nil {
		fmt.Fprintf(os.Stderr, "parse spec: %v\n", err)
		os.Exit(1)
	}

	if err := generateTypes(spec, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "generate types: %v\n", err)
		os.Exit(1)
	}

	if err := generateBackend(spec, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "generate backend: %v\n", err)
		os.Exit(1)
	}
}

// ── Type generation ─────────────────────────────────────────────────────

func generateTypes(spec OpenAPI, outDir string) error {
	schemas := spec.Components.Schemas

	// Determine which types need which imports.
	needsOS := false
	needsTime := false
	needsIO := false
	for _, s := range schemas {
		for _, p := range s.Properties {
			if p.GoType == "os.FileMode" {
				needsOS = true
			}
			if p.GoType == "time.Time" {
				needsTime = true
			}
		}
	}
	// ContainerArchiveResponse has io.ReadCloser
	for _, s := range schemas {
		if s.CodegenSkip {
			needsIO = true
		}
	}

	// Sort schema names for deterministic output.
	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	sort.Strings(names)

	// Build structs.
	var structs []GoStruct
	for _, name := range names {
		s := schemas[name]
		if s.CodegenSkip {
			continue
		}
		gs := schemaToStruct(name, s)
		structs = append(structs, gs)
	}

	// Build imports.
	var imports []string
	if needsIO {
		imports = append(imports, `"io"`)
	}
	if needsOS {
		imports = append(imports, `"os"`)
	}
	if needsTime {
		imports = append(imports, `"time"`)
	}

	tmpl := template.Must(template.New("types").Parse(typesTmpl))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{
		"Imports": imports,
		"Structs": structs,
	}); err != nil {
		return fmt.Errorf("template: %w", err)
	}

	// Append hand-written ContainerArchiveResponse.
	buf.WriteString(`
// ContainerArchiveResponse wraps the stat and reader for a container archive get.
type ContainerArchiveResponse struct {
	Stat   ContainerPathStat
	Reader io.ReadCloser
}
`)

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// Write unformatted for debugging.
		os.WriteFile(filepath.Join(outDir, "types_gen.go"), buf.Bytes(), 0644)
		return fmt.Errorf("gofmt: %w", err)
	}

	return os.WriteFile(filepath.Join(outDir, "types_gen.go"), formatted, 0644)
}

func schemaToStruct(name string, s Schema) GoStruct {
	gs := GoStruct{
		Name:    name,
		Comment: fmt.Sprintf("%s is a generated type.", name),
	}

	// Embeds.
	for _, e := range s.GoEmbeds {
		gs.Embeds = append(gs.Embeds, "*"+e)
	}

	// Sort properties by a canonical order.
	propNames := make([]string, 0, len(s.Properties))
	for pn := range s.Properties {
		propNames = append(propNames, pn)
	}
	sort.Strings(propNames)

	for _, pn := range propNames {
		p := s.Properties[pn]
		goName := jsonToGoName(pn)
		goType := propToGoType(p)
		jsonTag := pn
		if p.JSONTag != "" {
			jsonTag = p.JSONTag
		}
		omit := shouldOmitempty(name, pn, p)
		gs.Fields = append(gs.Fields, GoField{
			Name:    goName,
			Type:    goType,
			JSONTag: jsonTag,
			Omit:    omit,
		})
	}

	return gs
}

// jsonToGoName converts a JSON field name to an exported Go name.
// Handles special cases like "Id" → "ID", "Dns" → "DNS", "CpuShares" → "CPUShares".
func jsonToGoName(json string) string {
	// Well-known renames (JSON tag → Go name).
	renames := map[string]string{
		"Id":          "ID",
		"ParentId":    "ParentID",
		"HostIp":      "HostIP",
		"IPAddress":   "IPAddress",
		"IPPrefixLen": "IPPrefixLen",
		"CpuShares":   "CPUShares",
		"CpuQuota":    "CPUQuota",
		"CpuPeriod":   "CPUPeriod",
		"NanoCpus":    "NanoCPUs",
		"Dns":         "DNS",
		"DnsSearch":   "DNSSearch",
		"DnsOptions":  "DNSOptions",
		"IP":          "IP",
	}
	if n, ok := renames[json]; ok {
		return n
	}
	// Lowercase first char means the JSON name IS the Go name for non-exported-looking fields.
	// But we need them to be exported. For types like ExecProcessConfig,
	// the JSON tags are lowercase but Go fields are title-cased.
	if len(json) > 0 && json[0] >= 'a' && json[0] <= 'z' {
		// Special cases for lowercase JSON names that stay lowercase.
		lowercaseFields := map[string]string{
			"tty":           "Tty",
			"entrypoint":    "Entrypoint",
			"arguments":     "Arguments",
			"privileged":    "Privileged",
			"user":          "User",
			"env":           "Env",
			"workingDir":    "WorkingDir",
			"username":      "Username",
			"password":      "Password",
			"email":         "Email",
			"serveraddress": "ServerAddress",
			"scope":         "Scope",
			"time":          "Time",
			"timeNano":      "TimeNano",
			"name":          "Name",
			"description":   "Description",
			"star_count":    "StarCount",
			"is_official":   "IsOfficial",
			"is_automated":  "IsAutomated",
			"container":     "Container",
			"repo":          "Repo",
			"tag":           "Tag",
			"comment":       "Comment",
			"author":        "Author",
			"pause":         "Pause",
			"changes":       "Changes",
			"config":        "Config",
			"size":          "Size",
			"mode":          "Mode",
			"mtime":         "Mtime",
			"linkTarget":    "LinkTarget",
			"tags":          "Tags",
			"dockerfile":    "Dockerfile",
			"buildArgs":     "BuildArgs",
			"noCache":       "NoCache",
			"remove":        "Remove",
			"forceRemove":   "ForceRemove",
			"labels":        "Labels",
			"target":        "Target",
			"platform":      "Platform",
			"pull":          "Pull",
			"quiet":         "Quiet",
			"networkMode":   "NetworkMode",
			"extraHosts":    "ExtraHosts",
			"shmSize":       "ShmSize",
			"hostname":      "Hostname",
			"share":         "Share",
			"no_infra":      "NoInfra",
		}
		if n, ok := lowercaseFields[json]; ok {
			return n
		}
		// Default: title case first letter.
		return strings.ToUpper(json[:1]) + json[1:]
	}
	return json
}

func propToGoType(p PropDef) string {
	if p.GoType != "" {
		return p.GoType
	}

	if p.Ref != "" {
		refName := refToName(p.Ref)
		if p.GoPointer {
			return "*" + refName
		}
		return refName
	}

	switch p.Type {
	case "string":
		if p.Format == "date-time" {
			return "time.Time"
		}
		return "string"
	case "boolean":
		if p.GoPointer {
			return "*bool"
		}
		return "bool"
	case "integer":
		base := intType(p.Format)
		if p.GoPointer {
			return "*" + base
		}
		return base
	case "array":
		if p.Items != nil {
			itemType := propToGoType(*p.Items)
			return "[]" + itemType
		}
		return "[]any"
	case "object":
		if p.AdditionalProperties != nil {
			valType := propToGoType(*p.AdditionalProperties)
			return "map[string]" + valType
		}
		return "map[string]any"
	}
	return "any"
}

func intType(format string) string {
	switch format {
	case "int64":
		return "int64"
	case "uint64":
		return "uint64"
	default:
		return "int"
	}
}

func refToName(ref string) string {
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

// shouldOmitempty determines whether a field should have omitempty.
// This matches the hand-written types.go conventions.
func shouldOmitempty(structName, fieldName string, p PropDef) bool {
	// Pointer types always omitempty.
	if p.GoPointer {
		return true
	}

	// Known omitempty fields by struct.
	omitFields := map[string]map[string]bool{
		"Container": {
			"SizeRw": true, "SizeRootFs": true,
			"AgentAddress": true, "AgentToken": true,
		},
		"ContainerConfig": {
			"ExposedPorts": true, "Volumes": true, "StopSignal": true,
			"StopTimeout": true, "Shell": true, "Healthcheck": true,
			"ArgsEscaped": true, "NetworkDisabled": true, "MacAddress": true,
			"OnBuild": true,
		},
		"HostConfig": {
			"Binds": true, "PortBindings": true, "Privileged": true,
			"CapAdd": true, "CapDrop": true, "Init": true,
			"UsernsMode": true, "ShmSize": true, "Tmpfs": true,
			"SecurityOpt": true, "ExtraHosts": true, "Mounts": true,
			"Isolation": true, "Dns": true, "DnsSearch": true, "DnsOptions": true,
			"Memory": true, "MemorySwap": true, "MemoryReservation": true,
			"CpuShares": true, "CpuQuota": true, "CpuPeriod": true,
			"CpusetCpus": true, "NanoCpus": true, "CpusetMems": true,
			"BlkioWeight": true, "PidMode": true, "IpcMode": true,
			"UTSMode": true, "VolumesFrom": true, "GroupAdd": true,
			"ReadonlyRootfs": true, "OomKillDisable": true, "PidsLimit": true,
			"Sysctls": true, "Runtime": true, "Links": true,
			"PublishAllPorts": true, "CgroupnsMode": true, "ConsoleSize": true,
		},
		"Mount": {
			"ReadOnly": true, "Consistency": true, "BindOptions": true,
			"VolumeOptions": true, "TmpfsOptions": true,
		},
		"BindOptions":   {"Propagation": true},
		"VolumeOptions": {"NoCopy": true, "Labels": true, "DriverConfig": true},
		"VolumeDriverConfig": {"Name": true, "Options": true},
		"TmpfsOptions":       {"SizeBytes": true, "Mode": true},
		"EndpointSettings": {
			"IPAMConfig": true, "Aliases": true, "Links": true,
			"DNSNames": true, "DriverOpts": true,
		},
		"EndpointIPAMConfig": {"IPv4Address": true, "IPv6Address": true, "LinkLocalIPs": true},
		"MountPoint":         {"Name": true, "Driver": true, "Propagation": true},
		"ContainerSummary":   {"SizeRw": true, "SizeRootFs": true, "HostConfig": true, "NetworkSettings": true},
		"ContainerCreateRequest": {"Name": true, "HostConfig": true, "NetworkingConfig": true},
		"NetworkingConfig":       {"EndpointsConfig": true},
		"ContainerListOptions":   {"Limit": true, "Filters": true},
		"ExecCreateRequest":      {"Env": true, "WorkingDir": true, "User": true, "DetachKeys": true},
		"ExecStartRequest":       {"ConsoleSize": true},
		"ExecInstance":           {"DetachKeys": true},
		"ExecProcessConfig":     {"privileged": true, "env": true, "workingDir": true},
		"ContainerAttachOptions": {"DetachKeys": true},
		"ContainerUpdateRequest": {
			"Memory": true, "MemorySwap": true, "MemoryReservation": true,
			"CpuShares": true, "CpuQuota": true, "CpuPeriod": true,
			"CpusetCpus": true, "CpusetMems": true, "BlkioWeight": true,
			"PidsLimit": true, "OomKillDisable": true,
		},
		"Image": {
			"Author": true, "Parent": true, "Comment": true, "DockerVersion": true,
		},
		"GraphDriverData":    {"Data": true},
		"RootFS":             {"Layers": true},
		"ImageMetadata":      {"LastTagTime": true},
		"ImageDeleteResponse": {"Untagged": true, "Deleted": true},
		"AuthRequest":        {"email": true},
		"AuthResponse":       {"IdentityToken": true},
		"IPAM":               {"Options": true},
		"IPAMConfig":         {"IPRange": true, "AuxiliaryAddresses": true},
		"NetworkCreateRequest": {"IPAM": true, "Options": true, "Labels": true},
		"NetworkConnectRequest": {"EndpointConfig": true},
		"Volume":              {"CreatedAt": true, "Status": true, "UsageData": true},
		"VolumeCreateRequest": {"Name": true, "Driver": true, "DriverOpts": true, "Labels": true},
		"EventsOptions":      {"Since": true, "Until": true, "Filters": true},
		"Event":              {"scope": true},
		"BuildCache":         {"Parent": true, "Description": true, "LastUsedAt": true},
		"ImageListOptions":   {"Filters": true},
		"ImageBuildOptions": {
			"tags": true, "buildArgs": true, "labels": true,
			"target": true, "platform": true, "pull": true,
			"networkMode": true, "extraHosts": true, "shmSize": true,
			"quiet": false,
		},
		"ContainerCommitRequest": {"changes": true, "config": true},
		"ContainerWaitResponse":  {"Error": true},
		"PodCreateRequest":       {"labels": true, "hostname": true, "share": true, "no_infra": true},
		"PodListOptions":         {"Filters": true},
		"ImagePullRequest":       {"Auth": true, "Platform": true},
	}

	if fields, ok := omitFields[structName]; ok {
		if fields[fieldName] {
			return true
		}
	}
	return false
}

var typesTmpl = `// Code generated by go run ./gen; DO NOT EDIT.

package api

{{- if .Imports }}

import (
{{- range .Imports }}
	{{ . }}
{{- end }}
)
{{- end }}

{{- range .Structs }}

// {{ .Comment }}
type {{ .Name }} struct {
{{- range .Embeds }}
	{{ . }}
{{- end }}
{{- range .Fields }}
	{{ .Name }}	{{ .Type }}	` + "`" + `json:"{{ .JSONTag }}{{ if .Omit }},omitempty{{ end }}"` + "`" + `
{{- end }}
}
{{- end }}
`

// ── Backend interface generation ────────────────────────────────────────

func generateBackend(spec OpenAPI, outDir string) error {
	methods := extractMethods(spec)

	tmpl := template.Must(template.New("backend").Parse(backendTmpl))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{
		"Methods": methods,
	}); err != nil {
		return fmt.Errorf("template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		os.WriteFile(filepath.Join(outDir, "backend_gen.go"), buf.Bytes(), 0644)
		return fmt.Errorf("gofmt: %w", err)
	}

	return os.WriteFile(filepath.Join(outDir, "backend_gen.go"), formatted, 0644)
}

func extractMethods(spec OpenAPI) []BackendMethod {
	// We need to produce methods in a specific order matching the hand-written interface.
	// Build a map first, then order them.
	methodMap := make(map[string]BackendMethod)

	for path, methods := range spec.Paths {
		for _, op := range methods {
			if op.GoMethod == "" {
				continue
			}
			m := buildMethod(op, path, spec)
			methodMap[op.GoMethod] = m
		}
	}

	// Order to match hand-written backend.go exactly.
	order := []string{
		// System
		"Info",
		// Containers
		"ContainerCreate", "ContainerInspect", "ContainerList",
		"ContainerStart", "ContainerStop", "ContainerKill", "ContainerRemove",
		"ContainerLogs", "ContainerWait", "ContainerAttach",
		"ContainerRestart", "ContainerTop", "ContainerPrune",
		"ContainerStats", "ContainerRename", "ContainerPause", "ContainerUnpause",
		// Exec
		"ExecCreate", "ExecStart", "ExecInspect",
		// Images
		"ImagePull", "ImageInspect", "ImageLoad", "ImageTag",
		"ImageList", "ImageRemove", "ImageHistory", "ImagePrune",
		// Auth
		"AuthLogin",
		// Networks
		"NetworkCreate", "NetworkList", "NetworkInspect",
		"NetworkConnect", "NetworkDisconnect", "NetworkRemove", "NetworkPrune",
		// Volumes
		"VolumeCreate", "VolumeList", "VolumeInspect",
		"VolumeRemove", "VolumePrune",
		// Exec resize
		"ExecResize",
		// Container archive + misc
		"ContainerResize", "ContainerPutArchive", "ContainerStatPath",
		"ContainerGetArchive", "ContainerUpdate", "ContainerChanges", "ContainerExport",
		// Image build/push/save/search + commit
		"ImageBuild", "ImagePush", "ImageSave", "ImageSearch", "ContainerCommit",
		// Pods
		"PodCreate", "PodList", "PodInspect", "PodExists",
		"PodStart", "PodStop", "PodKill", "PodRemove",
		// System
		"SystemEvents", "SystemDf",
	}

	var result []BackendMethod
	for _, name := range order {
		if m, ok := methodMap[name]; ok {
			result = append(result, m)
		}
	}
	return result
}

func buildMethod(op Operation, path string, spec OpenAPI) BackendMethod {
	m := BackendMethod{
		Name:    op.GoMethod,
		Comment: op.Summary,
	}

	// Build method signature based on the hand-written interface.
	// This is the critical mapping — each method has a specific signature.
	signatures := map[string]struct{ args, returns string }{
		"Info":                {"", "(*BackendInfo, error)"},
		"ContainerCreate":    {"req *ContainerCreateRequest", "(*ContainerCreateResponse, error)"},
		"ContainerInspect":   {"id string", "(*Container, error)"},
		"ContainerList":      {"opts ContainerListOptions", "([]*ContainerSummary, error)"},
		"ContainerStart":     {"id string", "error"},
		"ContainerStop":      {"id string, timeout *int", "error"},
		"ContainerKill":      {"id string, signal string", "error"},
		"ContainerRemove":    {"id string, force bool", "error"},
		"ContainerLogs":      {"id string, opts ContainerLogsOptions", "(io.ReadCloser, error)"},
		"ContainerWait":      {"id string, condition string", "(*ContainerWaitResponse, error)"},
		"ContainerAttach":    {"id string, opts ContainerAttachOptions", "(io.ReadWriteCloser, error)"},
		"ContainerRestart":   {"id string, timeout *int", "error"},
		"ContainerTop":       {"id string, psArgs string", "(*ContainerTopResponse, error)"},
		"ContainerPrune":     {"filters map[string][]string", "(*ContainerPruneResponse, error)"},
		"ContainerStats":     {"id string, stream bool", "(io.ReadCloser, error)"},
		"ContainerRename":    {"id string, newName string", "error"},
		"ContainerPause":     {"id string", "error"},
		"ContainerUnpause":   {"id string", "error"},
		"ExecCreate":         {"containerID string, req *ExecCreateRequest", "(*ExecCreateResponse, error)"},
		"ExecStart":          {"id string, opts ExecStartRequest", "(io.ReadWriteCloser, error)"},
		"ExecInspect":        {"id string", "(*ExecInstance, error)"},
		"ImagePull":          {"ref string, auth string", "(io.ReadCloser, error)"},
		"ImageInspect":       {"name string", "(*Image, error)"},
		"ImageLoad":          {"r io.Reader", "(io.ReadCloser, error)"},
		"ImageTag":           {"source string, repo string, tag string", "error"},
		"ImageList":          {"opts ImageListOptions", "([]*ImageSummary, error)"},
		"ImageRemove":        {"name string, force bool, prune bool", "([]*ImageDeleteResponse, error)"},
		"ImageHistory":       {"name string", "([]*ImageHistoryEntry, error)"},
		"ImagePrune":         {"filters map[string][]string", "(*ImagePruneResponse, error)"},
		"AuthLogin":          {"req *AuthRequest", "(*AuthResponse, error)"},
		"NetworkCreate":      {"req *NetworkCreateRequest", "(*NetworkCreateResponse, error)"},
		"NetworkList":        {"filters map[string][]string", "([]*Network, error)"},
		"NetworkInspect":     {"id string", "(*Network, error)"},
		"NetworkConnect":     {"id string, req *NetworkConnectRequest", "error"},
		"NetworkDisconnect":  {"id string, req *NetworkDisconnectRequest", "error"},
		"NetworkRemove":      {"id string", "error"},
		"NetworkPrune":       {"filters map[string][]string", "(*NetworkPruneResponse, error)"},
		"VolumeCreate":       {"req *VolumeCreateRequest", "(*Volume, error)"},
		"VolumeList":         {"filters map[string][]string", "(*VolumeListResponse, error)"},
		"VolumeInspect":      {"name string", "(*Volume, error)"},
		"VolumeRemove":       {"name string, force bool", "error"},
		"VolumePrune":        {"filters map[string][]string", "(*VolumePruneResponse, error)"},
		"ExecResize":         {"id string, h, w int", "error"},
		"ContainerResize":    {"id string, h, w int", "error"},
		"ContainerPutArchive": {"id string, path string, noOverwriteDirNonDir bool, body io.Reader", "error"},
		"ContainerStatPath":  {"id string, path string", "(*ContainerPathStat, error)"},
		"ContainerGetArchive": {"id string, path string", "(*ContainerArchiveResponse, error)"},
		"ContainerUpdate":    {"id string, req *ContainerUpdateRequest", "(*ContainerUpdateResponse, error)"},
		"ContainerChanges":   {"id string", "([]ContainerChangeItem, error)"},
		"ContainerExport":    {"id string", "(io.ReadCloser, error)"},
		"ImageBuild":         {"opts ImageBuildOptions, context io.Reader", "(io.ReadCloser, error)"},
		"ImagePush":          {"name string, tag string, auth string", "(io.ReadCloser, error)"},
		"ImageSave":          {"names []string", "(io.ReadCloser, error)"},
		"ImageSearch":        {"term string, limit int, filters map[string][]string", "([]*ImageSearchResult, error)"},
		"ContainerCommit":    {"req *ContainerCommitRequest", "(*ContainerCommitResponse, error)"},
		"PodCreate":          {"req *PodCreateRequest", "(*PodCreateResponse, error)"},
		"PodList":            {"opts PodListOptions", "([]*PodListEntry, error)"},
		"PodInspect":         {"name string", "(*PodInspectResponse, error)"},
		"PodExists":          {"name string", "(bool, error)"},
		"PodStart":           {"name string", "(*PodActionResponse, error)"},
		"PodStop":            {"name string, timeout *int", "(*PodActionResponse, error)"},
		"PodKill":            {"name string, signal string", "(*PodActionResponse, error)"},
		"PodRemove":          {"name string, force bool", "error"},
		"SystemEvents":       {"opts EventsOptions", "(io.ReadCloser, error)"},
		"SystemDf":           {"", "(*DiskUsageResponse, error)"},
	}

	if sig, ok := signatures[op.GoMethod]; ok {
		m.Args = sig.args
		m.Returns = sig.returns
	}

	return m
}

var backendTmpl = `// Code generated by go run ./gen; DO NOT EDIT.

package api

import "io"

// Backend defines the interface that all backends must implement.
type Backend interface {
{{- range .Methods }}
	// {{ .Comment }}
	{{ .Name }}({{ .Args }}) {{ .Returns }}
{{- end }}
}
`
