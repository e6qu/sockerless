package bleeplab

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ciConfig is the parsed subset of a .gitlab-ci.yml bleeplab models: the
// stage ordering and the per-job execution spec a docker-executor runner
// needs (image, script lifecycle, services, variables).
type ciConfig struct {
	Stages []string
	Jobs   []ciJob
}

type ciJob struct {
	Name         string
	Stage        string
	Script       []string
	Before       []string
	After        []string
	Image        string
	Services     []JobService
	Variables    []JobVariable
	Artifacts    []jobArtifactSpec
	Dependencies []string
}

// reserved top-level keys that are NOT jobs.
var reservedKeys = map[string]bool{
	"stages": true, "variables": true, "default": true, "include": true,
	"workflow": true, "image": true, "services": true, "before_script": true,
	"after_script": true, "cache": true, "pages": true,
}

// parseCIConfig parses the subset of .gitlab-ci.yml bleeplab supports. It
// honours top-level `stages`, global `image`/`before_script`/`after_script`/
// `variables` defaults, and per-job overrides. Hidden jobs (names starting
// with ".") are templates and are skipped.
func parseCIConfig(raw string) (*ciConfig, error) {
	if raw == "" {
		return nil, fmt.Errorf("project has no .gitlab-ci.yml")
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		return nil, fmt.Errorf("parse .gitlab-ci.yml: %w", err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf(".gitlab-ci.yml must be a mapping")
	}
	doc := root.Content[0]

	cfg := &ciConfig{}
	var globalImage string
	var globalBefore, globalAfter []string
	var globalVars []JobVariable

	// First pass: top-level config keys.
	for i := 0; i+1 < len(doc.Content); i += 2 {
		key := doc.Content[i].Value
		val := doc.Content[i+1]
		switch key {
		case "stages":
			cfg.Stages = decodeStringList(val)
		case "image":
			globalImage = decodeImageName(val)
		case "before_script":
			globalBefore = decodeStringList(val)
		case "after_script":
			globalAfter = decodeStringList(val)
		case "variables":
			globalVars = decodeVariables(val)
		}
	}
	if len(cfg.Stages) == 0 {
		cfg.Stages = []string{"build", "test", "deploy"}
	}

	// Second pass: jobs.
	for i := 0; i+1 < len(doc.Content); i += 2 {
		name := doc.Content[i].Value
		val := doc.Content[i+1]
		if reservedKeys[name] || len(name) == 0 || name[0] == '.' {
			continue
		}
		if val.Kind != yaml.MappingNode {
			continue
		}
		job := ciJob{
			Name:      name,
			Stage:     "test",
			Image:     globalImage,
			Before:    globalBefore,
			After:     globalAfter,
			Variables: append([]JobVariable{}, globalVars...),
		}
		for j := 0; j+1 < len(val.Content); j += 2 {
			jk := val.Content[j].Value
			jv := val.Content[j+1]
			switch jk {
			case "stage":
				job.Stage = jv.Value
			case "script":
				job.Script = decodeStringList(jv)
			case "before_script":
				job.Before = decodeStringList(jv)
			case "after_script":
				job.After = decodeStringList(jv)
			case "image":
				job.Image = decodeImageName(jv)
			case "services":
				job.Services = decodeServices(jv)
			case "variables":
				job.Variables = append(job.Variables, decodeVariables(jv)...)
			case "artifacts":
				job.Artifacts = decodeArtifacts(jv)
			case "dependencies":
				job.Dependencies = decodeStringList(jv)
			}
		}
		cfg.Jobs = append(cfg.Jobs, job)
	}
	if len(cfg.Jobs) == 0 {
		return nil, fmt.Errorf(".gitlab-ci.yml declares no jobs")
	}
	return cfg, nil
}

// decodeStringList accepts either a scalar (single command) or a sequence.
func decodeStringList(n *yaml.Node) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Value == "" {
			return nil
		}
		return []string{n.Value}
	case yaml.SequenceNode:
		out := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			// Nested sequences (rare) are flattened one level.
			if c.Kind == yaml.SequenceNode {
				out = append(out, decodeStringList(c)...)
			} else {
				out = append(out, c.Value)
			}
		}
		return out
	}
	return nil
}

// decodeImageName accepts `image: name` or `image: {name: ...}`.
func decodeImageName(n *yaml.Node) string {
	if n.Kind == yaml.ScalarNode {
		return n.Value
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == "name" {
				return n.Content[i+1].Value
			}
		}
	}
	return ""
}

// decodeServices accepts a sequence of scalars or `{name, alias}` maps.
func decodeServices(n *yaml.Node) []JobService {
	if n.Kind != yaml.SequenceNode {
		return nil
	}
	var out []JobService
	for _, c := range n.Content {
		switch c.Kind {
		case yaml.ScalarNode:
			out = append(out, JobService{Name: c.Value, Alias: serviceAlias(c.Value)})
		case yaml.MappingNode:
			var svc JobService
			for i := 0; i+1 < len(c.Content); i += 2 {
				switch c.Content[i].Value {
				case "name":
					svc.Name = c.Content[i+1].Value
				case "alias":
					svc.Alias = c.Content[i+1].Value
				}
			}
			if svc.Alias == "" {
				svc.Alias = serviceAlias(svc.Name)
			}
			out = append(out, svc)
		}
	}
	return out
}

// serviceAlias derives GitLab's default alias from an image name: the image
// path without registry/tag, '/' and ':' → '-' (e.g. postgres:15 → postgres).
func serviceAlias(image string) string {
	name := image
	if i := indexByte(name, '@'); i >= 0 {
		name = name[:i]
	}
	if i := indexByte(name, ':'); i >= 0 {
		name = name[:i]
	}
	if i := lastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// decodeArtifacts parses a job's `artifacts:` mapping (name, paths, when,
// expire_in, untracked) into the runner-API upload spec. GitLab models a
// single artifacts block per job; it's returned as a one-element slice to
// match the runner-API `artifacts` array shape.
func decodeArtifacts(n *yaml.Node) []jobArtifactSpec {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	var spec jobArtifactSpec
	spec.When = "on_success"
	for i := 0; i+1 < len(n.Content); i += 2 {
		switch n.Content[i].Value {
		case "name":
			spec.Name = n.Content[i+1].Value
		case "paths":
			spec.Paths = decodeStringList(n.Content[i+1])
		case "when":
			spec.When = n.Content[i+1].Value
		case "expire_in":
			spec.ExpireIn = n.Content[i+1].Value
		case "untracked":
			spec.Untracked = n.Content[i+1].Value == "true"
		}
	}
	if len(spec.Paths) == 0 && !spec.Untracked {
		return nil
	}
	return []jobArtifactSpec{spec}
}

func decodeVariables(n *yaml.Node) []JobVariable {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	var out []JobVariable
	for i := 0; i+1 < len(n.Content); i += 2 {
		out = append(out, JobVariable{Key: n.Content[i].Value, Value: n.Content[i+1].Value, Public: true})
	}
	return out
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func lastIndexByte(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// commitSHA derives a stable pseudo-commit SHA from the CI config so the
// pipeline/job CI_COMMIT_SHA is deterministic for a given config.
func commitSHA(content string) string {
	h := sha1.Sum([]byte("bleeplab\x00" + content))
	return hex.EncodeToString(h[:])
}
