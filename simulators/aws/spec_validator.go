package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Runtime wire-shape validation against the vendored Smithy models
// (specs/cloud-api/aws/). Armed when SOCKERLESS_SPEC_VALIDATE (report
// file) is set; SOCKERLESS_SPEC_DIR must then point at the vendored spec
// directory. Coverage: awsJson1.0 / awsJson1.1 operations (X-Amz-Target
// routing) — success responses are validated member-by-member against
// the operation's output shape: members the spec doesn't define and
// primitive type mismatches are violations. XML protocols (awsQuery /
// restXml) and restJson1 path operations are exercised by the static
// surface gate + SDK suites; their shape validation rides the same
// report mechanism when added.

type smithyShapeDef struct {
	Type    string                     `json:"type"`
	Version string                     `json:"version"`
	Member  *smithyMemberRef           `json:"member"`  // list
	Key     *smithyMemberRef           `json:"key"`     // map
	Value   *smithyMemberRef           `json:"value"`   // map
	Members map[string]smithyMemberRef `json:"members"` // structure/union/enum
	Output  *smithyMemberRef           `json:"output"`  // operation
	Traits  map[string]json.RawMessage `json:"traits"`
}

type smithyMemberRef struct {
	Target string                     `json:"target"`
	Traits map[string]json.RawMessage `json:"traits"`
}

type smithyModelIndex struct {
	shapes map[string]smithyShapeDef
	// opOutput maps short operation name -> output shape ID.
	opOutput map[string]string
	// serviceShort is the service shape's short name (the Go SDK's
	// X-Amz-Target prefix; botocore may prefix it with a java-style
	// namespace).
	serviceShort string
}

func loadSmithyIndex(dir string) (map[string]*smithyModelIndex, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.smithy.json.gz"))
	if err != nil || len(paths) == 0 {
		return nil, fmt.Errorf("no Smithy models under %s (glob err: %v)", dir, err)
	}
	byPrefix := map[string]*smithyModelIndex{}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		gz, err := gzip.NewReader(f)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		var doc struct {
			Shapes map[string]smithyShapeDef `json:"shapes"`
		}
		err = json.NewDecoder(gz).Decode(&doc)
		_ = gz.Close()
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		idx := &smithyModelIndex{shapes: doc.Shapes, opOutput: map[string]string{}}
		for id, shape := range doc.Shapes {
			short := id
			if i := strings.Index(id, "#"); i >= 0 {
				short = id[i+1:]
			}
			switch shape.Type {
			case "service":
				idx.serviceShort = short
			case "operation":
				if shape.Output != nil {
					idx.opOutput[short] = shape.Output.Target
				}
			}
		}
		if idx.serviceShort == "" {
			return nil, fmt.Errorf("%s: no service shape", p)
		}
		byPrefix[idx.serviceShort] = idx
	}
	return byPrefix, nil
}

// armSpecValidator wires runtime shape validation onto the server when
// SOCKERLESS_SPEC_VALIDATE is set. Hard failure when the spec dir is
// missing: the operator asked for validation.
func armSpecValidator(srv *sim.Server) error {
	if os.Getenv("SOCKERLESS_SPEC_VALIDATE") == "" {
		return nil
	}
	dir := os.Getenv("SOCKERLESS_SPEC_DIR")
	if dir == "" {
		return fmt.Errorf("SOCKERLESS_SPEC_VALIDATE is set but SOCKERLESS_SPEC_DIR is not")
	}
	models, err := loadSmithyIndex(dir)
	if err != nil {
		return err
	}
	srv.SetSpecValidator(func(req *http.Request, _ []byte, status int, respHeader http.Header, respBody []byte) []sim.SpecViolation {
		target := req.Header.Get("X-Amz-Target")
		if target == "" || status >= 400 || len(respBody) == 0 {
			return nil // non-awsJson exchange or error/empty response
		}
		ct := respHeader.Get("Content-Type")
		if ct != "" && !strings.Contains(ct, "json") {
			return nil
		}
		i := strings.LastIndex(target, ".")
		if i < 0 {
			return nil
		}
		prefix, op := target[:i], target[i+1:]
		idx, ok := models[prefix]
		if !ok {
			// botocore-style dotted prefix: last component is the shape name.
			if j := strings.LastIndex(prefix, "."); j >= 0 {
				idx, ok = models[prefix[j+1:]]
			}
		}
		if !ok {
			return nil // surface gate owns unknown targets
		}
		outShape, ok := idx.opOutput[op]
		if !ok {
			return nil
		}
		var body any
		if err := json.Unmarshal(respBody, &body); err != nil {
			return []sim.SpecViolation{{Op: target, Kind: "malformed-json", Field: "$", Detail: err.Error()}}
		}
		var out []sim.SpecViolation
		validateSmithyValue(idx, target, outShape, "$", body, &out)
		return out
	})
	return nil
}

