package bleephub

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Runtime response-shape validation against the vendored GitHub OpenAPI
// description (testdata/github-openapi.json.gz). The TestMain-owned
// shared server routes most of the package's test traffic; an observer
// validates every 2xx /api/v3 JSON response member-by-member against
// the documented response schema. Violations are deduped after m.Run()
// and gated against openapi-violation-allowlist.txt — an entry is
// either a real-but-undescribed member (GHES-only surface, with a
// citation) or a filed BUG on its way to being fixed; the list only
// shrinks. The companion route
// check (gh_api_definition_test.go) keeps paths honest; this keeps the
// bodies honest.

type shapeViolation struct {
	Op    string // "METHOD /spec/path/template -> status"
	Kind  string // unknown-field | type-mismatch | missing-required | malformed-json | internal-url
	Field string
}

func (v shapeViolation) Key() string {
	return v.Op + "\t" + v.Kind + "\t" + v.Field
}

type openAPIOperation struct {
	Method   string
	Template []string // path segments; "{}" = parameter
	Literals int
	// Responses maps status code (or "default") to the JSON schema.
	Responses map[string]map[string]any
}

type shapeValidator struct {
	mu         sync.Mutex
	seen       map[string]shapeViolation
	ops        []openAPIOperation
	schemas    map[string]any // components/schemas
	exchanges  int
	validated  int
	skippedOps int
}

var apiShapeValidator *shapeValidator

var shapeParamSegment = regexp.MustCompile(`^\{[^}]+\}$`)

func newShapeValidator() (*shapeValidator, error) {
	f, err := os.Open("testdata/github-openapi.json.gz")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	var doc struct {
		Paths      map[string]map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.NewDecoder(gz).Decode(&doc); err != nil {
		return nil, err
	}
	if len(doc.Paths) < 500 || len(doc.Components.Schemas) < 100 {
		return nil, fmt.Errorf("vendored OpenAPI looks truncated: %d paths, %d schemas", len(doc.Paths), len(doc.Components.Schemas))
	}

	v := &shapeValidator{seen: map[string]shapeViolation{}, schemas: doc.Components.Schemas}
	for path, methods := range doc.Paths {
		segs := strings.Split(strings.Trim(path, "/"), "/")
		template := make([]string, len(segs))
		literals := 0
		for i, seg := range segs {
			if shapeParamSegment.MatchString(seg) {
				template[i] = "{}"
			} else {
				template[i] = seg
				literals++
			}
		}
		for method, raw := range methods {
			switch method {
			case "get", "post", "put", "patch", "delete", "head":
			default:
				continue
			}
			var op struct {
				Responses map[string]struct {
					Content map[string]struct {
						Schema map[string]any `json:"schema"`
					} `json:"content"`
				} `json:"responses"`
			}
			if err := json.Unmarshal(raw, &op); err != nil {
				continue
			}
			responses := map[string]map[string]any{}
			for status, resp := range op.Responses {
				for ct, content := range resp.Content {
					if strings.Contains(ct, "json") && content.Schema != nil {
						responses[status] = content.Schema
						break
					}
				}
			}
			v.ops = append(v.ops, openAPIOperation{
				Method:    strings.ToUpper(method),
				Template:  template,
				Literals:  literals,
				Responses: responses,
			})
		}
	}
	return v, nil
}

// Observe is the Server response observer.
func (v *shapeValidator) Observe(req *http.Request, status int, header http.Header, body []byte) {
	path, ok := strings.CutPrefix(req.URL.Path, "/api/v3")
	if !ok || path == "" {
		return
	}
	v.mu.Lock()
	v.exchanges++
	v.mu.Unlock()
	if status < 200 || status >= 300 || len(body) == 0 {
		return
	}
	if ct := header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "json") {
		return
	}

	segs := strings.Split(strings.Trim(path, "/"), "/")
	var candidates []openAPIOperation
	for _, op := range v.ops {
		if op.Method != req.Method || len(op.Template) != len(segs) {
			continue
		}
		match := true
		for i, t := range op.Template {
			if t != "{}" && t != segs[i] {
				match = false
				break
			}
		}
		if match {
			candidates = append(candidates, op)
		}
	}
	if len(candidates) == 0 {
		// The route-existence test owns unknown paths.
		v.mu.Lock()
		v.skippedOps++
		v.mu.Unlock()
		return
	}
	// Most-literal template first: a concrete path must not be judged by
	// a generic sibling when a more specific one exists.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Literals > candidates[j].Literals })

	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		op := candidates[0]
		v.record(shapeViolation{Op: opLabel(op, status), Kind: "malformed-json", Field: "$"})
		return
	}
	for _, field := range internalURLFields(decoded, "$") {
		v.record(shapeViolation{Op: opLabel(candidates[0], status), Kind: "internal-url", Field: field})
	}

	var best []shapeViolation
	bestSet := false
	for _, op := range candidates {
		schema, ok := op.Responses[fmt.Sprintf("%d", status)]
		if !ok {
			schema, ok = op.Responses["default"]
		}
		if !ok {
			continue // status not documented with a body for this op
		}
		var out []shapeViolation
		v.walk(schema, decoded, opLabel(op, status), "$", &out, 0)
		if len(out) == 0 {
			return // a documented schema fully accepts the response
		}
		if !bestSet || len(out) < len(best) {
			best, bestSet = out, true
		}
	}
	if !bestSet {
		return // no candidate documents a body for this status
	}
	v.mu.Lock()
	v.validated++
	v.mu.Unlock()
	for _, viol := range best {
		v.record(viol)
	}
}

