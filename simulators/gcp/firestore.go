package main

import (
	"fmt"
	"net/http"
	"sort"
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
	sim.WriteJSON(w, http.StatusOK, map[string]any{"documents": docs})
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
	current.Fields = req.Fields
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
			Update *FSDocument `json:"update,omitempty"`
			Delete string      `json:"delete,omitempty"`
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
			doc = fsPutDocument(doc)
			writeResults = append(writeResults, map[string]any{"updateTime": doc.UpdateTime})
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
		StructuredQuery struct {
			From []struct {
				CollectionID string `json:"collectionId"`
			} `json:"from"`
			Where *struct {
				FieldFilter *struct {
					Field struct {
						FieldPath string `json:"fieldPath"`
					} `json:"field"`
					Op    string  `json:"op"`
					Value FSValue `json:"value"`
				} `json:"fieldFilter,omitempty"`
			} `json:"where,omitempty"`
		} `json:"structuredQuery"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid runQuery body: %v", err)
		return
	}
	if len(req.StructuredQuery.From) == 0 || req.StructuredQuery.From[0].CollectionID == "" {
		sim.GCPError(w, http.StatusBadRequest, "structuredQuery.from[0].collectionId is required", "INVALID_ARGUMENT")
		return
	}
	collection := strings.TrimSuffix(parent, "/") + "/" + req.StructuredQuery.From[0].CollectionID
	docs := fsDocuments.Filter(func(d FSDocument) bool {
		if fsCollectionParent(d.Name) != collection {
			return false
		}
		ff := req.StructuredQuery.Where
		if ff == nil || ff.FieldFilter == nil {
			return true
		}
		got, ok := d.Fields[ff.FieldFilter.Field.FieldPath]
		return ok && fsValuesEqual(got, ff.FieldFilter.Value)
	})
	sort.Slice(docs, func(i, j int) bool { return docs[i].Name < docs[j].Name })
	out := make([]map[string]any, 0, len(docs)+1)
	for _, d := range docs {
		out = append(out, map[string]any{"document": d, "readTime": fsNow()})
	}
	if len(out) == 0 {
		out = append(out, map[string]any{"readTime": fsNow(), "done": true})
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func fsValuesEqual(a, b FSValue) bool {
	return fmt.Sprintf("%#v", a) == fmt.Sprintf("%#v", b)
}