// validateSmithyValue walks a decoded JSON value against a Smithy shape,
// reporting members the spec doesn't define and primitive type
// mismatches. Null values are always acceptable (omitted members).
func validateSmithyValue(idx *smithyModelIndex, op, shapeID, path string, v any, out *[]sim.SpecViolation) {
	if v == nil {
		return
	}
	shape, ok := idx.shapes[shapeID]
	if !ok {
		// Prelude primitives (smithy.api#String etc.) are not in the
		// model's shape map.
		validateSmithyPrimitive(op, shapeID, path, v, out)
		return
	}
	switch shape.Type {
	case "structure", "union":
		obj, ok := v.(map[string]any)
		if !ok {
			*out = append(*out, sim.SpecViolation{Op: op, Kind: "type-mismatch", Field: path, Detail: fmt.Sprintf("spec %s is a %s, response has %T", shapeID, shape.Type, v)})
			return
		}
		// JSON member name = member name unless overridden by jsonName.
		byJSONName := make(map[string]smithyMemberRef, len(shape.Members))
		for name, ref := range shape.Members {
			jsonName := name
			if raw, ok := ref.Traits["smithy.api#jsonName"]; ok {
				var s string
				if json.Unmarshal(raw, &s) == nil && s != "" {
					jsonName = s
				}
			}
			byJSONName[jsonName] = ref
		}
		for key, val := range obj {
			ref, ok := byJSONName[key]
			if !ok {
				*out = append(*out, sim.SpecViolation{Op: op, Kind: "unknown-field", Field: path + "." + key, Detail: "member not defined by " + shapeID})
				continue
			}
			validateSmithyValue(idx, op, ref.Target, path+"."+key, val, out)
		}
	case "list", "set":
		arr, ok := v.([]any)
		if !ok {
			*out = append(*out, sim.SpecViolation{Op: op, Kind: "type-mismatch", Field: path, Detail: fmt.Sprintf("spec %s is a list, response has %T", shapeID, v)})
			return
		}
		if shape.Member != nil {
			for i, item := range arr {
				validateSmithyValue(idx, op, shape.Member.Target, fmt.Sprintf("%s[%d]", path, i), item, out)
			}
		}
	case "map":
		obj, ok := v.(map[string]any)
		if !ok {
			*out = append(*out, sim.SpecViolation{Op: op, Kind: "type-mismatch", Field: path, Detail: fmt.Sprintf("spec %s is a map, response has %T", shapeID, v)})
			return
		}
		if shape.Value != nil {
			for key, val := range obj {
				validateSmithyValue(idx, op, shape.Value.Target, path+"."+key, val, out)
			}
		}
	case "string", "enum", "blob":
		if _, ok := v.(string); !ok {
			*out = append(*out, sim.SpecViolation{Op: op, Kind: "type-mismatch", Field: path, Detail: fmt.Sprintf("spec %s is a %s, response has %T", shapeID, shape.Type, v)})
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			*out = append(*out, sim.SpecViolation{Op: op, Kind: "type-mismatch", Field: path, Detail: fmt.Sprintf("spec %s is a boolean, response has %T", shapeID, v)})
		}
	case "byte", "short", "integer", "long", "intEnum", "float", "double", "bigInteger", "bigDecimal":
		if _, ok := v.(float64); !ok {
			*out = append(*out, sim.SpecViolation{Op: op, Kind: "type-mismatch", Field: path, Detail: fmt.Sprintf("spec %s is numeric (%s), response has %T", shapeID, shape.Type, v)})
		}
	case "timestamp":
		// awsJson default is epoch-seconds (number); timestampFormat
		// traits allow string encodings.
		switch v.(type) {
		case float64, string:
		default:
			*out = append(*out, sim.SpecViolation{Op: op, Kind: "type-mismatch", Field: path, Detail: fmt.Sprintf("spec %s is a timestamp, response has %T", shapeID, v)})
		}
	case "document":
		// any JSON
	}
}

// validateSmithyPrimitive covers smithy.api# prelude targets.
func validateSmithyPrimitive(op, shapeID, path string, v any, out *[]sim.SpecViolation) {
	short := shapeID
	if i := strings.Index(shapeID, "#"); i >= 0 {
		short = shapeID[i+1:]
	}
	switch short {
	case "String", "Blob":
		if _, ok := v.(string); !ok {
			*out = append(*out, sim.SpecViolation{Op: op, Kind: "type-mismatch", Field: path, Detail: fmt.Sprintf("spec %s, response has %T", shapeID, v)})
		}
	case "Boolean", "PrimitiveBoolean":
		if _, ok := v.(bool); !ok {
			*out = append(*out, sim.SpecViolation{Op: op, Kind: "type-mismatch", Field: path, Detail: fmt.Sprintf("spec %s, response has %T", shapeID, v)})
		}
	case "Byte", "Short", "Integer", "Long", "Float", "Double", "PrimitiveByte", "PrimitiveShort", "PrimitiveInteger", "PrimitiveLong", "PrimitiveFloat", "PrimitiveDouble", "BigInteger", "BigDecimal":
		if _, ok := v.(float64); !ok {
			*out = append(*out, sim.SpecViolation{Op: op, Kind: "type-mismatch", Field: path, Detail: fmt.Sprintf("spec %s, response has %T", shapeID, v)})
		}
	case "Timestamp":
		switch v.(type) {
		case float64, string:
		default:
			*out = append(*out, sim.SpecViolation{Op: op, Kind: "type-mismatch", Field: path, Detail: fmt.Sprintf("spec %s, response has %T", shapeID, v)})
		}
	case "Document", "Unit":
		// any JSON / no value
	}
}
