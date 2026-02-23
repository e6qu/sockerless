package gitlabhub

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// PipelineDef represents a parsed .gitlab-ci.yml pipeline definition.
type PipelineDef struct {
	Stages    []string                `json:"stages"`
	Variables map[string]string       `json:"variables,omitempty"`
	Jobs      map[string]*PipelineJobDef `json:"jobs"`
	Image     string                  `json:"image,omitempty"`
}

// PipelineJobDef represents a single job definition in the pipeline YAML.
type PipelineJobDef struct {
	Stage        string              `json:"stage"`
	Image        string              `json:"image,omitempty"`
	Script       []string            `json:"script"`
	BeforeScript []string            `json:"before_script,omitempty"`
	AfterScript  []string            `json:"after_script,omitempty"`
	Variables    map[string]string   `json:"variables,omitempty"`
	Artifacts    *ArtifactsDef       `json:"artifacts,omitempty"`
	Services     []ServiceEntry      `json:"services,omitempty"`
	Needs        []string            `json:"needs,omitempty"`
	Dependencies []string            `json:"dependencies,omitempty"`
	Rules        []RuleDef           `json:"rules,omitempty"`
	AllowFailure bool               `json:"allow_failure"`
	When         string              `json:"when,omitempty"` // on_success, always, never, manual
	Cache        *CacheEntry         `json:"cache,omitempty"`
	Timeout      int                 `json:"timeout,omitempty"`
	Retry        *RetryDef           `json:"retry,omitempty"`
	Parallel      *ParallelDef        `json:"parallel,omitempty"`
	MatrixGroup   string              `json:"matrix_group,omitempty"`
	ResourceGroup string              `json:"resource_group,omitempty"`
}

// ArtifactsDef describes artifact configuration.
type ArtifactsDef struct {
	Paths    []string    `json:"paths"`
	ExpireIn string      `json:"expire_in,omitempty"`
	When     string      `json:"when,omitempty"`
	Reports  *ReportsDef `json:"reports,omitempty"`
}

// ReportsDef describes artifact report types.
type ReportsDef struct {
	Dotenv string `json:"dotenv,omitempty"`
}

// ServiceEntry describes a service container in the pipeline.
type ServiceEntry struct {
	Name       string            `json:"name"`
	Alias      string            `json:"alias,omitempty"`
	Entrypoint []string          `json:"entrypoint,omitempty"`
	Command    []string          `json:"command,omitempty"`
	Variables  map[string]string `json:"variables,omitempty"`
}

// RuleDef describes a rule condition.
type RuleDef struct {
	If   string `json:"if,omitempty"`
	When string `json:"when,omitempty"` // on_success, always, never
}

// CacheEntry describes cache configuration for a job.
type CacheEntry struct {
	Key    string   `json:"key,omitempty"`
	Paths  []string `json:"paths,omitempty"`
	Policy string   `json:"policy,omitempty"` // pull, push, pull-push
}

// rawPipeline is the intermediate YAML structure before normalization.
type rawPipeline struct {
	Stages    interface{}            `yaml:"stages"`
	Variables map[string]interface{} `yaml:"variables"`
	Image     interface{}            `yaml:"image"`
	Default   *rawDefault            `yaml:"default"`
	// All other keys are jobs — handled via a second-pass decode
}

type rawDefault struct {
	Image        interface{} `yaml:"image"`
	BeforeScript interface{} `yaml:"before_script"`
	AfterScript  interface{} `yaml:"after_script"`
}

type rawJobDef struct {
	Stage        string              `yaml:"stage"`
	Image        interface{}         `yaml:"image"`
	Script       interface{}         `yaml:"script"`
	BeforeScript interface{}         `yaml:"before_script"`
	AfterScript  interface{}         `yaml:"after_script"`
	Variables    map[string]interface{} `yaml:"variables"`
	Artifacts    *rawArtifacts       `yaml:"artifacts"`
	Services     interface{}         `yaml:"services"`
	Needs        interface{}         `yaml:"needs"`
	Dependencies interface{}         `yaml:"dependencies"`
	Rules        []rawRule           `yaml:"rules"`
	AllowFailure interface{}         `yaml:"allow_failure"`
	When         string              `yaml:"when"`
	Cache        *rawCache           `yaml:"cache"`
	Timeout      string              `yaml:"timeout"`
	Retry         interface{}         `yaml:"retry"`
	Parallel      interface{}         `yaml:"parallel"`
	ResourceGroup string              `yaml:"resource_group"`
}

type rawArtifacts struct {
	Paths    []string    `yaml:"paths"`
	ExpireIn string      `yaml:"expire_in"`
	When     string      `yaml:"when"`
	Reports  *rawReports `yaml:"reports"`
}

type rawReports struct {
	Dotenv string `yaml:"dotenv"`
}

type rawRule struct {
	If   string `yaml:"if"`
	When string `yaml:"when"`
}

type rawCache struct {
	Key    string   `yaml:"key"`
	Paths  []string `yaml:"paths"`
	Policy string   `yaml:"policy"`
}

