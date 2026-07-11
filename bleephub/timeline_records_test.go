package bleephub

// Timeline-record + log-upload coverage: the runner-facing
// /_apis/v1/Timeline, /_apis/v1/Logfiles and TimeLineWebConsoleLog routes
// are driven the way the official actions/runner drives them (PATCH with
// a VssJsonCollectionWrapper body, repeated record updates as state
// advances, block-wise log upload), and the GitHub-shape jobs API is
// asserted to serve the reported records rather than synthesized steps.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// linkJobToPlan installs the broker-side Job entry that ties a WorkflowJob
// to its plan, mirroring what dispatchWorkflowJob records when it sends
// the job to a runner.
func linkJobToPlan(t *testing.T, s *Server, wfJob *WorkflowJob) (planID, timelineID string) {
	t.Helper()
	planID = uuid.New().String()
	timelineID = uuid.New().String()
	s.store.mu.Lock()
	s.store.Jobs[wfJob.JobID] = &Job{
		ID:         wfJob.JobID,
		PlanID:     planID,
		TimelineID: timelineID,
		Status:     "running",
	}
	s.store.mu.Unlock()
	return planID, timelineID
}

// patchTimelineRecords PATCHes records through the real timeline route.
// wrapped=true sends the VssJsonCollectionWrapper {"count","value"} body
// the official actions/runner sends; wrapped=false sends a bare array.
func patchTimelineRecords(t *testing.T, s *Server, planID, timelineID string, wrapped bool, records []map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	var payload any = records
	if wrapped {
		payload = map[string]any{"count": len(records), "value": records}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal records: %v", err)
	}
	req := httptest.NewRequest("PATCH",
		fmt.Sprintf("/_apis/v1/Timeline/%s/build/%s/%s", uuid.New().String(), planID, timelineID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH timeline records: status = %d, body = %s", w.Code, w.Body.String())
	}
	return w
}

// createLogFile POSTs the Logfiles create route and returns the minted ID.
func createLogFile(t *testing.T, s *Server, planID string) int {
	t.Helper()
	req := httptest.NewRequest("POST",
		fmt.Sprintf("/_apis/v1/Logfiles/%s/build/%s", uuid.New().String(), planID),
		strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create log: status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode create-log response: %v", err)
	}
	return resp.ID
}

// uploadLogBlock POSTs one content block to the Logfiles upload route.
func uploadLogBlock(t *testing.T, s *Server, planID string, logID int, content []byte) {
	t.Helper()
	req := httptest.NewRequest("POST",
		fmt.Sprintf("/_apis/v1/Logfiles/%s/build/%s/%d", uuid.New().String(), planID, logID),
		bytes.NewReader(content))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("upload log block: status = %d, body = %s", w.Code, w.Body.String())
	}
}

// postConsoleLines POSTs live console lines the way the runner feeds the
// web console log route (a TimelineRecordFeedLinesWrapper body).
func postConsoleLines(t *testing.T, s *Server, planID, timelineID string, lines []string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"count":  len(lines),
		"value":  lines,
		"stepId": uuid.New().String(),
	})
	req := httptest.NewRequest("POST",
		fmt.Sprintf("/_apis/v1/TimeLineWebConsoleLog/%s/build/%s/%s/%s",
			uuid.New().String(), planID, timelineID, uuid.New().String()),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("post console lines: status = %d, body = %s", w.Code, w.Body.String())
	}
}

func newTimelineTestServer() *Server {
	s := newTestServer()
	s.registerTimelineRoutes()
	s.registerGHActionsRoutes()
	return s
}

func storedRecords(s *Server, planID string) []TimelineRecord {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	out := make([]TimelineRecord, 0, len(s.store.TimelineRecords[planID]))
	for _, rec := range s.store.TimelineRecords[planID] {
		out = append(out, *rec)
	}
	return out
}

