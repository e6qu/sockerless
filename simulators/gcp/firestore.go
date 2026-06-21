package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	sim "github.com/sockerless/simulator"
)

// fsEnum is a Firestore enum field value: the canonical uppercase name the
// handlers compare against. The gax REST transport the high-level client uses
// marshals enums with UseEnumNumbers (so it sends numbers); the low-level REST
// client sends the string name. Real Firestore accepts both, so the sim does
// too — fsDecodeEnum normalizes either form to the canonical name.
type fsEnum string

// fsDecodeEnum normalizes a Firestore enum JSON token (string name or numeric
// proto value) to its canonical uppercase name using the given number→name map.
func fsDecodeEnum(b []byte, names map[int]string) (fsEnum, error) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return "", nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return "", err
		}
		return fsEnum(strings.ToUpper(s)), nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return "", err
	}
	return fsEnum(names[n]), nil
}

var (
	fsServerValueNames = map[int]string{1: "REQUEST_TIME"}
	fsDirectionNames   = map[int]string{1: "ASCENDING", 2: "DESCENDING"}
	fsCompositeOpNames = map[int]string{1: "AND", 2: "OR"}
	fsUnaryOpNames     = map[int]string{2: "IS_NAN", 3: "IS_NULL", 4: "IS_NOT_NAN", 5: "IS_NOT_NULL"}
	fsFieldOpNames     = map[int]string{
		1: "LESS_THAN", 2: "LESS_THAN_OR_EQUAL", 3: "GREATER_THAN", 4: "GREATER_THAN_OR_EQUAL",
		5: "EQUAL", 6: "NOT_EQUAL", 7: "ARRAY_CONTAINS", 8: "IN", 9: "ARRAY_CONTAINS_ANY", 10: "NOT_IN",
	}
)

// Named enum wrappers so json.Unmarshal dispatches to the right number→name map.
type (
	fsServerValueEnum fsEnum
	fsDirectionEnum   fsEnum
	fsCompositeOpEnum fsEnum
	fsUnaryOpEnum     fsEnum
	fsFieldOpEnum     fsEnum
)

func (e *fsServerValueEnum) UnmarshalJSON(b []byte) error {
	v, err := fsDecodeEnum(b, fsServerValueNames)
	*e = fsServerValueEnum(v)
	return err
}
func (e *fsDirectionEnum) UnmarshalJSON(b []byte) error {
	v, err := fsDecodeEnum(b, fsDirectionNames)
	*e = fsDirectionEnum(v)
	return err
}
func (e *fsCompositeOpEnum) UnmarshalJSON(b []byte) error {
	v, err := fsDecodeEnum(b, fsCompositeOpNames)
	*e = fsCompositeOpEnum(v)
	return err
}
func (e *fsUnaryOpEnum) UnmarshalJSON(b []byte) error {
	v, err := fsDecodeEnum(b, fsUnaryOpNames)
	*e = fsUnaryOpEnum(v)
	return err
}
func (e *fsFieldOpEnum) UnmarshalJSON(b []byte) error {
	v, err := fsDecodeEnum(b, fsFieldOpNames)
	*e = fsFieldOpEnum(v)
	return err
}

// Firestore v1 REST document surface. This slice persists documents in
// Firestore's typed-value JSON shape and implements document CRUD,
// commit/batchGet/batchWrite, and structured equality queries.

type FSDocument struct {
	Name       string             `json:"name"`
	Fields     map[string]FSValue `json:"fields,omitempty"`
	CreateTime string             `json:"createTime,omitempty"`
	UpdateTime string             `json:"updateTime,omitempty"`
}

type FSValue struct {
	NullValue      any           `json:"nullValue,omitempty"`
	BooleanValue   *bool         `json:"booleanValue,omitempty"`
	IntegerValue   string        `json:"integerValue,omitempty"`
	DoubleValue    *float64      `json:"doubleValue,omitempty"`
	TimestampValue string        `json:"timestampValue,omitempty"`
	StringValue    string        `json:"stringValue,omitempty"`
	BytesValue     string        `json:"bytesValue,omitempty"`
	ReferenceValue string        `json:"referenceValue,omitempty"`
	ArrayValue     *FSArrayValue `json:"arrayValue,omitempty"`
	MapValue       *FSMapValue   `json:"mapValue,omitempty"`
}

type FSArrayValue struct {
	Values []FSValue `json:"values,omitempty"`
}

type FSMapValue struct {
	Fields map[string]FSValue `json:"fields,omitempty"`
}