// reservedKeys are top-level YAML keys that are not jobs.
var reservedKeys = map[string]bool{
	"stages": true, "variables": true, "image": true, "default": true,
	"include": true, "workflow": true, "services": true, "before_script": true,
	"after_script": true, "cache": true, "pages": true,
}

// ParsePipeline parses a .gitlab-ci.yml into a PipelineDef.
func ParsePipeline(yamlBytes []byte) (*PipelineDef, error) {
	// First pass: decode all top-level keys into a generic map
	var topLevel map[string]interface{}
	if err := yaml.Unmarshal(yamlBytes, &topLevel); err != nil {
		return nil, fmt.Errorf("parse pipeline YAML: %w", err)
	}

	// Parse stages
	stages := []string{"build", "test", "deploy"}
	if raw, ok := topLevel["stages"]; ok {
		stages = nil
		list, ok := raw.([]interface{})
		if !ok {
			return nil, fmt.Errorf("stages must be a list")
		}
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("stage name must be a string, got %T", item)
			}
			stages = append(stages, s)
		}
	}

	// Parse global variables
	globalVars := make(map[string]string)
	if rawVars, ok := topLevel["variables"]; ok {
		if m, ok := rawVars.(map[string]interface{}); ok {
			for k, v := range m {
				globalVars[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	// Parse global image
	globalImage := ""
	if rawImg, ok := topLevel["image"]; ok {
		globalImage = parseImageString(rawImg)
	}

	// Parse default block
	if rawDef, ok := topLevel["default"]; ok {
		if defMap, ok := rawDef.(map[string]interface{}); ok {
			if img, ok := defMap["image"]; ok && globalImage == "" {
				globalImage = parseImageString(img)
			}
		}
	}

	// Resolve extends keywords before processing jobs
	if err := resolveExtends(topLevel); err != nil {
		return nil, fmt.Errorf("resolve extends: %w", err)
	}

	// Second pass: extract job definitions
	jobs := make(map[string]*PipelineJobDef)
	for key := range topLevel {
		if reservedKeys[key] {
			continue
		}
		// Ignore keys starting with "."
		if strings.HasPrefix(key, ".") {
			continue
		}

		// Re-marshal this key back to YAML and decode as rawJobDef
		jobYAML, err := yaml.Marshal(topLevel[key])
		if err != nil {
			return nil, fmt.Errorf("job %q: marshal failed: %w", key, err)
		}

		var rj rawJobDef
		if err := yaml.Unmarshal(jobYAML, &rj); err != nil {
			return nil, fmt.Errorf("job %q: %w", key, err)
		}

		jd, err := normalizeJobDef(key, &rj, stages, globalVars)
		if err != nil {
			return nil, fmt.Errorf("job %q: %w", key, err)
		}
		jobs[key] = jd
	}

	if len(jobs) == 0 {
		return nil, fmt.Errorf("pipeline has no jobs")
	}

	pdef := &PipelineDef{
		Stages:    stages,
		Variables: globalVars,
		Jobs:      jobs,
		Image:     globalImage,
	}

	// Expand parallel/matrix jobs
	expandParallelJobs(pdef)

	return pdef, nil
}

// normalizeJobDef converts a rawJobDef into a PipelineJobDef.
func normalizeJobDef(key string, rj *rawJobDef, stages []string, globalVars map[string]string) (*PipelineJobDef, error) {
	jd := &PipelineJobDef{
		Stage: rj.Stage,
		When:  rj.When,
	}

	// Default stage to "test" if not specified
	if jd.Stage == "" {
		jd.Stage = "test"
	}

	// Default when to "on_success" if not specified
	if jd.When == "" {
		jd.When = "on_success"
	}

	// Parse image
	jd.Image = parseImageString(rj.Image)

	// Parse script (string or []string)
	scripts, err := parseStringList(rj.Script)
	if err != nil {
		return nil, fmt.Errorf("script: %w", err)
	}
	jd.Script = scripts

	// Parse before_script
	jd.BeforeScript, _ = parseStringList(rj.BeforeScript)

	// Parse after_script
	jd.AfterScript, _ = parseStringList(rj.AfterScript)

	// Parse variables: merge global + job (job overrides)
	jd.Variables = make(map[string]string, len(globalVars))
	for k, v := range globalVars {
		jd.Variables[k] = v
	}
	if rj.Variables != nil {
		for k, v := range rj.Variables {
			jd.Variables[k] = fmt.Sprintf("%v", v)
		}
	}

	// Parse artifacts
	if rj.Artifacts != nil {
		jd.Artifacts = &ArtifactsDef{
			Paths:    rj.Artifacts.Paths,
			ExpireIn: rj.Artifacts.ExpireIn,
			When:     rj.Artifacts.When,
		}
		if rj.Artifacts.Reports != nil {
			jd.Artifacts.Reports = &ReportsDef{
				Dotenv: rj.Artifacts.Reports.Dotenv,
			}
		}
	}

	// Parse services
	jd.Services, err = parseServices(rj.Services)
	if err != nil {
		return nil, fmt.Errorf("services: %w", err)
	}

	// Parse needs
	jd.Needs, err = parseStringListFromInterface(rj.Needs)
	if err != nil {
		return nil, fmt.Errorf("needs: %w", err)
	}

	// Parse dependencies
	jd.Dependencies, err = parseStringListFromInterface(rj.Dependencies)
	if err != nil {
		return nil, fmt.Errorf("dependencies: %w", err)
	}

	// Parse rules
	for _, rr := range rj.Rules {
		jd.Rules = append(jd.Rules, RuleDef{
			If:   rr.If,
			When: rr.When,
		})
	}

	// Parse allow_failure
	switch v := rj.AllowFailure.(type) {
	case bool:
		jd.AllowFailure = v
	case string:
		jd.AllowFailure = v == "true"
	}

	// Parse cache
	if rj.Cache != nil {
		jd.Cache = &CacheEntry{
			Key:    rj.Cache.Key,
			Paths:  rj.Cache.Paths,
			Policy: rj.Cache.Policy,
		}
	}

	// Parse timeout
	if rj.Timeout != "" {
		jd.Timeout = parseGitLabDuration(rj.Timeout)
	}

	// Parse retry
	if rj.Retry != nil {
		jd.Retry = parseRetry(rj.Retry)
	}

	// Parse parallel
	if rj.Parallel != nil {
		jd.Parallel = parseParallel(rj.Parallel)
	}

	// Parse resource_group
	jd.ResourceGroup = rj.ResourceGroup

	return jd, nil
}

// parseImageString extracts an image name from a string or object.
func parseImageString(v interface{}) string {
	switch img := v.(type) {
	case string:
		return img
	case map[string]interface{}:
		if name, ok := img["name"].(string); ok {
			return name
		}
	}
	return ""
}

// parseStringList parses a YAML value that can be a string or list of strings.
func parseStringList(v interface{}) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	switch val := v.(type) {
	case string:
		return []string{val}, nil
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", item)
			}
			result = append(result, s)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected string or list, got %T", v)
	}
}

// parseStringListFromInterface handles needs/dependencies that can be string, []string, or []object with "job" key.
func parseStringListFromInterface(v interface{}) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	switch val := v.(type) {
	case string:
		return []string{val}, nil
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			switch it := item.(type) {
			case string:
				result = append(result, it)
			case map[string]interface{}:
				// needs: [{job: "name"}]
				if job, ok := it["job"].(string); ok {
					result = append(result, job)
				}
			default:
				return nil, fmt.Errorf("expected string or object, got %T", item)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected string or list, got %T", v)
	}
}

// parseServices parses service definitions from YAML.
func parseServices(v interface{}) ([]ServiceEntry, error) {
	if v == nil {
		return nil, nil
	}
	list, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("services must be a list")
	}
	var services []ServiceEntry
	for _, item := range list {
		switch val := item.(type) {
		case string:
			services = append(services, ServiceEntry{Name: val})
		case map[string]interface{}:
			svc := ServiceEntry{}
			if name, ok := val["name"].(string); ok {
				svc.Name = name
			}
			if alias, ok := val["alias"].(string); ok {
				svc.Alias = alias
			}
			entrypoint, _ := parseStringList(val["entrypoint"])
			svc.Entrypoint = entrypoint
			command, _ := parseStringList(val["command"])
			svc.Command = command
			// Parse service-level variables
			if vars, ok := val["variables"].(map[string]interface{}); ok {
				svc.Variables = make(map[string]string, len(vars))
				for k, v := range vars {
					svc.Variables[k] = fmt.Sprintf("%v", v)
				}
			}
			services = append(services, svc)
		default:
			return nil, fmt.Errorf("service entry must be string or object, got %T", item)
		}
	}
	return services, nil
}

// parseParallel parses the parallel: configuration from raw YAML.
func parseParallel(v interface{}) *ParallelDef {
	switch val := v.(type) {
	case int:
		return &ParallelDef{Count: val}
	case float64:
		return &ParallelDef{Count: int(val)}
	case map[string]interface{}:
		// parallel: {matrix: [{...}]}
		if matrixRaw, ok := val["matrix"]; ok {
			if matrixList, ok := matrixRaw.([]interface{}); ok {
				var matrix []map[string][]string
				for _, item := range matrixList {
					if m, ok := item.(map[string]interface{}); ok {
						entry := make(map[string][]string)
						for k, v := range m {
							switch vv := v.(type) {
							case []interface{}:
								for _, s := range vv {
									entry[k] = append(entry[k], fmt.Sprintf("%v", s))
								}
							case string:
								entry[k] = []string{vv}
							default:
								entry[k] = []string{fmt.Sprintf("%v", vv)}
							}
						}
						matrix = append(matrix, entry)
					}
				}
				return &ParallelDef{Matrix: matrix}
			}
		}
	}
	return nil
}
