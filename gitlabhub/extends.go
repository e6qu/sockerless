package gitlabhub

import "fmt"

// resolveExtends processes `extends:` keywords in the raw YAML top-level map,
// deep-merging template definitions into jobs.
// Must be called BEFORE job normalization.
//
// Deep merge rules (per GitLab docs):
// - script, before_script, after_script, services, artifacts, cache, rules: REPLACED entirely
// - variables: MERGED (job overrides template)
// - Scalars (image, stage, when, allow_failure): REPLACED
// - Multi-level chains (A extends B extends C) supported
// - extends: [.a, .b] list form supported (later overrides earlier)
// - Circular detection via visited set
func resolveExtends(topLevel map[string]interface{}) error {
	// Build template map: all dot-prefixed keys
	templates := make(map[string]map[string]interface{})
	for key, val := range topLevel {
		if len(key) > 0 && key[0] == '.' {
			if m, ok := val.(map[string]interface{}); ok {
				templates[key] = m
			}
		}
	}

	// Process each non-reserved, non-template key
	for key, val := range topLevel {
		if len(key) == 0 {
			continue
		}
		if key[0] == '.' || reservedKeys[key] {
			continue
		}
		m, ok := val.(map[string]interface{})
		if !ok {
			continue
		}
		resolved, err := resolveExtendsChain(key, m, templates, topLevel, nil)
		if err != nil {
			return err
		}
		topLevel[key] = resolved
	}
	return nil
}

// resolveExtendsChain recursively resolves extends for a single job/template.
func resolveExtendsChain(name string, job map[string]interface{}, templates map[string]map[string]interface{}, topLevel map[string]interface{}, visited map[string]bool) (map[string]interface{}, error) {
	if visited == nil {
		visited = make(map[string]bool)
	}
	if visited[name] {
		return nil, fmt.Errorf("circular extends detected: %s", name)
	}
	visited[name] = true

	extendsRaw, ok := job["extends"]
	if !ok {
		return job, nil
	}
	delete(job, "extends")

	// Normalize extends to list
	var extendsList []string
	switch v := extendsRaw.(type) {
	case string:
		extendsList = []string{v}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				extendsList = append(extendsList, s)
			}
		}
	}

	// Start with empty base, merge each parent in order, then merge job on top
	base := make(map[string]interface{})
	for _, parentName := range extendsList {
		parent, found := lookupTemplate(parentName, templates, topLevel)
		if !found {
			return nil, fmt.Errorf("extends: template %q not found", parentName)
		}
		// Recursively resolve parent's extends first
		parentCopy := copyMap(parent)
		resolved, err := resolveExtendsChain(parentName, parentCopy, templates, topLevel, copyVisited(visited))
		if err != nil {
			return nil, err
		}
		deepMergeGitLab(base, resolved)
	}

	// Merge job on top of base
	deepMergeGitLab(base, job)
	return base, nil
}

// deepMergeGitLab merges src into dst following GitLab merge rules.
func deepMergeGitLab(dst, src map[string]interface{}) {
	replaceKeys := map[string]bool{
		"script": true, "before_script": true, "after_script": true,
		"services": true, "artifacts": true, "cache": true, "rules": true,
	}
	for key, srcVal := range src {
		if replaceKeys[key] {
			dst[key] = srcVal
			continue
		}
		if key == "variables" {
			// Merge variables: dst is the base, src overrides
			dstVars, _ := dst[key].(map[string]interface{})
			srcVars, _ := srcVal.(map[string]interface{})
			if dstVars == nil {
				dstVars = make(map[string]interface{})
			}
			for k, v := range srcVars {
				dstVars[k] = v
			}
			dst[key] = dstVars
			continue
		}
		dst[key] = srcVal
	}
}

// lookupTemplate finds a template by name, first in the templates map, then in topLevel.
func lookupTemplate(name string, templates map[string]map[string]interface{}, topLevel map[string]interface{}) (map[string]interface{}, bool) {
	if m, ok := templates[name]; ok {
		return m, true
	}
	// Fall back to topLevel for non-dot-prefixed extends targets
	if raw, ok := topLevel[name]; ok {
		if m, ok := raw.(map[string]interface{}); ok {
			return m, true
		}
	}
	return nil, false
}

// copyMap creates a shallow copy of a map.
func copyMap(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// copyVisited creates a copy of the visited set for branch-independent recursion.
func copyVisited(visited map[string]bool) map[string]bool {
	out := make(map[string]bool, len(visited))
	for k, v := range visited {
		out[k] = v
	}
	return out
}