func TestTimelineRecords_WrapperBodyStored(t *testing.T) {
	s := newTimelineTestServer()
	_, wfJob := seedRun(t, s, "octo/repo", "running", "")
	planID, timelineID := linkJobToPlan(t, s, wfJob)

	recID := uuid.New().String()
	w := patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": recID, "type": "Task", "name": "Step one", "order": 1, "state": "pending"},
	})

	var resp struct {
		Count int               `json:"count"`
		Value []*TimelineRecord `json:"value"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Count != 1 || len(resp.Value) != 1 {
		t.Fatalf("response = count:%d len:%d, want 1/1", resp.Count, len(resp.Value))
	}

	got := storedRecords(s, planID)
	if len(got) != 1 {
		t.Fatalf("stored records = %d, want 1", len(got))
	}
	if got[0].ID != recID || got[0].Type != "Task" || got[0].Name != "Step one" || got[0].State != "pending" {
		t.Errorf("stored record = %+v", got[0])
	}
}

func TestTimelineRecords_BareArrayBodyStored(t *testing.T) {
	s := newTimelineTestServer()
	_, wfJob := seedRun(t, s, "octo/repo", "running", "")
	planID, timelineID := linkJobToPlan(t, s, wfJob)

	recID := uuid.New().String()
	patchTimelineRecords(t, s, planID, timelineID, false, []map[string]any{
		{"id": recID, "type": "Task", "name": "Bare", "order": 1, "state": "inProgress"},
	})

	got := storedRecords(s, planID)
	if len(got) != 1 || got[0].ID != recID || got[0].State != "inProgress" {
		t.Fatalf("stored records = %+v, want the bare-array record", got)
	}
}

func TestTimelineRecords_InvalidBodyRejected(t *testing.T) {
	s := newTimelineTestServer()
	req := httptest.NewRequest("PATCH",
		fmt.Sprintf("/_apis/v1/Timeline/%s/build/%s/%s", uuid.New().String(), uuid.New().String(), uuid.New().String()),
		strings.NewReader("not json"))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestTimelineRecords_UpsertMergesStateAdvance(t *testing.T) {
	s := newTimelineTestServer()
	_, wfJob := seedRun(t, s, "octo/repo", "running", "")
	planID, timelineID := linkJobToPlan(t, s, wfJob)
	recID := uuid.New().String()

	// pending → inProgress (start time appears) → completed (result +
	// finish time + log ref appear), the way the runner re-PATCHes.
	patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": recID, "type": "Task", "name": "Run tests", "order": 1, "state": "pending"},
	})
	patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": recID, "type": "Task", "name": "Run tests", "order": 1,
			"state": "inProgress", "startTime": "2026-06-12T10:00:00Z"},
	})
	patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": recID, "type": "Task", "name": "Run tests", "order": 1,
			"state": "completed", "result": "succeeded",
			"startTime": "2026-06-12T10:00:00Z", "finishTime": "2026-06-12T10:00:09Z",
			"log": map[string]any{"id": 7}},
	})

	got := storedRecords(s, planID)
	if len(got) != 1 {
		t.Fatalf("stored records = %d, want 1 (upsert by ID, not append)", len(got))
	}
	rec := got[0]
	if rec.State != "completed" || rec.Result != "succeeded" {
		t.Errorf("state/result = %q/%q, want completed/succeeded", rec.State, rec.Result)
	}
	if rec.StartTime != "2026-06-12T10:00:00Z" || rec.FinishTime != "2026-06-12T10:00:09Z" {
		t.Errorf("times = %q/%q", rec.StartTime, rec.FinishTime)
	}
	if rec.Log == nil || rec.Log.ID != 7 {
		t.Errorf("log ref = %+v, want id 7", rec.Log)
	}
}

func TestTimelineRecords_NoRegressOnPartialUpdate(t *testing.T) {
	s := newTimelineTestServer()
	_, wfJob := seedRun(t, s, "octo/repo", "running", "")
	planID, timelineID := linkJobToPlan(t, s, wfJob)
	recID := uuid.New().String()

	patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": recID, "type": "Task", "name": "Run tests", "order": 3,
			"state": "completed", "result": "failed",
			"startTime": "2026-06-12T10:00:00Z", "finishTime": "2026-06-12T10:00:09Z",
			"log": map[string]any{"id": 4}},
	})
	// A later partial update that omits result / times / log / name must
	// not erase the previously reported values.
	patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": recID, "state": "completed"},
	})

	got := storedRecords(s, planID)
	if len(got) != 1 {
		t.Fatalf("stored records = %d, want 1", len(got))
	}
	rec := got[0]
	if rec.Result != "failed" {
		t.Errorf("result regressed to %q, want failed", rec.Result)
	}
	if rec.StartTime == "" || rec.FinishTime == "" {
		t.Errorf("times regressed: %q/%q", rec.StartTime, rec.FinishTime)
	}
	if rec.Log == nil || rec.Log.ID != 4 {
		t.Errorf("log ref regressed: %+v", rec.Log)
	}
	if rec.Name != "Run tests" || rec.Type != "Task" || rec.Order != 3 {
		t.Errorf("identity fields regressed: %+v", rec)
	}
}

func TestJobSteps_ResultMappingTable(t *testing.T) {
	s := newTimelineTestServer()
	_, wfJob := seedRun(t, s, "octo/repo", "completed", "success")
	planID, timelineID := linkJobToPlan(t, s, wfJob)

	cases := []struct {
		result string
		want   string
	}{
		{"succeeded", "success"},
		{"succeededWithIssues", "success"},
		{"failed", "failure"},
		{"canceled", "cancelled"},
		{"skipped", "skipped"},
		{"abandoned", "cancelled"},
	}
	records := make([]map[string]any, 0, len(cases))
	for i, c := range cases {
		records = append(records, map[string]any{
			"id": uuid.New().String(), "type": "Task",
			"name": "step-" + c.result, "order": i + 1,
			"state": "completed", "result": c.result,
		})
	}
	patchTimelineRecords(t, s, planID, timelineID, true, records)

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/jobs/%d", stableJobID(wfJob.JobID)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var got struct {
		Steps []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			Number     int    `json:"number"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Steps) != len(cases) {
		t.Fatalf("steps len = %d, want %d", len(got.Steps), len(cases))
	}
	for i, c := range cases {
		st := got.Steps[i]
		if st.Status != "completed" {
			t.Errorf("step %q status = %q, want completed", st.Name, st.Status)
		}
		if st.Conclusion != c.want {
			t.Errorf("step %q conclusion = %q, want %q", st.Name, st.Conclusion, c.want)
		}
		if st.Number != i+1 {
			t.Errorf("step %q number = %d, want %d", st.Name, st.Number, i+1)
		}
	}
}