// fsFieldTransform mirrors Firestore's FieldTransform: exactly one of the
// transform operators applies to fieldPath.
type fsFieldTransform struct {
	FieldPath             string            `json:"fieldPath"`
	SetToServerValue      fsServerValueEnum `json:"setToServerValue,omitempty"`
	Increment             *FSValue          `json:"increment,omitempty"`
	Maximum               *FSValue          `json:"maximum,omitempty"`
	Minimum               *FSValue          `json:"minimum,omitempty"`
	AppendMissingElements *FSArrayValue     `json:"appendMissingElements,omitempty"`
	RemoveAllFromArray    *FSArrayValue     `json:"removeAllFromArray,omitempty"`
}

// fsDocumentTransform mirrors Firestore's DocumentTransform (the Write.transform
// field): a target document plus an ordered list of field transforms.
type fsDocumentTransform struct {
	Document        string             `json:"document,omitempty"`
	FieldTransforms []fsFieldTransform `json:"fieldTransforms,omitempty"`
}

// fsPrecondition mirrors Firestore's Precondition (Write.currentDocument):
// either exists (presence assertion) or updateTime (optimistic-concurrency
// assertion). Exactly one is set when present.
type fsPrecondition struct {
	Exists     *bool  `json:"exists,omitempty"`
	UpdateTime string `json:"updateTime,omitempty"`
}

// fsWrite mirrors Firestore's Write: an update/delete plus optional updateMask,
// a standalone or trailing transform (DocumentTransform), inline
// updateTransforms applied after the update, and a currentDocument precondition.
type fsWrite struct {
	Update     *FSDocument `json:"update,omitempty"`
	UpdateMask *struct {
		FieldPaths []string `json:"fieldPaths"`
	} `json:"updateMask,omitempty"`
	Delete           string               `json:"delete,omitempty"`
	Transform        *fsDocumentTransform `json:"transform,omitempty"`
	UpdateTransforms []fsFieldTransform   `json:"updateTransforms,omitempty"`
	CurrentDocument  *fsPrecondition      `json:"currentDocument,omitempty"`
}

var fsDocuments sim.Store[FSDocument]

func registerFirestore(srv *sim.Server) {
	fsDocuments = sim.MakeStore[FSDocument](srv.DB(), "firestore_documents")

	srv.HandleFunc("POST /v1/projects/{project}/databases/{database}/documents:commit", handleFSCommit)
	srv.HandleFunc("POST /v1/projects/{project}/databases/{database}/documents:batchGet", handleFSBatchGet)
	srv.HandleFunc("POST /v1/projects/{project}/databases/{database}/documents:batchWrite", handleFSBatchWrite)
	srv.HandleFunc("POST /v1/projects/{project}/databases/{database}/documents:runQuery", handleFSRunRootQuery)
	srv.HandleFunc("POST /v1/projects/{project}/databases/{database}/documents/{postPath...}", handleFSPostDocuments)
	srv.HandleFunc("GET /v1/projects/{project}/databases/{database}/documents/{docPath...}", handleFSGetOrList)
	srv.HandleFunc("PATCH /v1/projects/{project}/databases/{database}/documents/{docPath...}", handleFSPatchDocument)
	srv.HandleFunc("DELETE /v1/projects/{project}/databases/{database}/documents/{docPath...}", handleFSDeleteDocument)
}

func fsDatabasePrefix(project, database string) string {
	return "projects/" + project + "/databases/" + database + "/documents"
}

func fsNow() string {
	return nowTimestamp()
}

func fsFullName(project, database, docPath string) string {
	docPath = strings.Trim(docPath, "/")
	if docPath == "" {
		return fsDatabasePrefix(project, database)
	}
	return fsDatabasePrefix(project, database) + "/" + docPath
}

func fsCollectionParent(name string) string {
	idx := strings.LastIndex(name, "/")
	if idx < 0 {
		return ""
	}
	return name[:idx]
}

func fsPutDocument(doc FSDocument) FSDocument {
	now := fsNow()
	if doc.CreateTime == "" {
		doc.CreateTime = now
	}
	doc.UpdateTime = now
	if doc.Fields == nil {
		doc.Fields = map[string]FSValue{}
	}
	fsDocuments.Put(doc.Name, doc)
	return doc
}

func handleFSPostDocuments(w http.ResponseWriter, r *http.Request) {
	path := sim.PathParam(r, "postPath")
	if strings.HasSuffix(path, ":runQuery") {
		handleFSRunQuery(w, r, strings.TrimSuffix(path, ":runQuery"))
		return
	}
	project, database := sim.PathParam(r, "project"), sim.PathParam(r, "database")
	docID := r.URL.Query().Get("documentId")
	if docID == "" {
		docID = generateUUID()
	}
	var req FSDocument
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid document body: %v", err)
		return
	}
	name := fsFullName(project, database, strings.Trim(path, "/")+"/"+docID)
	if _, ok := fsDocuments.Get(name); ok {
		sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "Document already exists: %s", name)
		return
	}
	req.Name = name
	sim.WriteJSON(w, http.StatusOK, fsPutDocument(req))
}