func internalURLFields(v any, field string) []string {
	switch x := v.(type) {
	case map[string]any:
		var out []string
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, internalURLFields(x[k], field+"."+k)...)
		}
		return out
	case []any:
		var out []string
		for i, item := range x {
			out = append(out, internalURLFields(item, fmt.Sprintf("%s[%d]", field, i))...)
		}
		return out
	case string:
		if strings.Contains(x, "/internal/") {
			return []string{field}
		}
	}
	return nil
}

func opLabel(op openAPIOperation, status int) string {
	return fmt.Sprintf("%s /%s -> %d", op.Method, strings.Join(op.Template, "/"), status)
}

func (v *shapeValidator) record(viol shapeViolation) {
	viol.Field = collapseIndexes(viol.Field)
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.seen[viol.Key()]; !ok {
		v.seen[viol.Key()] = viol
	}
}

var indexSegment = regexp.MustCompile(`\[\d+\]`)

func collapseIndexes(field string) string {
	return indexSegment.ReplaceAllString(field, "[]")
}

// resolve follows $ref chains into components/schemas.
func (v *shapeValidator) resolve(schema map[string]any) map[string]any {
	for i := 0; i < 16; i++ {
		ref, _ := schema["$ref"].(string)
		if ref == "" {
			return schema
		}
		name, ok := strings.CutPrefix(ref, "#/components/schemas/")
		if !ok {
			return schema
		}
		next, _ := v.schemas[name].(map[string]any)
		if next == nil {
			return schema
		}
		schema = next
	}
	return schema
}

// flatten merges a schema's allOf chain into one effective schema view:
// the union of properties + required, and whether additional properties
// are allowed anywhere in the chain.
func (v *shapeValidator) flatten(schema map[string]any) (props map[string]map[string]any, required []string, additional any) {
	props = map[string]map[string]any{}
	var visit func(s map[string]any)
	visit = func(s map[string]any) {
		s = v.resolve(s)
		if ap, ok := s["additionalProperties"]; ok {
			additional = ap
		}
		if p, ok := s["properties"].(map[string]any); ok {
			for name, sub := range p {
				if m, ok := sub.(map[string]any); ok {
					props[name] = m
				}
			}
		}
		if reqs, ok := s["required"].([]any); ok {
			for _, r := range reqs {
				if name, ok := r.(string); ok {
					required = append(required, name)
				}
			}
		}
		if all, ok := s["allOf"].([]any); ok {
			for _, branch := range all {
				if m, ok := branch.(map[string]any); ok {
					visit(m)
				}
			}
		}
	}
	visit(schema)
	return props, required, additional
}

