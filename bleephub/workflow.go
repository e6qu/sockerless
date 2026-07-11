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
	Name            string                 `yaml:"name"`
	RunsOn          interface{}            `yaml:"runs-on"`
	Container       interface{}            `yaml:"container"` // string or object
	Services        map[string]*ServiceDef // parsed from string or ServiceDef object
	Needs           []string               // parsed from string or list
	Env             map[string]string      `yaml:"env"`
	Outputs         map[string]string      `yaml:"outputs"`
	Strategy        *StrategyDef           `yaml:"strategy"`
	Steps           []StepDef              `yaml:"steps"`
	If              string                 `yaml:"if"`
	ContinueOnError bool                   `yaml:"continue-on-error"`
	TimeoutMinutes  int                    `yaml:"timeout-minutes"`
	Environment     interface{}            `yaml:"environment"` // string or {name, url}

	// Uses marks the job as a reusable-workflow call
	// ("./.github/workflows/x.yml" or "owner/repo/.github/workflows/x.yml@ref");
	// With carries its inputs (values kept as raw template strings) and
	// SecretsInherit / SecretsMap its `secrets:` configuration.
	Uses           string
	With           map[string]string
	SecretsInherit bool
	SecretsMap     map[string]string

	// Call links jobs produced by reusable-workflow expansion to their
	// call binding; ServerCompleted marks synthetic gate/collector nodes
	// the engine completes itself instead of dispatching to a runner.
	Call            *WorkflowCallBinding
	ServerCompleted bool
	// CallRole distinguishes the synthetic nodes: "gate" (resolves
	// inputs once dependencies finish) or "collector" (maps called-
	// workflow outputs onto the public caller job key).
	CallRole string
}

// EnvironmentName resolves the job's target environment name from either
// the string or the {name, url} object form; empty when the job declares
// no environment.
func (jd *JobDef) EnvironmentName() string {
	switch env := jd.Environment.(type) {
	case string:
		return env
	case map[string]interface{}:
		name, _ := env["name"].(string)
		return name
	case map[interface{}]interface{}:
		name, _ := env["name"].(string)
		return name
	}
	return ""
}

// StrategyDef represents a job's strategy configuration.
type StrategyDef struct {
	Matrix      MatrixDef `yaml:"matrix"`
	FailFast    *bool     `yaml:"fail-fast"`
	MaxParallel int       `yaml:"max-parallel"`
}

// FailFast returns the matrix strategy's fail-fast value. Defaults to true
// per the GitHub Actions spec when the strategy or the field is absent.
// Lets callers consult the resolved value with a single deref instead of
// stacking nil guards at every level of the chain.
func (j *JobDef) FailFast() bool {
	if j == nil || j.Strategy == nil || j.Strategy.FailFast == nil {
		return true
	}
	return *j.Strategy.FailFast
}

// MatrixDef represents a matrix strategy configuration.
type MatrixDef struct {
	Values  map[string][]interface{} // non-reserved keys
	Include []map[string]interface{} // include entries
	Exclude []map[string]interface{} // exclude entries
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
	Needs           interface{}            `yaml:"needs"`    // string or []string
	Env             map[string]string      `yaml:"env"`
	Outputs         map[string]string      `yaml:"outputs"`
	Strategy        *rawStrategyDef        `yaml:"strategy"`
	Steps           []StepDef              `yaml:"steps"`
	If              string                 `yaml:"if"`
	ContinueOnError bool                   `yaml:"continue-on-error"`
	TimeoutMinutes  int                    `yaml:"timeout-minutes"`
	Environment     interface{}            `yaml:"environment"` // string or {name, url}
	Uses            string                 `yaml:"uses"`
	With            map[string]interface{} `yaml:"with"`
	Secrets         interface{}            `yaml:"secrets"` // "inherit" or map
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
		Environment:     rj.Environment,
		Uses:            rj.Uses,
	}

	// Reusable-workflow call inputs/secrets (jobs.<id>.uses).
	if len(rj.With) > 0 {
		jd.With = make(map[string]string, len(rj.With))
		for k, v := range rj.With {
			jd.With[k] = exprToString(normalizeYAMLValue(v))
		}
	}
	switch sec := rj.Secrets.(type) {
	case nil:
	case string:
		if sec != "inherit" {
			return nil, fmt.Errorf("secrets must be a map or the string 'inherit', got %q", sec)
		}
		jd.SecretsInherit = true
	case map[string]interface{}:
		jd.SecretsMap = make(map[string]string, len(sec))
		for k, v := range sec {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("secrets.%s must be a string", k)
			}
			jd.SecretsMap[k] = s
		}
	default:
		return nil, fmt.Errorf("secrets must be a map or 'inherit', got %T", sec)
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

	if jd.Uses == "" {
		if err := validateRunsOn(jd.RunsOn); err != nil {
			return nil, err
		}
	}

	return jd, nil
}

func validateRunsOn(v interface{}) error {
	switch runsOn := v.(type) {
	case nil:
		return fmt.Errorf("runs-on is required")
	case string:
		if strings.TrimSpace(runsOn) == "" {
			return fmt.Errorf("runs-on is required")
		}
		return nil
	case []interface{}:
		if len(runsOn) == 0 {
			return fmt.Errorf("runs-on must include at least one label")
		}
		for _, item := range runsOn {
			label, ok := item.(string)
			if !ok || strings.TrimSpace(label) == "" {
				return fmt.Errorf("runs-on labels must be non-empty strings")
			}
		}
		return nil
	default:
		return fmt.Errorf("runs-on must be a string or list of strings, got %T", v)
	}
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

// RunsOnLabels returns the job's runs-on labels (string or list form);
// nil when unset.
func (jd *JobDef) RunsOnLabels() []string {
	if jd == nil {
		return nil
	}
	switch v := jd.RunsOn.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
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

// ContainerObject returns the parsed ContainerDef when `container:`
// was declared in object form (image + env/ports/volumes/options),
// nil for the bare-string and absent forms.
func (jd *JobDef) ContainerObject() *ContainerDef {
	m, ok := jd.Container.(map[string]interface{})
	if !ok {
		return nil
	}
	cd := &ContainerDef{}
	if img, ok := m["image"].(string); ok {
		cd.Image = img
	}
	if env, ok := m["env"].(map[string]interface{}); ok {
		cd.Env = make(map[string]string, len(env))
		for k, v := range env {
			cd.Env[k] = fmt.Sprintf("%v", v)
		}
	}
	if ports, ok := m["ports"].([]interface{}); ok {
		cd.Ports = ports
	}
	if vols, ok := m["volumes"].([]interface{}); ok {
		for _, v := range vols {
			if s, ok := v.(string); ok {
				cd.Volumes = append(cd.Volumes, s)
			}
		}
	}
	if opts, ok := m["options"].(string); ok {
		cd.Options = opts
	}
	return cd
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
