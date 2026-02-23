package gitlabhub

import (
	"fmt"
)

// buildJobVariables constructs the full variables array for a job response.
func (s *Server) buildJobVariables(pipeline *Pipeline, job *PipelineJob, jobDef *PipelineJobDef) []VariableDef {
	serverURL := pipeline.ServerURL
	project := s.store.GetProject(pipeline.ProjectID)
	projectName := ""
	if project != nil {
		projectName = project.Name
	}

	vars := []VariableDef{
		{Key: "CI", Value: "true", Public: true},
		{Key: "GITLAB_CI", Value: "true", Public: true},
		{Key: "CI_SERVER", Value: "yes", Public: true},
		{Key: "CI_SERVER_URL", Value: fmt.Sprintf("http://%s", serverURL), Public: true},
		{Key: "CI_SERVER_HOST", Value: serverURL, Public: true},
		{Key: "CI_SERVER_PORT", Value: "80", Public: true},
		{Key: "CI_SERVER_PROTOCOL", Value: "http", Public: true},
		{Key: "CI_SERVER_NAME", Value: "gitlabhub", Public: true},
		{Key: "CI_SERVER_VERSION", Value: "17.0.0", Public: true},
		{Key: "CI_API_V4_URL", Value: fmt.Sprintf("http://%s/api/v4", serverURL), Public: true},
		{Key: "CI_JOB_ID", Value: fmt.Sprintf("%d", job.ID), Public: true},
		{Key: "CI_JOB_TOKEN", Value: job.Token, Public: false},
		{Key: "CI_JOB_NAME", Value: job.Name, Public: true},
		{Key: "CI_JOB_STAGE", Value: job.Stage, Public: true},
		{Key: "CI_JOB_STATUS", Value: "running", Public: true},
		{Key: "CI_PIPELINE_ID", Value: fmt.Sprintf("%d", pipeline.ID), Public: true},
		{Key: "CI_PIPELINE_SOURCE", Value: "push", Public: true},
		{Key: "CI_PIPELINE_URL", Value: fmt.Sprintf("http://%s/%s/-/pipelines/%d", serverURL, projectName, pipeline.ID), Public: true},
		{Key: "CI_PROJECT_ID", Value: fmt.Sprintf("%d", pipeline.ProjectID), Public: true},
		{Key: "CI_PROJECT_NAME", Value: projectName, Public: true},
		{Key: "CI_PROJECT_PATH", Value: projectName, Public: true},
		{Key: "CI_PROJECT_URL", Value: fmt.Sprintf("http://%s/%s", serverURL, projectName), Public: true},
		{Key: "CI_PROJECT_DIR", Value: fmt.Sprintf("/builds/%s", projectName), Public: true},
		{Key: "CI_COMMIT_SHA", Value: pipeline.Sha, Public: true},
		{Key: "CI_COMMIT_SHORT_SHA", Value: shortSha(pipeline.Sha), Public: true},
		{Key: "CI_COMMIT_REF_NAME", Value: pipeline.Ref, Public: true},
		{Key: "CI_COMMIT_BRANCH", Value: pipeline.Ref, Public: true},
		{Key: "CI_COMMIT_REF_SLUG", Value: pipeline.Ref, Public: true},
		{Key: "CI_REPOSITORY_URL", Value: fmt.Sprintf("http://gitlab-ci-token:%s@%s/%s.git", job.Token, serverURL, projectName), Public: false},
		{Key: "CI_BUILDS_DIR", Value: "/builds", Public: true},
		{Key: "CI_CONCURRENT_ID", Value: "0", Public: true},
		{Key: "CI_CONCURRENT_PROJECT_ID", Value: "0", Public: true},
	}

	// Add user-defined variables from pipeline YAML (already merged global+job in parser)
	if jobDef != nil && jobDef.Variables != nil {
		for k, v := range jobDef.Variables {
			vars = append(vars, VariableDef{
				Key:    k,
				Value:  v,
				Public: true,
			})
		}
	}

	// Add project-level variables
	if project != nil {
		for _, pv := range project.Variables {
			if pv.Protected {
				// Only inject protected vars on protected branches (we treat all as protected)
			}
			vars = append(vars, VariableDef{
				Key:    pv.Key,
				Value:  pv.Value,
				Public: !pv.Masked,
				Masked: pv.Masked,
			})
		}
	}

	return vars
}

func shortSha(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
