package gitlabhub

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"strconv"
)

func (s *Server) registerArtifactRoutes() {
	s.mux.HandleFunc("POST /api/v4/jobs/{id}/artifacts", s.handleUploadArtifacts)
	s.mux.HandleFunc("GET /api/v4/jobs/{id}/artifacts", s.handleDownloadArtifacts)
}

// handleUploadArtifacts handles POST /api/v4/jobs/:id/artifacts.
// The runner uploads a zip file as multipart/form-data with field "file".
func (s *Server) handleUploadArtifacts(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid job ID", http.StatusBadRequest)
		return
	}

	job := s.store.GetJob(jobID)
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	// Parse multipart form (max 256MB)
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		http.Error(w, "failed to parse multipart form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}

	s.store.StoreArtifact(jobID, data)

	// Check if this job has dotenv reports — parse them from the zip
	pipeline := s.store.GetPipeline(job.PipelineID)
	if pipeline != nil && pipeline.Def != nil {
		if jobDef, ok := pipeline.Def.Jobs[job.Name]; ok {
			if jobDef.Artifacts != nil && jobDef.Artifacts.Reports != nil && jobDef.Artifacts.Reports.Dotenv != "" {
				dotenvContent := extractFileFromZip(data, jobDef.Artifacts.Reports.Dotenv)
				if dotenvContent != "" {
					vars := parseDotenv(dotenvContent)
					s.store.mu.Lock()
					job.DotenvVars = vars
					s.store.mu.Unlock()
					s.logger.Info().
						Int("job_id", jobID).
						Int("dotenv_vars", len(vars)).
						Msg("parsed dotenv artifact")
				}
			}
		}
	}

	s.logger.Info().
		Int("job_id", jobID).
		Int("size", len(data)).
		Msg("artifact uploaded")

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":       jobID,
		"size":     len(data),
		"filename": "artifacts.zip",
	})
}

// handleDownloadArtifacts handles GET /api/v4/jobs/:id/artifacts.
func (s *Server) handleDownloadArtifacts(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid job ID", http.StatusBadRequest)
		return
	}

	data := s.store.GetArtifact(jobID)
	if data == nil {
		http.Error(w, "no artifacts found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Disposition", "attachment; filename=artifacts.zip")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// extractFileFromZip extracts a single file's contents from zip data.
func extractFileFromZip(zipData []byte, filename string) string {
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return ""
	}
	for _, f := range r.File {
		if f.Name == filename {
			rc, err := f.Open()
			if err != nil {
				return ""
			}
			defer rc.Close()
			buf := new(bytes.Buffer)
			buf.ReadFrom(rc)
			return buf.String()
		}
	}
	return ""
}