func handleFSGetOrList(w http.ResponseWriter, r *http.Request) {
	project, database, docPath := sim.PathParam(r, "project"), sim.PathParam(r, "database"), sim.PathParam(r, "docPath")
	name := fsFullName(project, database, docPath)
	if strings.Count(strings.Trim(docPath, "/"), "/")%2 == 0 {
		handleFSListDocuments(w, r, name)
		return
	}
	doc, ok := fsDocuments.Get(name)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Document not found: %s", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, doc)
}

func handleFSListDocuments(w http.ResponseWriter, r *http.Request, collection string) {
	prefix := strings.TrimSuffix(collection, "/") + "/"
	docs := fsDocuments.Filter(func(d FSDocument) bool {
		if !strings.HasPrefix(d.Name, prefix) {
			return false
		}
		rest := strings.TrimPrefix(d.Name, prefix)
		return rest != "" && !strings.Contains(rest, "/")
	})
	sort.Slice(docs, func(i, j int) bool { return docs[i].Name < docs[j].Name })
	page, next, ok := paginateList(w, r, docs)
	if !ok {
		return
	}
	resp := map[string]any{"documents": page}
	if next != "" {
		resp["nextPageToken"] = next
	}
	sim.WriteJSON(w, http.StatusOK, resp)
}

// fsApplyUpdateMask merges incoming fields into existing per the Firestore
// updateMask. An ABSENT mask (mask == nil) replaces the document wholesale (Set
// without merge). A PRESENT mask (mask != nil, even with zero paths) writes only
// the listed top-level field paths (present in the body → set, absent → delete)
// and preserves every other existing field — which is what DocumentRef.Update,
// Set(..., MergeAll), and a transform-only Update (empty mask + updateTransforms)
// rely on. Absent vs present-but-empty is load-bearing: the SDK's transform-only
// Update sends an empty fields doc with a present empty mask, which must preserve
// the existing fields so the transform reads them — collapsing the two wipes the
// doc.
func fsApplyUpdateMask(existing, incoming map[string]FSValue, mask *[]string) map[string]FSValue {
	if mask == nil {
		return incoming
	}
	result := make(map[string]FSValue, len(existing)+len(incoming))
	for k, v := range existing {
		result[k] = v
	}
	for _, path := range *mask {
		if v, ok := incoming[path]; ok {
			result[path] = v
		} else {
			delete(result, path)
		}
	}
	return result
}

func handleFSPatchDocument(w http.ResponseWriter, r *http.Request) {
	project, database, docPath := sim.PathParam(r, "project"), sim.PathParam(r, "database"), sim.PathParam(r, "docPath")
	name := fsFullName(project, database, docPath)
	// The REST patch endpoint carries the currentDocument precondition as query
	// params (currentDocument.exists / currentDocument.updateTime).
	if pre := fsPreconditionFromQuery(r); pre != nil {
		if e := fsEvalPrecondition(name, pre); e != nil {
			sim.GCPError(w, e.httpStatus, e.message, e.status)
			return
		}
	}
	current, ok := fsDocuments.Get(name)
	if !ok {
		current = FSDocument{Name: name}
	}
	var req FSDocument
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid document body: %v", err)
		return
	}
	// An absent updateMask.fieldPaths query param replaces wholesale; a present
	// one merges the listed paths.
	var mask *[]string
	if paths, ok := r.URL.Query()["updateMask.fieldPaths"]; ok {
		mask = &paths
	}
	current.Fields = fsApplyUpdateMask(current.Fields, req.Fields, mask)
	sim.WriteJSON(w, http.StatusOK, fsPutDocument(current))
}

