package bleephub

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConcurrencyDef represents workflow-level concurrency control.
type ConcurrencyDef struct {
	Group            string `yaml:"group" json:"group"`
	CancelInProgress bool   `yaml:"cancel-in-progress" json:"cancel_in_progress"`
}

// WorkflowDef represents a parsed GitHub Actions workflow YAML.
type WorkflowDef struct {
	Name        string            `yaml:"name"`
	Env         map[string]string `yaml:"env"`
	Concurrency *ConcurrencyDef
	Jobs        map[string]*JobDef
}

// JobDef represents a single job definition within a workflow.
type JobDef struct {
	Name            string                   `yaml:"name"`
	RunsOn          interface{}              `yaml:"runs-on"`
	Container       interface{}              `yaml:"container"` // string or object
	Services        map[string]*ServiceDef   // parsed from string or ServiceDef object
	Needs           []string                 // parsed from string or list
	Env             map[string]string        `yaml:"env"`
	Outputs         map[string]string        `yaml:"outputs"`
	Strategy        *StrategyDef             `yaml:"strategy"`
	Steps           []StepDef                `yaml:"steps"`
	If              string                   `yaml:"if"`
	ContinueOnError bool                     `yaml:"continue-on-error"`
	TimeoutMinutes  int                      `yaml:"timeout-minutes"`
}

// StrategyDef represents a job's strategy configuration.
type StrategyDef struct {
	Matrix      MatrixDef `yaml:"matrix"`
	FailFast    *bool     `yaml:"fail-fast"`
	MaxParallel int       `yaml:"max-parallel"`
}

// MatrixDef represents a matrix strategy configuration.
type MatrixDef struct {
	Values  map[string][]interface{}   // non-reserved keys
	Include []map[string]interface{}   // include entries
	Exclude []map[string]interface{}   // exclude entries
}

// StepDef represents a single step in a job.
type StepDef struct {
	ID    string            `yaml:"id"`
	Name  string            `yaml:"name"`
	Uses  string            `yaml:"uses"`
	Run   string            `yaml:"run"`
	With  map[string]string `yaml:"with"`
	Env   map[string]string `yaml:"env"`
	If    string            `yaml:"if"`
	Shell string            `yaml:"shell"`
}

// ContainerDef represents a container configuration when specified as an object.
type ContainerDef struct {
	Image   string            `yaml:"image"`
	Env     map[string]string `yaml:"env"`
	Ports   []interface{}     `yaml:"ports"`
	Volumes []string          `yaml:"volumes"`
	Options string            `yaml:"options"`
}

