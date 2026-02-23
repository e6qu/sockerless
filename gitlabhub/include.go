package gitlabhub

import (
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"gopkg.in/yaml.v3"
)

// ResolveIncludes processes `include:` directives in a .gitlab-ci.yml, reads
// the referenced local files from the git storage, and returns merged YAML.
// Supports: include: "path", include: [{local: "path"}], include: {local: "path"}
func ResolveIncludes(yamlBytes []byte, gitStorage *memory.Storage) ([]byte, error) {
	if gitStorage == nil {
		return yamlBytes, nil
	}

	var topLevel map[string]interface{}
	if err := yaml.Unmarshal(yamlBytes, &topLevel); err != nil {
		return nil, fmt.Errorf("parse YAML for includes: %w", err)
	}

	includeRaw, ok := topLevel["include"]
	if !ok {
		return yamlBytes, nil
	}

	// Parse include paths
	paths, err := parseIncludePaths(includeRaw)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return yamlBytes, nil
	}

	// Read and merge each included file
	for _, path := range paths {
		content, err := readFileFromGit(gitStorage, path)
		if err != nil {
			return nil, fmt.Errorf("include %q: %w", path, err)
		}

		var included map[string]interface{}
		if err := yaml.Unmarshal([]byte(content), &included); err != nil {
			return nil, fmt.Errorf("include %q: parse YAML: %w", path, err)
		}

		// Merge included into topLevel (included is base, topLevel overrides)
		mergeIncluded(topLevel, included)
	}

	// Remove the include key from the merged result
	delete(topLevel, "include")

	// Re-marshal to YAML
	return yaml.Marshal(topLevel)
}

// parseIncludePaths extracts local file paths from the include: directive.
func parseIncludePaths(v interface{}) ([]string, error) {
	switch val := v.(type) {
	case string:
		return []string{val}, nil
	case map[string]interface{}:
		if local, ok := val["local"].(string); ok {
			return []string{local}, nil
		}
		return nil, fmt.Errorf("include: missing 'local' key")
	case []interface{}:
		var paths []string
		for _, item := range val {
			switch it := item.(type) {
			case string:
				paths = append(paths, it)
			case map[string]interface{}:
				if local, ok := it["local"].(string); ok {
					paths = append(paths, local)
				}
			}
		}
		return paths, nil
	}
	return nil, nil
}

// mergeIncluded merges an included YAML document into the main document.
// Rules: stages are unioned, variables merge (main overrides), jobs from included
// are added if not already present in main.
func mergeIncluded(main, included map[string]interface{}) {
	for key, inclVal := range included {
		if key == "include" {
			continue // don't propagate nested includes
		}

		mainVal, exists := main[key]
		if !exists {
			main[key] = inclVal
			continue
		}

		switch key {
		case "stages":
			// Union stages (preserve order, included first, main additions after)
			main[key] = unionStages(inclVal, mainVal)
		case "variables":
			// Merge: included is base, main overrides
			merged := make(map[string]interface{})
			if m, ok := inclVal.(map[string]interface{}); ok {
				for k, v := range m {
					merged[k] = v
				}
			}
			if m, ok := mainVal.(map[string]interface{}); ok {
				for k, v := range m {
					merged[k] = v // main overrides
				}
			}
			main[key] = merged
		default:
			// For jobs and other keys: main takes precedence (don't override)
			// So do nothing - main already has this key
		}
	}
}

// unionStages merges two stage lists, preserving order (included first).
func unionStages(a, b interface{}) []interface{} {
	seen := make(map[string]bool)
	var result []interface{}

	addStages := func(v interface{}) {
		if list, ok := v.([]interface{}); ok {
			for _, item := range list {
				if s, ok := item.(string); ok {
					if !seen[s] {
						seen[s] = true
						result = append(result, s)
					}
				}
			}
		}
	}

	addStages(a)
	addStages(b)
	return result
}

// readFileFromGit reads a file from go-git in-memory storage.
func readFileFromGit(stor *memory.Storage, path string) (string, error) {
	// Strip leading slash
	path = strings.TrimPrefix(path, "/")

	// Get HEAD ref
	ref, err := stor.Reference(plumbing.HEAD)
	if err != nil {
		// Try refs/heads/main
		ref, err = stor.Reference(plumbing.NewBranchReferenceName("main"))
		if err != nil {
			return "", fmt.Errorf("cannot resolve HEAD: %w", err)
		}
	}

	// Resolve symbolic ref
	target := ref.Hash()
	if ref.Type() == plumbing.SymbolicReference {
		realRef, err := stor.Reference(ref.Target())
		if err != nil {
			return "", fmt.Errorf("cannot resolve symbolic ref: %w", err)
		}
		target = realRef.Hash()
	}

	// Get commit
	commitObj, err := stor.EncodedObject(plumbing.CommitObject, target)
	if err != nil {
		return "", fmt.Errorf("cannot get commit: %w", err)
	}
	commit, err := object.DecodeCommit(stor, commitObj)
	if err != nil {
		return "", fmt.Errorf("cannot decode commit: %w", err)
	}

	// Get tree
	tree, err := object.GetTree(stor, commit.TreeHash)
	if err != nil {
		return "", fmt.Errorf("cannot get tree: %w", err)
	}

	// Try tree.File first (works with properly nested trees)
	file, err := tree.File(path)
	if err == nil {
		return readBlobContent(file)
	}

	// Fall back: scan flat tree entries by exact name match.
	// This handles the case where createProjectRepo stores files as flat
	// entries (e.g. "templates/jobs.yml" as a single entry name).
	for _, entry := range tree.Entries {
		if entry.Name == path {
			blobObj, blobErr := stor.EncodedObject(plumbing.BlobObject, entry.Hash)
			if blobErr != nil {
				return "", fmt.Errorf("cannot get blob for %s: %w", path, blobErr)
			}
			reader, rErr := blobObj.Reader()
			if rErr != nil {
				return "", rErr
			}
			defer reader.Close()
			data, dErr := io.ReadAll(reader)
			if dErr != nil {
				return "", dErr
			}
			return string(data), nil
		}
	}

	return "", fmt.Errorf("file not found: %s", path)
}

// readBlobContent reads the full content of a git file object.
func readBlobContent(file *object.File) (string, error) {
	reader, err := file.Reader()
	if err != nil {
		return "", err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
