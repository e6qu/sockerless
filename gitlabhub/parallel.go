package gitlabhub

import (
	"fmt"
	"sort"
	"strings"
)

// ParallelDef represents the parallel: configuration.
type ParallelDef struct {
	// Simple: parallel: N (expand to N copies)
	Count int
	// Matrix: parallel:matrix: [{VAR1: [a,b], VAR2: [c,d]}]
	Matrix []map[string][]string
}

// expandParallelJobs expands jobs with parallel: or parallel:matrix: into multiple jobs.
// Called after ParsePipeline produces the initial PipelineDef.
func expandParallelJobs(def *PipelineDef) {
	expanded := make(map[string]*PipelineJobDef)
	for name, job := range def.Jobs {
		if job.Parallel == nil {
			expanded[name] = job
			continue
		}

		if job.Parallel.Count > 0 {
			// Simple parallel: N -> "job 1/N", "job 2/N", etc.
			for i := 1; i <= job.Parallel.Count; i++ {
				newName := fmt.Sprintf("%s %d/%d", name, i, job.Parallel.Count)
				clone := cloneJobDef(job)
				clone.Parallel = nil
				// Inject CI_NODE_INDEX and CI_NODE_TOTAL
				if clone.Variables == nil {
					clone.Variables = make(map[string]string)
				}
				clone.Variables["CI_NODE_INDEX"] = fmt.Sprintf("%d", i)
				clone.Variables["CI_NODE_TOTAL"] = fmt.Sprintf("%d", job.Parallel.Count)
				clone.MatrixGroup = name
				expanded[newName] = clone
			}
		} else if len(job.Parallel.Matrix) > 0 {
			// Matrix expansion: cartesian product
			combos := expandMatrix(job.Parallel.Matrix)
			for _, combo := range combos {
				// Build display name: "job (val1, val2)"
				var parts []string
				// Sort keys for deterministic naming
				keys := make([]string, 0, len(combo))
				for k := range combo {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					parts = append(parts, combo[k])
				}
				newName := fmt.Sprintf("%s (%s)", name, strings.Join(parts, ", "))
				clone := cloneJobDef(job)
				clone.Parallel = nil
				if clone.Variables == nil {
					clone.Variables = make(map[string]string)
				}
				for k, v := range combo {
					clone.Variables[k] = v
				}
				clone.MatrixGroup = name
				expanded[newName] = clone
			}
		}
	}
	def.Jobs = expanded
}

// expandMatrix produces the cartesian product of all matrix entries.
func expandMatrix(matrix []map[string][]string) []map[string]string {
	var result []map[string]string
	for _, entry := range matrix {
		// Get sorted keys for determinism
		keys := make([]string, 0, len(entry))
		for k := range entry {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Cartesian product for this entry
		combos := []map[string]string{{}}
		for _, key := range keys {
			vals := entry[key]
			var newCombos []map[string]string
			for _, combo := range combos {
				for _, val := range vals {
					c := make(map[string]string, len(combo)+1)
					for k, v := range combo {
						c[k] = v
					}
					c[key] = val
					newCombos = append(newCombos, c)
				}
			}
			combos = newCombos
		}
		result = append(result, combos...)
	}
	return result
}

// cloneJobDef creates a deep copy of a PipelineJobDef.
func cloneJobDef(j *PipelineJobDef) *PipelineJobDef {
	clone := *j
	clone.Script = append([]string{}, j.Script...)
	if j.BeforeScript != nil {
		clone.BeforeScript = append([]string{}, j.BeforeScript...)
	}
	if j.AfterScript != nil {
		clone.AfterScript = append([]string{}, j.AfterScript...)
	}
	if j.Variables != nil {
		clone.Variables = make(map[string]string, len(j.Variables))
		for k, v := range j.Variables {
			clone.Variables[k] = v
		}
	}
	if j.Needs != nil {
		clone.Needs = append([]string{}, j.Needs...)
	}
	if j.Dependencies != nil {
		clone.Dependencies = append([]string{}, j.Dependencies...)
	}
	if j.Services != nil {
		clone.Services = append([]ServiceEntry{}, j.Services...)
	}
	if j.Rules != nil {
		clone.Rules = append([]RuleDef{}, j.Rules...)
	}
	// Artifacts and Cache are pointers -- shallow copy is fine (not modified per-clone)
	return &clone
}
