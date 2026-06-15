package bleeplab

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// TestArtifactFlow drives the full GitLab artifact lifecycle a real
// gitlab-runner exercises: a build job declares `artifacts:` and uploads an
// archive; a downstream test job's job-response advertises that build as a
// dependency (with the artifact size + a download token); the archive
// round-trips byte-for-byte through the object-store-backed artifact store.
func TestArtifactFlow(t *testing.T) {
	s := NewServer(":0", zerolog.Nop())
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	// Project + CI config: build-job produces an artifact, test-job consumes it.
	_, body := do(t, ts, http.MethodPost, "/api/v4/projects", map[string]any{"name": "demo"}, nil)
	var proj struct {
		ID int `json:"id"`
	}
	mustJSON(t, body, &proj)

	ci := "stages: [build, test]\n" +
		"build-job:\n  stage: build\n  script: [make]\n  artifacts:\n    paths: [calc]\n" +
		"test-job:\n  stage: test\n  script: [./calc]\n"
	do(t, ts, http.MethodPost, "/api/v4/projects/"+strconv.Itoa(proj.ID)+"/repository/commits",
		map[string]any{"branch": "main", "actions": []map[string]string{{"file_path": ".gitlab-ci.yml", "content": ci}}}, nil)

	_, body = do(t, ts, http.MethodPost, "/api/v4/user/runners", map[string]any{"runner_type": "project_type"}, nil)
	var runner struct {
		Token string `json:"token"`
	}
	mustJSON(t, body, &runner)
	do(t, ts, http.MethodPost, "/api/v4/projects/"+strconv.Itoa(proj.ID)+"/pipeline", map[string]any{"ref": "main"}, nil)

	// Claim build-job; it must advertise its artifact upload spec.
	build := claimJob(t, ts, runner.Token)
	if len(build.Artifacts) != 1 || len(build.Artifacts[0].Paths) != 1 || build.Artifacts[0].Paths[0] != "calc" {
		t.Fatalf("build job missing artifacts spec: %+v", build.Artifacts)
	}

	// Upload the build artifact (multipart `file` part, as the runner does).
	archive := []byte("PK\x03\x04 fake-zip-bytes for calc")
	uploadArtifact(t, ts, build.ID, build.Token, archive)
	// Mark build-job success so the test stage is enqueued.
	do(t, ts, http.MethodPut, "/api/v4/jobs/"+strconv.Itoa(build.ID),
		map[string]any{"token": build.Token, "state": "success"}, nil)

	// Claim test-job; it must depend on build-job with the artifact metadata.
	test := claimJob(t, ts, runner.Token)
	if len(test.Dependencies) != 1 {
		t.Fatalf("test job expected 1 dependency, got %d: %+v", len(test.Dependencies), test.Dependencies)
	}
	dep := test.Dependencies[0]
	if dep.ID != build.ID {
		t.Fatalf("dependency id %d != build id %d", dep.ID, build.ID)
	}
	if dep.ArtifactsFile == nil || dep.ArtifactsFile.Size != int64(len(archive)) {
		t.Fatalf("dependency artifacts_file %+v != size %d", dep.ArtifactsFile, len(archive))
	}

	// Download with the dependency's token (the runner's path) and verify bytes.
	got := downloadArtifact(t, ts, build.ID, dep.Token)
	if !bytes.Equal(got, archive) {
		t.Fatalf("downloaded artifact mismatch: got %q want %q", got, archive)
	}

	// The UI's internal job view surfaces the artifact filename + size.
	_, jbody := do(t, ts, http.MethodGet, "/internal/jobs/"+strconv.Itoa(build.ID), nil, nil)
	var jv struct {
		ArtifactSize     int64  `json:"artifact_size"`
		ArtifactFilename string `json:"artifact_filename"`
	}
	mustJSON(t, jbody, &jv)
	if jv.ArtifactSize != int64(len(archive)) || jv.ArtifactFilename == "" {
		t.Fatalf("internal job view artifact: size=%d filename=%q", jv.ArtifactSize, jv.ArtifactFilename)
	}

	// The UI's unauthenticated internal download returns the archive with the
	// recorded filename in Content-Disposition (no JOB-TOKEN needed).
	resp, dbody := do(t, ts, http.MethodGet, "/internal/jobs/"+strconv.Itoa(build.ID)+"/artifact", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("internal artifact download: status %d", resp.StatusCode)
	}
	if !bytes.Equal(dbody, archive) {
		t.Fatalf("internal artifact download mismatch: got %q want %q", dbody, archive)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, jv.ArtifactFilename) {
		t.Fatalf("Content-Disposition %q missing filename %q", cd, jv.ArtifactFilename)
	}
}

func mustJSON(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
}

func claimJob(t *testing.T, ts *httptest.Server, runnerToken string) jobResponse {
	t.Helper()
	resp, body := do(t, ts, http.MethodPost, "/api/v4/jobs/request", map[string]any{"token": runnerToken}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("claim job: status %d body %s", resp.StatusCode, body)
	}
	var jr jobResponse
	mustJSON(t, body, &jr)
	return jr
}

func uploadArtifact(t *testing.T, ts *httptest.Server, jobID int, token string, data []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "artifacts.zip")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v4/jobs/"+strconv.Itoa(jobID)+"/artifacts", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("JOB-TOKEN", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload artifact: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload artifact: status %d", resp.StatusCode)
	}
}

func downloadArtifact(t *testing.T, ts *httptest.Server, jobID int, token string) []byte {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v4/jobs/"+strconv.Itoa(jobID)+"/artifacts", nil)
	req.Header.Set("JOB-TOKEN", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("download artifact: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download artifact: status %d", resp.StatusCode)
	}
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return buf.Bytes()
}