func handleFSDeleteDocument(w http.ResponseWriter, r *http.Request) {
	name := fsFullName(sim.PathParam(r, "project"), sim.PathParam(r, "database"), sim.PathParam(r, "docPath"))
	if !fsDocuments.Delete(name) {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Document not found: %s", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

// fsWriteError carries a gRPC-mapped error for a single write: the HTTP status,
// the canonical status string, the numeric gRPC code (for batchWrite's per-write
// google.rpc.Status), and the message.
type fsWriteError struct {
	httpStatus int
	status     string
	grpcCode   int
	message    string
}

// gRPC canonical codes used by the Firestore preconditions.
const (
	fsGRPCNotFound           = 5
	fsGRPCAlreadyExists      = 6
	fsGRPCFailedPrecondition = 9
)

// fsPreconditionFromQuery extracts a Precondition from the currentDocument.*
// query params the REST patch endpoint carries, or nil when none are present.
func fsPreconditionFromQuery(r *http.Request) *fsPrecondition {
	q := r.URL.Query()
	existsRaw := q.Get("currentDocument.exists")
	updateTime := q.Get("currentDocument.updateTime")
	if existsRaw == "" && updateTime == "" {
		return nil
	}
	pre := &fsPrecondition{UpdateTime: updateTime}
	if existsRaw != "" {
		if b, err := strconv.ParseBool(existsRaw); err == nil {
			pre.Exists = &b
		}
	}
	return pre
}

// fsEvalPrecondition evaluates a Write.currentDocument precondition against the
// stored document. A nil precondition (or a satisfied one) returns nil; a
// mismatch returns the gRPC-mapped error. Firestore REST maps ALREADY_EXISTS →
// 409, NOT_FOUND → 404, FAILED_PRECONDITION → 400.
func fsEvalPrecondition(name string, pre *fsPrecondition) *fsWriteError {
	if pre == nil {
		return nil
	}
	existing, ok := fsDocuments.Get(name)
	if pre.Exists != nil {
		if *pre.Exists && !ok {
			return &fsWriteError{http.StatusNotFound, "NOT_FOUND", fsGRPCNotFound,
				fmt.Sprintf("No document to update: %s", name)}
		}
		if !*pre.Exists && ok {
			return &fsWriteError{http.StatusConflict, "ALREADY_EXISTS", fsGRPCAlreadyExists,
				fmt.Sprintf("Document already exists: %s", name)}
		}
	}
	if pre.UpdateTime != "" {
		if !ok || existing.UpdateTime != pre.UpdateTime {
			return &fsWriteError{http.StatusBadRequest, "FAILED_PRECONDITION", fsGRPCFailedPrecondition,
				"the stored version does not match the required base version"}
		}
	}
	return nil
}

// fsApplyTransforms applies each FieldTransform against the (already
// update-applied) field map in declaration order, mutating fields in place and
// returning the resulting Value for each transform (the transformResults the
// write result carries, one per transform, in order). now is the commit time
// used for setToServerValue: REQUEST_TIME.
func fsApplyTransforms(fields map[string]FSValue, transforms []fsFieldTransform, now string) []FSValue {
	results := make([]FSValue, 0, len(transforms))
	for _, t := range transforms {
		var result FSValue
		switch {
		case fsEnum(t.SetToServerValue) == "REQUEST_TIME":
			result = FSValue{TimestampValue: now}
		case t.Increment != nil:
			result = fsIncrement(fields[t.FieldPath], *t.Increment)
		case t.Maximum != nil:
			result = fsMaxMin(fields[t.FieldPath], *t.Maximum, true)
		case t.Minimum != nil:
			result = fsMaxMin(fields[t.FieldPath], *t.Minimum, false)
		case t.AppendMissingElements != nil:
			result = fsArrayUnion(fields[t.FieldPath], t.AppendMissingElements)
		case t.RemoveAllFromArray != nil:
			result = fsArrayRemove(fields[t.FieldPath], t.RemoveAllFromArray)
		default:
			// Unknown/unset transform — record the field's current value.
			result = fields[t.FieldPath]
		}
		fields[t.FieldPath] = result
		results = append(results, result)
	}
	return results
}

// fsIncrement adds operand to current. If either operand is a doubleValue the
// result is a doubleValue; otherwise both are integers and the result is an
// integerValue. A missing/non-numeric current is treated as 0.
func fsIncrement(current, operand FSValue) FSValue {
	cur, _ := fsNumeric(current)
	op, _ := fsNumeric(operand)
	sum := cur + op
	if current.DoubleValue != nil || operand.DoubleValue != nil {
		v := sum
		return FSValue{DoubleValue: &v}
	}
	return FSValue{IntegerValue: strconv.FormatInt(int64(sum), 10)}
}

// fsMaxMin returns the per-type max (wantMax) or min of current and operand.
// A missing/non-numeric current yields operand. The result preserves the
// chosen operand's exact representation (integerValue vs doubleValue).
func fsMaxMin(current, operand FSValue, wantMax bool) FSValue {
	_, curNum := fsNumeric(current)
	if !curNum {
		return operand
	}
	cmp := fsCompareValues(current, operand)
	if (wantMax && cmp >= 0) || (!wantMax && cmp <= 0) {
		return current
	}
	return operand
}

// fsArrayUnion appends each element of add not already present in current's
// arrayValue (by fsValuesEqual), returning the resulting arrayValue.
func fsArrayUnion(current FSValue, add *FSArrayValue) FSValue {
	out := &FSArrayValue{}
	if current.ArrayValue != nil {
		out.Values = append(out.Values, current.ArrayValue.Values...)
	}
	for _, e := range add.Values {
		present := false
		for _, x := range out.Values {
			if fsValuesEqual(x, e) {
				present = true
				break
			}
		}
		if !present {
			out.Values = append(out.Values, e)
		}
	}
	return FSValue{ArrayValue: out}
}

// fsArrayRemove drops every element of current's arrayValue that equals any
// element of remove (by fsValuesEqual), returning the resulting arrayValue.
func fsArrayRemove(current FSValue, remove *FSArrayValue) FSValue {
	out := &FSArrayValue{Values: []FSValue{}}
	if current.ArrayValue == nil {
		return FSValue{ArrayValue: out}
	}
	for _, x := range current.ArrayValue.Values {
		drop := false
		for _, e := range remove.Values {
			if fsValuesEqual(x, e) {
				drop = true
				break
			}
		}
		if !drop {
			out.Values = append(out.Values, x)
		}
	}
	return FSValue{ArrayValue: out}
}

// fsApplyWrite executes a single Write against the store: enforces the
// precondition, applies the update (honoring updateMask), applies the inline
// updateTransforms and the standalone/trailing DocumentTransform, and persists.
// It returns the per-write result map (updateTime plus transformResults when any
// transform ran), or the gRPC-mapped error on a precondition/validation
// failure. A standalone transform (no Update) still creates/updates the targeted
// doc.
func fsApplyWrite(wr fsWrite) (map[string]any, *fsWriteError) {
	if wr.Delete != "" {
		if e := fsEvalPrecondition(wr.Delete, wr.CurrentDocument); e != nil {
			return nil, e
		}
		fsDocuments.Delete(wr.Delete)
		return map[string]any{"updateTime": fsNow()}, nil
	}

	name := ""
	if wr.Update != nil {
		name = wr.Update.Name
	} else if wr.Transform != nil {
		name = wr.Transform.Document
	}
	if name == "" {
		return nil, &fsWriteError{http.StatusBadRequest, "INVALID_ARGUMENT", 3,
			"write.update.name or write.transform.document is required"}
	}
	if e := fsEvalPrecondition(name, wr.CurrentDocument); e != nil {
		return nil, e
	}

	existing, _ := fsDocuments.Get(name)
	merged := FSDocument{Name: name, CreateTime: existing.CreateTime}
	if wr.Update != nil {
		// An absent updateMask replaces wholesale; a present one (even empty)
		// merges. A transform-only Update sends an empty present mask that must
		// preserve existing fields so the transform reads them.
		var mask *[]string
		if wr.UpdateMask != nil {
			fp := wr.UpdateMask.FieldPaths
			mask = &fp
		}
		merged.Fields = fsApplyUpdateMask(existing.Fields, wr.Update.Fields, mask)
	} else {
		// Standalone transform: start from the existing field set.
		merged.Fields = map[string]FSValue{}
		for k, v := range existing.Fields {
			merged.Fields[k] = v
		}
	}
	if merged.Fields == nil {
		merged.Fields = map[string]FSValue{}
	}

	now := fsNow()
	var transformResults []FSValue
	transformResults = append(transformResults, fsApplyTransforms(merged.Fields, wr.UpdateTransforms, now)...)
	if wr.Transform != nil {
		transformResults = append(transformResults, fsApplyTransforms(merged.Fields, wr.Transform.FieldTransforms, now)...)
	}

	stored := fsPutDocument(merged)
	result := map[string]any{"updateTime": stored.UpdateTime}
	if len(transformResults) > 0 {
		result["transformResults"] = transformResults
	}
	return result, nil
}

func handleFSCommit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Writes []fsWrite `json:"writes"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid commit body: %v", err)
		return
	}
	writeResults := make([]map[string]any, 0, len(req.Writes))
	for _, wr := range req.Writes {
		res, e := fsApplyWrite(wr)
		if e != nil {
			// commit is atomic: the first failing write aborts the whole commit
			// with the gRPC-mapped HTTP error.
			sim.GCPError(w, e.httpStatus, e.message, e.status)
			return
		}
		writeResults = append(writeResults, res)
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"writeResults": writeResults, "commitTime": fsNow()})
}

func handleFSBatchWrite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Writes []fsWrite `json:"writes"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid batchWrite body: %v", err)
		return
	}
	// batchWrite is non-atomic: each write succeeds or fails independently and
	// the response always carries HTTP 200 with a per-write google.rpc.Status.
	status := make([]map[string]any, 0, len(req.Writes))
	results := make([]map[string]any, 0, len(req.Writes))
	for _, wr := range req.Writes {
		res, e := fsApplyWrite(wr)
		if e != nil {
			results = append(results, map[string]any{})
			status = append(status, map[string]any{"code": e.grpcCode, "message": e.message})
			continue
		}
		results = append(results, res)
		status = append(status, map[string]any{"code": 0})
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"writeResults": results, "status": status})
}