// ServiceDef represents a service container configuration.
type ServiceDef struct {
	Image       string            `yaml:"image"`
	Env         map[string]string `yaml:"env"`
	Ports       []interface{}     `yaml:"ports"`
	Volumes     []string          `yaml:"volumes"`
	Options     string            `yaml:"options"`
	Credentials struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"credentials"`
}

// rawWorkflow is the intermediate YAML structure before normalization.
type rawWorkflow struct {
	Name        string                `yaml:"name"`
	Env         map[string]string     `yaml:"env"`
	Concurrency interface{}           `yaml:"concurrency"` // string or object
	Jobs        map[string]*rawJobDef `yaml:"jobs"`
}

type rawJobDef struct {
	Name            string                 `yaml:"name"`
	RunsOn          interface{}            `yaml:"runs-on"`
	Container       interface{}            `yaml:"container"`
	Services        map[string]interface{} `yaml:"services"` // string or ServiceDef object
	Needs           interface{}            `yaml:"needs"`     // string or []string
	Env             map[string]string      `yaml:"env"`
	Outputs         map[string]string      `yaml:"outputs"`
	Strategy        *rawStrategyDef        `yaml:"strategy"`
	Steps           []StepDef              `yaml:"steps"`
	If              string                 `yaml:"if"`
	ContinueOnError bool                   `yaml:"continue-on-error"`
	TimeoutMinutes  int                    `yaml:"timeout-minutes"`
}

type rawStrategyDef struct {
	Matrix      yaml.Node `yaml:"matrix"`
	FailFast    *bool     `yaml:"fail-fast"`
	MaxParallel int       `yaml:"max-parallel"`
}

// ParseWorkflow parses a GitHub Actions workflow YAML definition.
func ParseWorkflow(yamlBytes []byte) (*WorkflowDef, error) {
	var raw rawWorkflow
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return nil, fmt.Errorf("parse workflow YAML: %w", err)
	}

	if len(raw.Jobs) == 0 {
		return nil, fmt.Errorf("workflow has no jobs")
	}

	wf := &WorkflowDef{
		Name: raw.Name,
		Env:  raw.Env,
		Jobs: make(map[string]*JobDef, len(raw.Jobs)),
	}

	// Parse concurrency: string → {Group: s}, object → decode
	if raw.Concurrency != nil {
		switch v := raw.Concurrency.(type) {
		case string:
			wf.Concurrency = &ConcurrencyDef{Group: v}
		case map[string]interface{}:
			cd := &ConcurrencyDef{}
			if g, ok := v["group"].(string); ok {
				cd.Group = g
			}
			if ci, ok := v["cancel-in-progress"].(bool); ok {
				cd.CancelInProgress = ci
			}
			wf.Concurrency = cd
		}
	}

	for key, rj := range raw.Jobs {
		jd, err := normalizeJob(rj)
		if err != nil {
			return nil, fmt.Errorf("job %q: %w", key, err)
		}
		wf.Jobs[key] = jd
	}

	return wf, nil
}

// normalizeJob converts a rawJobDef into a JobDef, handling quirks.
func normalizeJob(rj *rawJobDef) (*JobDef, error) {
	jd := &JobDef{
		Name:            rj.Name,
		RunsOn:          rj.RunsOn,
		Container:       rj.Container,
		Env:             rj.Env,
		Outputs:         rj.Outputs,
		Steps:           rj.Steps,
		If:              rj.If,
		ContinueOnError: rj.ContinueOnError,
		TimeoutMinutes:  rj.TimeoutMinutes,
	}

	// Normalize needs: string → []string
	switch v := rj.Needs.(type) {
	case nil:
		jd.Needs = nil
	case string:
		jd.Needs = []string{v}
	case []interface{}:
		jd.Needs = make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("needs must contain strings, got %T", item)
			}
			jd.Needs = append(jd.Needs, s)
		}
	default:
		return nil, fmt.Errorf("needs must be string or list, got %T", v)
	}

	// Parse services: each value can be a string (image name) or an object
	if len(rj.Services) > 0 {
		jd.Services = make(map[string]*ServiceDef, len(rj.Services))
		for name, val := range rj.Services {
			switch v := val.(type) {
			case string:
				jd.Services[name] = &ServiceDef{Image: v}
			case map[string]interface{}:
				svc := &ServiceDef{}
				// Re-marshal via YAML to decode into struct cleanly
				yamlBytes, err := yaml.Marshal(v)
				if err != nil {
					return nil, fmt.Errorf("service %q: %w", name, err)
				}
				if err := yaml.Unmarshal(yamlBytes, svc); err != nil {
					return nil, fmt.Errorf("service %q: %w", name, err)
				}
				jd.Services[name] = svc
			default:
				return nil, fmt.Errorf("service %q: must be string or object, got %T", name, val)
			}
		}
	}

	// Parse strategy/matrix
	if rj.Strategy != nil {
		sd, err := normalizeStrategy(rj.Strategy)
		if err != nil {
			return nil, fmt.Errorf("strategy: %w", err)
		}
		jd.Strategy = sd
	}

	return jd, nil
}

// normalizeStrategy parses a rawStrategyDef into a StrategyDef.
func normalizeStrategy(rs *rawStrategyDef) (*StrategyDef, error) {
	sd := &StrategyDef{
		FailFast:    rs.FailFast,
		MaxParallel: rs.MaxParallel,
	}

	md, err := parseMatrixNode(&rs.Matrix)
	if err != nil {
		return nil, err
	}
	sd.Matrix = md
	return sd, nil
}

// parseMatrixNode parses a YAML matrix node, separating reserved keys
// (include, exclude) from value keys.
func parseMatrixNode(node *yaml.Node) (MatrixDef, error) {
	md := MatrixDef{
		Values: make(map[string][]interface{}),
	}

	if node == nil || node.Kind == 0 {
		return md, nil
	}

	// Decode into a generic map first
	var raw map[string]interface{}
	if err := node.Decode(&raw); err != nil {
		return md, fmt.Errorf("parse matrix: %w", err)
	}

	for key, val := range raw {
		switch key {
		case "include":
			list, ok := val.([]interface{})
			if !ok {
				return md, fmt.Errorf("matrix.include must be a list")
			}
			md.Include = make([]map[string]interface{}, 0, len(list))
			for _, item := range list {
				m, ok := item.(map[string]interface{})
				if !ok {
					return md, fmt.Errorf("matrix.include entries must be maps")
				}
				md.Include = append(md.Include, m)
			}
		case "exclude":
			list, ok := val.([]interface{})
			if !ok {
				return md, fmt.Errorf("matrix.exclude must be a list")
			}
			md.Exclude = make([]map[string]interface{}, 0, len(list))
			for _, item := range list {
				m, ok := item.(map[string]interface{})
				if !ok {
					return md, fmt.Errorf("matrix.exclude entries must be maps")
				}
				md.Exclude = append(md.Exclude, m)
			}
		default:
			list, ok := val.([]interface{})
			if !ok {
				return md, fmt.Errorf("matrix.%s must be a list", key)
			}
			md.Values[key] = list
		}
	}

	return md, nil
}

// ContainerImage returns the container image string from a JobDef.Container,
// which may be a plain string or a ContainerDef object.
func (jd *JobDef) ContainerImage() string {
	switch v := jd.Container.(type) {
	case string:
		return v
	case map[string]interface{}:
		if img, ok := v["image"].(string); ok {
			return img
		}
	}
	return ""
}

// ParseActionRef splits a "uses" reference like "actions/checkout@v4" into
// owner/repo, path (if any), and ref.
// Supported formats:
//   - "owner/repo@ref"
//   - "owner/repo/path@ref"
//   - "./local/path" (returns empty owner/repo, path only)
func ParseActionRef(uses string) (nameWithOwner, path, ref string, isLocal bool) {
	if strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "../") {
		return "", uses, "", true
	}

	// Split at @
	atIdx := strings.LastIndex(uses, "@")
	if atIdx < 0 {
		return uses, "", "", false
	}
	ref = uses[atIdx+1:]
	nameAndPath := uses[:atIdx]

	// Split owner/repo from path
	parts := strings.SplitN(nameAndPath, "/", 3)
	if len(parts) >= 2 {
		nameWithOwner = parts[0] + "/" + parts[1]
	}
	if len(parts) >= 3 {
		path = parts[2]
	}
	return nameWithOwner, path, ref, false
}
