package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	sim "github.com/sockerless/simulator"
)

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
// updateMask. With no mask the document is replaced wholesale (Set without
// merge); with a mask only the listed top-level field paths are written
// (present in the body → set, absent → delete) and every other existing field
// is preserved — which is what DocumentRef.Update(...) and Set(..., MergeAll)
// rely on. Without this, masked writes silently drop all unmentioned fields.
func fsApplyUpdateMask(existing, incoming map[string]FSValue, mask []string) map[string]FSValue {
	if len(mask) == 0 {
		return incoming
	}
	result := make(map[string]FSValue, len(existing)+len(incoming))
	for k, v := range existing {
		result[k] = v
	}
	for _, path := range mask {
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
	current, ok := fsDocuments.Get(name)
	if !ok {
		current = FSDocument{Name: name}
	}
	var req FSDocument
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid document body: %v", err)
		return
	}
	current.Fields = fsApplyUpdateMask(current.Fields, req.Fields, r.URL.Query()["updateMask.fieldPaths"])
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

func handleFSCommit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Writes []struct {
			Update     *FSDocument `json:"update,omitempty"`
			UpdateMask *struct {
				FieldPaths []string `json:"fieldPaths"`
			} `json:"updateMask,omitempty"`
			Delete string `json:"delete,omitempty"`
		} `json:"writes"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid commit body: %v", err)
		return
	}
	writeResults := make([]map[string]any, 0, len(req.Writes))
	for _, wr := range req.Writes {
		if wr.Update != nil {
			doc := *wr.Update
			if doc.Name == "" {
				sim.GCPError(w, http.StatusBadRequest, "write.update.name is required", "INVALID_ARGUMENT")
				return
			}
			// Honor the per-write updateMask so masked writes (Update /
			// Set-MergeAll) preserve unmentioned fields instead of replacing
			// the whole document.
			existing, _ := fsDocuments.Get(doc.Name)
			var mask []string
			if wr.UpdateMask != nil {
				mask = wr.UpdateMask.FieldPaths
			}
			merged := FSDocument{Name: doc.Name, CreateTime: existing.CreateTime}
			merged.Fields = fsApplyUpdateMask(existing.Fields, doc.Fields, mask)
			merged = fsPutDocument(merged)
			writeResults = append(writeResults, map[string]any{"updateTime": merged.UpdateTime})
		}
		if wr.Delete != "" {
			fsDocuments.Delete(wr.Delete)
			writeResults = append(writeResults, map[string]any{"updateTime": fsNow()})
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"writeResults": writeResults, "commitTime": fsNow()})
}

func handleFSBatchWrite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Writes []struct {
			Update *FSDocument `json:"update,omitempty"`
			Delete string      `json:"delete,omitempty"`
		} `json:"writes"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid batchWrite body: %v", err)
		return
	}
	status := make([]map[string]any, 0, len(req.Writes))
	results := make([]map[string]any, 0, len(req.Writes))
	for _, wr := range req.Writes {
		if wr.Update != nil {
			doc := fsPutDocument(*wr.Update)
			results = append(results, map[string]any{"updateTime": doc.UpdateTime})
		} else if wr.Delete != "" {
			fsDocuments.Delete(wr.Delete)
			results = append(results, map[string]any{"updateTime": fsNow()})
		}
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
			if strings.EqualFold(ob.Direction, "DESCENDING") {
				return cmp > 0
			}
			return cmp < 0
		}
		return docs[i].Name < docs[j].Name
	})

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
		out = append(out, map[string]any{"document": d, "readTime": fsNow()})
	}
	if len(out) == 0 {
		out = append(out, map[string]any{"readTime": fsNow(), "done": true})
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

type fsStructuredQuery struct {
	From []struct {
		CollectionID string `json:"collectionId"`
	} `json:"from"`
	Where   *fsFilter `json:"where,omitempty"`
	OrderBy []struct {
		Field struct {
			FieldPath string `json:"fieldPath"`
		} `json:"field"`
		Direction string `json:"direction"`
	} `json:"orderBy,omitempty"`
	Limit  *int `json:"limit,omitempty"`
	Offset int  `json:"offset,omitempty"`
}

type fsFilter struct {
	CompositeFilter *struct {
		Op      string     `json:"op"`
		Filters []fsFilter `json:"filters"`
	} `json:"compositeFilter,omitempty"`
	FieldFilter *struct {
		Field struct {
			FieldPath string `json:"fieldPath"`
		} `json:"field"`
		Op    string  `json:"op"`
		Value FSValue `json:"value"`
	} `json:"fieldFilter,omitempty"`
	UnaryFilter *struct {
		Op    string `json:"op"`
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
		isOr := strings.EqualFold(f.CompositeFilter.Op, "OR")
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
		switch strings.ToUpper(f.UnaryFilter.Op) {
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
		switch strings.ToUpper(f.FieldFilter.Op) {
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

func fsValuesEqual(a, b FSValue) bool {
	return fmt.Sprintf("%#v", a) == fmt.Sprintf("%#v", b)
}