func handleFSBatchGet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Documents []string `json:"documents"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid batchGet body: %v", err)
		return
	}
	out := make([]map[string]any, 0, len(req.Documents))
	readTime := fsNow()
	for _, name := range req.Documents {
		if doc, ok := fsDocuments.Get(name); ok {
			out = append(out, map[string]any{"found": doc, "readTime": readTime})
		} else {
			out = append(out, map[string]any{"missing": name, "readTime": readTime})
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleFSRunRootQuery(w http.ResponseWriter, r *http.Request) {
	handleFSRunQuery(w, r, "")
}

func handleFSRunQuery(w http.ResponseWriter, r *http.Request, parentPath string) {
	project, database := sim.PathParam(r, "project"), sim.PathParam(r, "database")
	parent := fsFullName(project, database, strings.Trim(parentPath, "/"))
	var req struct {
		StructuredQuery fsStructuredQuery `json:"structuredQuery"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid runQuery body: %v", err)
		return
	}
	q := req.StructuredQuery
	if len(q.From) == 0 || q.From[0].CollectionID == "" {
		sim.GCPError(w, http.StatusBadRequest, "structuredQuery.from[0].collectionId is required", "INVALID_ARGUMENT")
		return
	}
	collection := strings.TrimSuffix(parent, "/") + "/" + q.From[0].CollectionID
	docs := fsDocuments.Filter(func(d FSDocument) bool {
		return fsCollectionParent(d.Name) == collection && fsWhereMatches(d, q.Where)
	})

	// Ordering: explicit orderBy fields (with direction) take precedence,
	// otherwise documents sort by name (Firestore's implicit __name__ order).
	sort.SliceStable(docs, func(i, j int) bool {
		for _, ob := range q.OrderBy {
			path := ob.Field.FieldPath
			if path == "" || path == "__name__" {
				continue
			}
			cmp := fsCompareValues(docs[i].Fields[path], docs[j].Fields[path])
			if cmp == 0 {
				continue
			}
			if fsEnum(ob.Direction) == "DESCENDING" {
				return cmp > 0
			}
			return cmp < 0
		}
		return docs[i].Name < docs[j].Name
	})

	// Cursors: applied against the ordered slice, before offset/limit. startAt
	// trims the leading edge, endAt trims the trailing edge, each positioned by
	// comparing the cursor values against the orderBy fields (honoring `before`).
	if q.StartAt != nil {
		docs = docs[fsCursorIndex(docs, q.OrderBy, q.StartAt):]
	}
	if q.EndAt != nil {
		docs = docs[:fsCursorIndex(docs, q.OrderBy, q.EndAt)]
	}

	// Honor offset + limit (the StructuredQuery cursor controls page size).
	if q.Offset > 0 {
		if q.Offset >= len(docs) {
			docs = nil
		} else {
			docs = docs[q.Offset:]
		}
	}
	if q.Limit != nil && *q.Limit >= 0 && *q.Limit < len(docs) {
		docs = docs[:*q.Limit]
	}

	out := make([]map[string]any, 0, len(docs)+1)
	for _, d := range docs {
		out = append(out, map[string]any{"document": fsProjectDocument(d, q.Select), "readTime": fsNow()})
	}
	if len(out) == 0 {
		out = append(out, map[string]any{"readTime": fsNow(), "done": true})
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

// fsCompareToCursor compares a document's orderBy key against the cursor values,
// returning -1/0/1. Only as many orderBy positions as the cursor supplies are
// compared (a partial cursor is a prefix). The `__name__` order field compares
// by document name.
func fsCompareToCursor(d FSDocument, orderBy []fsOrderBy, cur *fsCursor) int {
	for i, cv := range cur.Values {
		if i >= len(orderBy) {
			break
		}
		ob := orderBy[i]
		var cmp int
		if ob.Field.FieldPath == "__name__" {
			cmp = strings.Compare(d.Name, cv.ReferenceValue)
		} else {
			cmp = fsCompareValues(d.Fields[ob.Field.FieldPath], cv)
		}
		if fsEnum(ob.Direction) == "DESCENDING" {
			cmp = -cmp
		}
		if cmp != 0 {
			return cmp
		}
	}
	return 0
}

// fsCursorIndex finds the boundary index in the ordered docs for a cursor. The
// same comparison serves both bounds: as a startAt it is the inclusive lower
// bound, and as an endAt it is the exclusive upper bound. before=true treats a
// document equal to the cursor as on/after the boundary (StartAt / EndBefore);
// before=false places it before the boundary (StartAfter / EndAt). docs are
// already ordered.
func fsCursorIndex(docs []FSDocument, orderBy []fsOrderBy, cur *fsCursor) int {
	for i := range docs {
		cmp := fsCompareToCursor(docs[i], orderBy, cur)
		if (cur.Before && cmp >= 0) || (!cur.Before && cmp > 0) {
			return i
		}
	}
	return len(docs)
}

// fsProjectDocument applies a StructuredQuery select projection: the returned
// document carries only the listed field paths (preserving name/timestamps). A
// nil projection returns the document unchanged; a projection of only
// `__name__` yields a keys-only document (empty Fields).
func fsProjectDocument(d FSDocument, sel *fsProjection) FSDocument {
	if sel == nil {
		return d
	}
	projected := FSDocument{
		Name:       d.Name,
		CreateTime: d.CreateTime,
		UpdateTime: d.UpdateTime,
		Fields:     map[string]FSValue{},
	}
	for _, f := range sel.Fields {
		if f.FieldPath == "__name__" {
			continue
		}
		if v, ok := d.Fields[f.FieldPath]; ok {
			projected.Fields[f.FieldPath] = v
		}
	}
	return projected
}

type fsOrderBy struct {
	Field struct {
		FieldPath string `json:"fieldPath"`
	} `json:"field"`
	Direction fsDirectionEnum `json:"direction"`
}

type fsCursor struct {
	Values []FSValue `json:"values,omitempty"`
	Before bool      `json:"before,omitempty"`
}

type fsProjection struct {
	Fields []struct {
		FieldPath string `json:"fieldPath"`
	} `json:"fields,omitempty"`
}

type fsStructuredQuery struct {
	From []struct {
		CollectionID string `json:"collectionId"`
	} `json:"from"`
	Where   *fsFilter     `json:"where,omitempty"`
	OrderBy []fsOrderBy   `json:"orderBy,omitempty"`
	StartAt *fsCursor     `json:"startAt,omitempty"`
	EndAt   *fsCursor     `json:"endAt,omitempty"`
	Select  *fsProjection `json:"select,omitempty"`
	Limit   *int          `json:"limit,omitempty"`
	Offset  int           `json:"offset,omitempty"`
}

type fsFilter struct {
	CompositeFilter *struct {
		Op      fsCompositeOpEnum `json:"op"`
		Filters []fsFilter        `json:"filters"`
	} `json:"compositeFilter,omitempty"`
	FieldFilter *struct {
		Field struct {
			FieldPath string `json:"fieldPath"`
		} `json:"field"`
		Op    fsFieldOpEnum `json:"op"`
		Value FSValue       `json:"value"`
	} `json:"fieldFilter,omitempty"`
	UnaryFilter *struct {
		Op    fsUnaryOpEnum `json:"op"`
		Field struct {
			FieldPath string `json:"fieldPath"`
		} `json:"field"`
	} `json:"unaryFilter,omitempty"`
}

// fsWhereMatches evaluates a Firestore structured-query filter (field, unary, or
// composite AND/OR) against a document. A nil filter matches every document.
func fsWhereMatches(d FSDocument, f *fsFilter) bool {
	if f == nil {
		return true
	}
	switch {
	case f.CompositeFilter != nil:
		isOr := fsEnum(f.CompositeFilter.Op) == "OR"
		for i := range f.CompositeFilter.Filters {
			m := fsWhereMatches(d, &f.CompositeFilter.Filters[i])
			if isOr && m {
				return true
			}
			if !isOr && !m {
				return false
			}
		}
		return !isOr
	case f.UnaryFilter != nil:
		got, ok := d.Fields[f.UnaryFilter.Field.FieldPath]
		switch fsEnum(f.UnaryFilter.Op) {
		case "IS_NULL":
			return ok && got.NullValue != nil
		case "IS_NOT_NULL":
			return ok && got.NullValue == nil
		default:
			return false
		}
	case f.FieldFilter != nil:
		got, ok := d.Fields[f.FieldFilter.Field.FieldPath]
		want := f.FieldFilter.Value
		switch fsEnum(f.FieldFilter.Op) {
		case "EQUAL":
			return ok && fsValuesEqual(got, want)
		case "NOT_EQUAL":
			return ok && !fsValuesEqual(got, want)
		case "LESS_THAN":
			return ok && fsCompareValues(got, want) < 0
		case "LESS_THAN_OR_EQUAL":
			return ok && fsCompareValues(got, want) <= 0
		case "GREATER_THAN":
			return ok && fsCompareValues(got, want) > 0
		case "GREATER_THAN_OR_EQUAL":
			return ok && fsCompareValues(got, want) >= 0
		case "IN":
			return ok && fsValueInArray(got, want)
		case "NOT_IN":
			return ok && !fsValueInArray(got, want)
		case "ARRAY_CONTAINS":
			return ok && fsArrayContains(got, want)
		case "ARRAY_CONTAINS_ANY":
			return ok && fsArrayContainsAny(got, want)
		default:
			return false
		}
	default:
		return true
	}
}

// fsValueInArray reports whether got equals any element of want's arrayValue.
func fsValueInArray(got, want FSValue) bool {
	if want.ArrayValue == nil {
		return false
	}
	for _, v := range want.ArrayValue.Values {
		if fsValuesEqual(got, v) {
			return true
		}
	}
	return false
}

// fsArrayContains reports whether got's arrayValue contains want.
func fsArrayContains(got, want FSValue) bool {
	if got.ArrayValue == nil {
		return false
	}
	for _, v := range got.ArrayValue.Values {
		if fsValuesEqual(v, want) {
			return true
		}
	}
	return false
}

// fsArrayContainsAny reports whether got's arrayValue contains any element of
// want's arrayValue.
func fsArrayContainsAny(got, want FSValue) bool {
	if want.ArrayValue == nil {
		return false
	}
	for _, v := range want.ArrayValue.Values {
		if fsArrayContains(got, v) {
			return true
		}
	}
	return false
}

// fsCompareValues orders two scalar Firestore values of the same type, returning
// -1, 0, or 1. Numeric values (integer/double) compare numerically; strings and
// timestamps compare lexically; unknown/mixed types fall back to string form.
func fsCompareValues(a, b FSValue) int {
	an, aIsNum := fsNumeric(a)
	bn, bIsNum := fsNumeric(b)
	if aIsNum && bIsNum {
		switch {
		case an < bn:
			return -1
		case an > bn:
			return 1
		default:
			return 0
		}
	}
	as, bs := fsScalarString(a), fsScalarString(b)
	return strings.Compare(as, bs)
}

func fsNumeric(v FSValue) (float64, bool) {
	if v.DoubleValue != nil {
		return *v.DoubleValue, true
	}
	if v.IntegerValue != "" {
		if n, err := strconv.ParseFloat(v.IntegerValue, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}

func fsScalarString(v FSValue) string {
	switch {
	case v.StringValue != "":
		return v.StringValue
	case v.TimestampValue != "":
		return v.TimestampValue
	case v.ReferenceValue != "":
		return v.ReferenceValue
	default:
		return fmt.Sprintf("%#v", v)
	}
}

// fsValuesEqual reports whether two Firestore values are equal by Firestore's
// own equality semantics: integerValue and doubleValue unify by numeric value
// (so 1 == 1.0), scalars compare structurally, and arrayValue/mapValue compare
// element-/field-wise recursively. This drives EQUAL/NOT_EQUAL/IN/NOT_IN and
// the ARRAY_CONTAINS family.
func fsValuesEqual(a, b FSValue) bool {
	an, aIsNum := fsNumeric(a)
	bn, bIsNum := fsNumeric(b)
	if aIsNum || bIsNum {
		return aIsNum && bIsNum && an == bn
	}
	switch {
	case a.NullValue != nil || b.NullValue != nil:
		return a.NullValue != nil && b.NullValue != nil
	case a.BooleanValue != nil || b.BooleanValue != nil:
		return a.BooleanValue != nil && b.BooleanValue != nil && *a.BooleanValue == *b.BooleanValue
	case a.StringValue != "" || b.StringValue != "":
		return a.StringValue == b.StringValue
	case a.TimestampValue != "" || b.TimestampValue != "":
		return a.TimestampValue == b.TimestampValue
	case a.BytesValue != "" || b.BytesValue != "":
		return a.BytesValue == b.BytesValue
	case a.ReferenceValue != "" || b.ReferenceValue != "":
		return a.ReferenceValue == b.ReferenceValue
	case a.ArrayValue != nil || b.ArrayValue != nil:
		return fsArraysEqual(a.ArrayValue, b.ArrayValue)
	case a.MapValue != nil || b.MapValue != nil:
		return fsMapsEqual(a.MapValue, b.MapValue)
	default:
		// Both values are empty/untyped — treat as equal.
		return true
	}
}

func fsArraysEqual(a, b *FSArrayValue) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if len(a.Values) != len(b.Values) {
		return false
	}
	for i := range a.Values {
		if !fsValuesEqual(a.Values[i], b.Values[i]) {
			return false
		}
	}
	return true
}

func fsMapsEqual(a, b *FSMapValue) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if len(a.Fields) != len(b.Fields) {
		return false
	}
	for k, av := range a.Fields {
		bv, ok := b.Fields[k]
		if !ok || !fsValuesEqual(av, bv) {
			return false
		}
	}
	return true
}