func TestJobSteps_StateMappingAndOrder(t *testing.T) {
	s := newTimelineTestServer()
	_, wfJob := seedRun(t, s, "octo/repo", "running", "")
	planID, timelineID := linkJobToPlan(t, s, wfJob)

	// Reported out of order, mixed states, with a Job record in between
	// that must not surface as a step.
	patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": uuid.New().String(), "type": "Task", "name": "third", "order": 3, "state": "pending"},
		{"id": uuid.New().String(), "type": "Job", "name": "Build", "order": 1, "state": "inProgress"},
		{"id": uuid.New().String(), "type": "Task", "name": "first", "order": 1,
			"state": "completed", "result": "succeeded"},
		{"id": uuid.New().String(), "type": "Task", "name": "second", "order": 2, "state": "inProgress"},
	})

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/jobs/%d", stableJobID(wfJob.JobID)))
	var got struct {
		Steps []struct {
			Name       string  `json:"name"`
			Status     string  `json:"status"`
			Conclusion *string `json:"conclusion"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Steps) != 3 {
		t.Fatalf("steps len = %d, want 3 (Job record excluded)", len(got.Steps))
	}
	wantNames := []string{"first", "second", "third"}
	wantStatus := []string{"completed", "in_progress", "queued"}
	for i := range wantNames {
		if got.Steps[i].Name != wantNames[i] {
			t.Errorf("step %d name = %q, want %q (sorted by Order)", i, got.Steps[i].Name, wantNames[i])
		}
		if got.Steps[i].Status != wantStatus[i] {
			t.Errorf("step %d status = %q, want %q", i, got.Steps[i].Status, wantStatus[i])
		}
	}
	if got.Steps[1].Conclusion != nil || got.Steps[2].Conclusion != nil {
		t.Errorf("non-completed steps must have null conclusion: %+v", got.Steps)
	}
}

func TestJobSteps_NoRecordsMeansEmptyArray(t *testing.T) {
	s := newTimelineTestServer()
	_, wfJob := seedRun(t, s, "octo/repo", "completed", "success")
	// Step definitions exist, but the runner never reported records —
	// the steps array must be empty, never fabricated from the defs.
	s.store.mu.Lock()
	wfJob.Def = &JobDef{Steps: []StepDef{
		{Name: "Checkout", Uses: "actions/checkout@v4"},
		{Run: "go test ./..."},
	}}
	s.store.mu.Unlock()
	linkJobToPlan(t, s, wfJob)

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/jobs/%d", stableJobID(wfJob.JobID)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	steps, ok := got["steps"]
	if !ok {
		t.Fatal("steps member missing")
	}
	if string(steps) != "[]" {
		t.Errorf("steps = %s, want [] (no fabricated steps)", steps)
	}
}

func TestLogfilesUpload_AppendsBlocks(t *testing.T) {
	s := newTimelineTestServer()
	planID := uuid.New().String()
	logID := createLogFile(t, s, planID)

	uploadLogBlock(t, s, planID, logID, []byte("hello\n"))
	uploadLogBlock(t, s, planID, logID, []byte("world\n"))

	s.store.mu.RLock()
	got := string(s.store.LogFiles[logID])
	s.store.mu.RUnlock()
	if got != "hello\nworld\n" {
		t.Errorf("stored log = %q, want blocks appended", got)
	}
}

func TestLogfilesUpload_WritesObjectStore(t *testing.T) {
	fs := newS3FSForTest(t)
	objectFS := &s3FS{client: fs.client, bucket: fs.bucket, prefix: "objects"}
	s := newTimelineTestServer()
	s.artifactStore = NewArtifactStoreWithByteStore("", &s3ActionsByteStore{fs: objectFS})
	planID := uuid.New().String()
	logID := createLogFile(t, s, planID)

	uploadLogBlock(t, s, planID, logID, []byte("object-backed log\n"))

	got := readS3TestFile(t, objectFS, fmt.Sprintf("actions/logs/%d/data", logID))
	if string(got) != "object-backed log\n" {
		t.Fatalf("s3 log data = %q", string(got))
	}
}

func TestLogfilesUpload_ObjectStoreFailurePreservesState(t *testing.T) {
	fs := newS3FSForTest(t)
	objectFS := &s3FS{client: fs.client, bucket: "missing-bucket", prefix: "objects"}
	s := newTimelineTestServer()
	s.artifactStore = NewArtifactStoreWithByteStore("", &s3ActionsByteStore{fs: objectFS})
	planID := uuid.New().String()
	logID := createLogFile(t, s, planID)

	s.store.mu.Lock()
	s.store.LogFiles[logID] = []byte("kept\n")
	s.store.mu.Unlock()

	req := httptest.NewRequest("POST",
		fmt.Sprintf("/_apis/v1/Logfiles/%s/build/%s/%d", uuid.New().String(), planID, logID),
		bytes.NewReader([]byte("not durable\n")))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	s.store.mu.RLock()
	got := string(s.store.LogFiles[logID])
	s.store.mu.RUnlock()
	if got != "kept\n" {
		t.Fatalf("log state = %q, want previous bytes after object-store failure", got)
	}
}

func TestJobLogs_ReadsUploadedLogFilesFromObjectStore(t *testing.T) {
	fs := newS3FSForTest(t)
	objectFS := &s3FS{client: fs.client, bucket: fs.bucket, prefix: "objects"}
	s := newTimelineTestServer()
	s.artifactStore = NewArtifactStoreWithByteStore("", &s3ActionsByteStore{fs: objectFS})
	_, wfJob := seedRun(t, s, "octo/repo", "completed", "success")
	planID, timelineID := linkJobToPlan(t, s, wfJob)

	logID := createLogFile(t, s, planID)
	uploadLogBlock(t, s, planID, logID, []byte("object-store job log\n"))
	patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": uuid.New().String(), "type": "Task", "name": "object step", "order": 1,
			"state": "completed", "result": "succeeded", "log": map[string]any{"id": logID}},
	})

	s.store.mu.Lock()
	delete(s.store.LogFiles, logID)
	s.store.mu.Unlock()

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/jobs/%d/logs", stableJobID(wfJob.JobID)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "object-store job log\n" {
		t.Fatalf("job log body = %q, want object-store bytes", got)
	}
}

func TestRunLogsDelete_ObjectStoreFailurePreservesState(t *testing.T) {
	fs := newS3FSForTest(t)
	objectFS := &s3FS{client: fs.client, bucket: "missing-bucket", prefix: "objects"}
	s := newTimelineTestServer()
	s.registerGHActionsPermissionsRoutes()
	s.artifactStore = NewArtifactStoreWithByteStore("", &s3ActionsByteStore{fs: objectFS})
	wf, wfJob := seedRun(t, s, "octo/repo", "completed", "success")
	planID, timelineID := linkJobToPlan(t, s, wfJob)

	logID := createLogFile(t, s, planID)
	s.store.mu.Lock()
	s.store.LogFiles[logID] = []byte("still visible\n")
	s.store.LogLines[wfJob.JobID] = []string{"console line"}
	s.store.mu.Unlock()
	patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": uuid.New().String(), "type": "Task", "name": "object step", "order": 1,
			"state": "completed", "result": "succeeded", "log": map[string]any{"id": logID}},
	})

	w := runAuthedRequest(s, "DELETE", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d/logs", wf.RunID))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	s.store.mu.RLock()
	_, hasLog := s.store.LogFiles[logID]
	_, hasTimeline := s.store.TimelineRecords[planID]
	lines := append([]string(nil), s.store.LogLines[wfJob.JobID]...)
	s.store.mu.RUnlock()
	if !hasLog || !hasTimeline || len(lines) != 1 || lines[0] != "console line" {
		t.Fatalf("delete failure changed state: hasLog=%v hasTimeline=%v lines=%v", hasLog, hasTimeline, lines)
	}
}

func TestLogfilesUpload_CapsAtFourMiBWithMarker(t *testing.T) {
	s := newTimelineTestServer()
	planID := uuid.New().String()
	logID := createLogFile(t, s, planID)

	marker := "\n[bleephub] log truncated at 4 MiB\n"
	block := bytes.Repeat([]byte{'a'}, 3<<20)
	uploadLogBlock(t, s, planID, logID, block)
	uploadLogBlock(t, s, planID, logID, bytes.Repeat([]byte{'b'}, 2<<20))

	s.store.mu.RLock()
	stored := append([]byte(nil), s.store.LogFiles[logID]...)
	s.store.mu.RUnlock()

	if want := (4 << 20) + len(marker); len(stored) != want {
		t.Fatalf("stored len = %d, want %d (4 MiB + marker)", len(stored), want)
	}
	if !bytes.HasSuffix(stored, []byte(marker)) {
		t.Fatal("truncated log must end with the truncation marker")
	}
	if n := bytes.Count(stored, []byte(marker)); n != 1 {
		t.Fatalf("marker appears %d times, want exactly 1", n)
	}
	// The head is preserved: first 3 MiB are the 'a' block, the next
	// 1 MiB the head of the 'b' block.
	if stored[0] != 'a' || stored[(3<<20)-1] != 'a' || stored[3<<20] != 'b' || stored[(4<<20)-1] != 'b' {
		t.Error("truncation must keep the first 4 MiB of uploaded content")
	}

	// Further uploads past the cap are dropped — the marker stays last
	// and the size doesn't grow.
	uploadLogBlock(t, s, planID, logID, []byte("more"))
	s.store.mu.RLock()
	after := len(s.store.LogFiles[logID])
	s.store.mu.RUnlock()
	if after != len(stored) {
		t.Errorf("log grew past the cap: %d → %d", len(stored), after)
	}
}

func TestWebConsoleLog_CapsWithMarkerLine(t *testing.T) {
	s := newTimelineTestServer()
	_, wfJob := seedRun(t, s, "octo/repo", "running", "")
	s.store.mu.Lock()
	s.store.LogLines[wfJob.JobID] = nil
	s.store.mu.Unlock()
	planID, timelineID := linkJobToPlan(t, s, wfJob)

	chunk := make([]string, 6000)
	for i := range chunk {
		chunk[i] = fmt.Sprintf("line %d", i)
	}
	postConsoleLines(t, s, planID, timelineID, chunk)
	postConsoleLines(t, s, planID, timelineID, chunk)

	marker := "[bleephub] console log truncated at 10000 lines"
	s.store.mu.RLock()
	lines := append([]string(nil), s.store.LogLines[wfJob.JobID]...)
	s.store.mu.RUnlock()
	if len(lines) != 10001 {
		t.Fatalf("captured lines = %d, want 10000 + marker", len(lines))
	}
	if lines[len(lines)-1] != marker {
		t.Fatalf("last line = %q, want the truncation marker", lines[len(lines)-1])
	}
	count := 0
	for _, l := range lines {
		if l == marker {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("marker appears %d times, want exactly 1", count)
	}

	// Another chunk past the cap doesn't grow the capture.
	postConsoleLines(t, s, planID, timelineID, chunk)
	s.store.mu.RLock()
	after := len(s.store.LogLines[wfJob.JobID])
	s.store.mu.RUnlock()
	if after != 10001 {
		t.Errorf("capture grew past the cap: %d", after)
	}
}

func TestWebConsoleLog_BareArrayBodyAccepted(t *testing.T) {
	s := newTimelineTestServer()
	_, wfJob := seedRun(t, s, "octo/repo", "running", "")
	s.store.mu.Lock()
	s.store.LogLines[wfJob.JobID] = nil
	s.store.mu.Unlock()
	planID, timelineID := linkJobToPlan(t, s, wfJob)

	body, _ := json.Marshal([]string{"bare line"})
	req := httptest.NewRequest("POST",
		fmt.Sprintf("/_apis/v1/TimeLineWebConsoleLog/%s/build/%s/%s/%s",
			uuid.New().String(), planID, timelineID, uuid.New().String()),
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	s.store.mu.RLock()
	lines := append([]string(nil), s.store.LogLines[wfJob.JobID]...)
	s.store.mu.RUnlock()
	if len(lines) != 1 || lines[0] != "bare line" {
		t.Errorf("captured lines = %v, want the bare-array line", lines)
	}
}

func TestJobLogs_PrefersUploadedLogFiles(t *testing.T) {
	s := newTimelineTestServer()
	_, wfJob := seedRun(t, s, "octo/repo", "completed", "success")
	planID, timelineID := linkJobToPlan(t, s, wfJob)

	// Console capture exists (seedRun installs it), and the runner also
	// uploaded full per-step logs — the uploaded logs win, concatenated
	// in step Order.
	log1 := createLogFile(t, s, planID)
	log2 := createLogFile(t, s, planID)
	uploadLogBlock(t, s, planID, log1, []byte("step one full output\n"))
	uploadLogBlock(t, s, planID, log2, []byte("step two full output"))
	patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": uuid.New().String(), "type": "Task", "name": "two", "order": 2,
			"state": "completed", "result": "succeeded", "log": map[string]any{"id": log2}},
		{"id": uuid.New().String(), "type": "Task", "name": "one", "order": 1,
			"state": "completed", "result": "succeeded", "log": map[string]any{"id": log1}},
	})

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/jobs/%d/logs", stableJobID(wfJob.JobID)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	body, _ := io.ReadAll(w.Body)
	want := "step one full output\nstep two full output\n"
	if string(body) != want {
		t.Errorf("logs body = %q, want %q (uploaded logs in step order)", body, want)
	}
}

func TestJobLogs_NoUploadedLogFilesFailsLoud(t *testing.T) {
	s := newTimelineTestServer()
	_, wfJob := seedRun(t, s, "octo/repo", "completed", "success")
	planID, timelineID := linkJobToPlan(t, s, wfJob)

	// Records exist but reference no uploaded logs. The public download
	// endpoint must not substitute live console capture for durable log
	// artifacts.
	patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": uuid.New().String(), "type": "Task", "name": "one", "order": 1,
			"state": "completed", "result": "succeeded"},
	})

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/jobs/%d/logs", stableJobID(wfJob.JobID)))
	body, _ := io.ReadAll(w.Body)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, body)
	}
	if strings.Contains(string(body), "line one") || strings.Contains(string(body), "line two") {
		t.Fatalf("logs response leaked console capture: %q", body)
	}
}

func TestRunLogsZip_GitHubArchiveLayout(t *testing.T) {
	s := newTimelineTestServer()
	s.registerGHActionsExtrasRoutes()
	wf, wfJob := seedRun(t, s, "octo/repo", "completed", "success")
	planID, timelineID := linkJobToPlan(t, s, wfJob)

	logID := createLogFile(t, s, planID)
	uploadLogBlock(t, s, planID, logID, []byte("checkout output\n"))
	patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": uuid.New().String(), "type": "Task", "name": "actions/checkout@v4", "order": 1,
			"state": "completed", "result": "succeeded", "log": map[string]any{"id": logID}},
		{"id": uuid.New().String(), "type": "Task", "name": "no log yet", "order": 2,
			"state": "inProgress"},
	})

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d/logs", wf.RunID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	entries := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		content, _ := io.ReadAll(rc)
		rc.Close()
		entries[f.Name] = string(content)
	}

	// Job display name is "Build" (seedRun); the step name's '/' is
	// sanitized so it stays a single path segment.
	if got := entries["0_Build.txt"]; got != "checkout output\n" {
		t.Errorf("0_Build.txt = %q, want the full job log; entries: %v", got, entryNames(zr))
	}
	if got := entries["Build/1_actions_checkout@v4.txt"]; got != "checkout output\n" {
		t.Errorf("step log entry = %q; entries: %v", got, entryNames(zr))
	}
	// The in-progress step has no uploaded log → no entry for it.
	for name := range entries {
		if strings.Contains(name, "no log yet") {
			t.Errorf("unexpected entry for logless step: %s", name)
		}
	}
	if len(entries) != 2 {
		t.Errorf("zip entries = %v, want exactly the job + step files", entryNames(zr))
	}
}

func TestRunLogsZip_NoUploadedLogFilesFailsLoud(t *testing.T) {
	s := newTimelineTestServer()
	s.registerGHActionsExtrasRoutes()
	wf, wfJob := seedRun(t, s, "octo/repo", "completed", "success")
	linkJobToPlan(t, s, wfJob)

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d/logs", wf.RunID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), wfJob.JobID) || strings.Contains(w.Body.String(), "line one") {
		t.Fatalf("run logs response leaked console capture/job identity: %q", w.Body.String())
	}
}

func entryNames(zr *zip.Reader) []string {
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	return names
}