func schemaTypes(schema map[string]any) []string {
	switch t := schema["type"].(type) {
	case string:
		return []string{t}
	case []any:
		var out []string
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// walk validates a decoded JSON value against an OpenAPI schema,
// reporting unknown members, type mismatches, and absent required
// members. Composition: allOf is flattened; anyOf/oneOf pass when any
// branch fully accepts the value.
func (v *shapeValidator) walk(schema map[string]any, val any, op, path string, out *[]shapeViolation, depth int) {
	if depth > 24 || schema == nil {
		return
	}
	schema = v.resolve(schema)

	if val == nil {
		return // null is acceptable wherever a member is emitted at all
	}

	if branches, ok := schema["anyOf"].([]any); ok {
		v.walkBranches(branches, val, op, path, out, depth)
		return
	}
	if branches, ok := schema["oneOf"].([]any); ok {
		v.walkBranches(branches, val, op, path, out, depth)
		return
	}

	types := schemaTypes(schema)
	_, hasProps := schema["properties"]
	_, hasAllOf := schema["allOf"]
	isObjectSchema := len(types) == 0 && (hasProps || hasAllOf)

	switch value := val.(type) {
	case map[string]any:
		if len(types) > 0 && !contains(types, "object") {
			*out = append(*out, shapeViolation{Op: op, Kind: "type-mismatch", Field: path})
			return
		}
		if len(types) == 0 && !isObjectSchema {
			return // untyped open schema
		}
		props, required, additional := v.flatten(schema)
		if len(props) == 0 && additional == nil {
			return // object with no declared members: open
		}
		for _, name := range required {
			if _, ok := value[name]; !ok {
				*out = append(*out, shapeViolation{Op: op, Kind: "missing-required", Field: path + "." + name})
			}
		}
		for name, member := range value {
			sub, known := props[name]
			if known {
				v.walk(sub, member, op, path+"."+name, out, depth+1)
				continue
			}
			switch ap := additional.(type) {
			case bool:
				if !ap {
					*out = append(*out, shapeViolation{Op: op, Kind: "unknown-field", Field: path + "." + name})
				}
			case map[string]any:
				v.walk(ap, member, op, path+"."+name, out, depth+1)
			default:
				// additionalProperties unspecified: the description's
				// schemas enumerate real members exhaustively, so an
				// undeclared member is a bleephub invention.
				*out = append(*out, shapeViolation{Op: op, Kind: "unknown-field", Field: path + "." + name})
			}
		}
	case []any:
		if len(types) > 0 && !contains(types, "array") {
			*out = append(*out, shapeViolation{Op: op, Kind: "type-mismatch", Field: path})
			return
		}
		items, _ := schema["items"].(map[string]any)
		if items == nil {
			return
		}
		for i, item := range value {
			v.walk(items, item, op, fmt.Sprintf("%s[%d]", path, i), out, depth+1)
		}
	case string:
		if len(types) > 0 && !contains(types, "string") {
			*out = append(*out, shapeViolation{Op: op, Kind: "type-mismatch", Field: path})
		}
	case bool:
		if len(types) > 0 && !contains(types, "boolean") {
			*out = append(*out, shapeViolation{Op: op, Kind: "type-mismatch", Field: path})
		}
	case float64:
		if len(types) > 0 && !contains(types, "number") && !contains(types, "integer") {
			*out = append(*out, shapeViolation{Op: op, Kind: "type-mismatch", Field: path})
		}
	}
}

func (v *shapeValidator) walkBranches(branches []any, val any, op, path string, out *[]shapeViolation, depth int) {
	var best []shapeViolation
	bestSet := false
	for _, b := range branches {
		schema, ok := b.(map[string]any)
		if !ok {
			continue
		}
		var attempt []shapeViolation
		v.walk(schema, val, op, path, &attempt, depth+1)
		if len(attempt) == 0 {
			return
		}
		if !bestSet || len(attempt) < len(best) {
			best, bestSet = attempt, true
		}
	}
	*out = append(*out, best...)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ratchet compares the deduped violations against the allowlist and
// returns the new keys. Allowlist format: key lines (op<TAB>kind<TAB>field)
// with #-comments; every entry carries a BUG ID.
func (v *shapeValidator) ratchet() (newKeys []string, total int) {
	allowed := map[string]bool{}
	if data, err := os.ReadFile("openapi-violation-allowlist.txt"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if i := strings.Index(line, "#"); i >= 0 {
				line = line[:i]
			}
			line = strings.TrimRight(line, " \t")
			if strings.TrimSpace(line) == "" {
				continue
			}
			allowed[line] = true
		}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for key := range v.seen {
		if !allowed[key] {
			newKeys = append(newKeys, key)
		}
	}
	sort.Strings(newKeys)
	return newKeys, len(v.seen)
}
