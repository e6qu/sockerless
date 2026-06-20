package main

import (
	"fmt"
	"strconv"
	"strings"
)

// DynamoDB UpdateExpression evaluator.
//
// Modern clients (aws CLI / SDK / terraform-provider-aws) drive UpdateItem with
// an UpdateExpression, not the legacy AttributeUpdates parameter. This applies
// the common subset, in place, against the stored item (attribute values are
// the wire shape, e.g. {"N":"5"} / {"S":"x"} / {"SS":[...]}):
//
//	SET  path = operand                 (assignment)
//	SET  path = operand +|- operand     (numeric arithmetic)
//	SET  path = if_not_exists(path, op)  (default when absent)
//	REMOVE path[, path...]
//	ADD  path operand                   (number increment, or string/number-set union)
//	DELETE path operand                 (set-element removal)
//
// Placeholders #name (ExpressionAttributeNames) and :val
// (ExpressionAttributeValues) are resolved. Top-level attribute paths only —
// nested document paths are not modelled.
func ddbApplyUpdateExpression(item map[string]any, expr string, names map[string]string, values map[string]any) error {
	for kw, body := range ddbSplitUpdateClauses(expr) {
		switch kw {
		case "SET":
			for _, part := range ddbSplitTopLevel(body, ',') {
				eq := strings.Index(part, "=")
				if eq < 0 {
					return fmt.Errorf("invalid SET action %q", strings.TrimSpace(part))
				}
				path := ddbResolveName(strings.TrimSpace(part[:eq]), names)
				val, err := ddbEvalSetRHS(strings.TrimSpace(part[eq+1:]), item, names, values)
				if err != nil {
					return err
				}
				item[path] = val
			}
		case "REMOVE":
			for _, p := range ddbSplitTopLevel(body, ',') {
				delete(item, ddbResolveName(strings.TrimSpace(p), names))
			}
		case "ADD":
			for _, p := range ddbSplitTopLevel(body, ',') {
				path, operand, err := ddbPathOperand(p, item, names, values)
				if err != nil {
					return err
				}
				item[path] = ddbAddValues(item[path], operand)
			}
		case "DELETE":
			for _, p := range ddbSplitTopLevel(body, ',') {
				path, operand, err := ddbPathOperand(p, item, names, values)
				if err != nil {
					return err
				}
				if cur, ok := item[path]; ok {
					item[path] = ddbDeleteSetElems(cur, operand)
				}
			}
		}
	}
	return nil
}

// ddbSplitUpdateClauses splits an UpdateExpression into {KEYWORD: body}. The
// four action keywords each appear at most once and introduce a clause.
func ddbSplitUpdateClauses(expr string) map[string]string {
	keywords := []string{"SET", "REMOVE", "ADD", "DELETE"}
	type mark struct {
		idx, end int
		kw       string
	}
	var marks []mark
	// ASCII-only uppercasing that preserves byte length, so the keyword indices
	// computed against `upper` remain valid offsets into the original `expr`.
	// strings.ToUpper can change the byte length of non-ASCII / invalid-UTF-8
	// input, which would make the slice offsets below out-of-range for expr.
	upper := ddbASCIIUpper(expr)
	for _, kw := range keywords {
		for i := 0; i+len(kw) <= len(upper); i++ {
			if upper[i:i+len(kw)] != kw {
				continue
			}
			if i > 0 && isWordChar(upper[i-1]) {
				continue
			}
			if i+len(kw) < len(upper) && isWordChar(upper[i+len(kw)]) {
				continue
			}
			marks = append(marks, mark{idx: i, end: i + len(kw), kw: kw})
			break
		}
	}
	// Order clause starts so each body runs to the next clause.
	for i := 0; i < len(marks); i++ {
		for j := i + 1; j < len(marks); j++ {
			if marks[j].idx < marks[i].idx {
				marks[i], marks[j] = marks[j], marks[i]
			}
		}
	}
	out := map[string]string{}
	for i, m := range marks {
		bodyEnd := len(expr)
		if i+1 < len(marks) {
			bodyEnd = marks[i+1].idx
		}
		out[m.kw] = strings.TrimSpace(expr[m.end:bodyEnd])
	}
	return out
}

// ddbASCIIUpper uppercases only ASCII letters, byte-for-byte, leaving every
// other byte (including invalid-UTF-8 bytes) untouched so the result has the
// same byte length and indexing as the input.
func ddbASCIIUpper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 'a' - 'A'
		}
	}
	return string(b)
}

func isWordChar(b byte) bool {
	return b == '_' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

// ddbSplitTopLevel splits on sep at paren depth 0 (so if_not_exists(a, b)
// commas are not split).
func ddbSplitTopLevel(s string, sep byte) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(s[start:]); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func ddbResolveName(token string, names map[string]string) string {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(token, "#") {
		if n, ok := names[token]; ok {
			return n
		}
	}
	return token
}

func ddbPathOperand(part string, item map[string]any, names map[string]string, values map[string]any) (string, any, error) {
	fields := strings.Fields(strings.TrimSpace(part))
	if len(fields) != 2 {
		return "", nil, fmt.Errorf("invalid action %q", strings.TrimSpace(part))
	}
	operand, err := ddbResolveOperand(fields[1], item, names, values)
	if err != nil {
		return "", nil, err
	}
	return ddbResolveName(fields[0], names), operand, nil
}

// ddbResolveOperand resolves a single operand to its attribute-value: a :value
// placeholder, or a #name / literal attribute's current value.
func ddbResolveOperand(token string, item map[string]any, names map[string]string, values map[string]any) (any, error) {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(token, ":") {
		v, ok := values[token]
		if !ok {
			return nil, fmt.Errorf("ExpressionAttributeValues missing %q", token)
		}
		return v, nil
	}
	return item[ddbResolveName(token, names)], nil
}

func ddbEvalSetRHS(rhs string, item map[string]any, names map[string]string, values map[string]any) (any, error) {
	rhs = strings.TrimSpace(rhs)
	const inpfx = "if_not_exists("
	if strings.HasPrefix(strings.ToLower(rhs), inpfx) {
		if !strings.HasSuffix(rhs, ")") || len(rhs) < len(inpfx)+1 {
			return nil, fmt.Errorf("malformed if_not_exists: %q", rhs)
		}
		inner := rhs[len(inpfx) : len(rhs)-1]
		args := ddbSplitTopLevel(inner, ',')
		if len(args) != 2 {
			return nil, fmt.Errorf("if_not_exists expects 2 args: %q", rhs)
		}
		if cur, ok := item[ddbResolveName(args[0], names)]; ok {
			return cur, nil
		}
		return ddbResolveOperand(args[1], item, names, values)
	}
	// Binary +/- on numbers (split at top level so paths/placeholders are intact).
	for _, op := range []byte{'+', '-'} {
		if parts := ddbSplitTopLevel(rhs, op); len(parts) == 2 {
			a, err := ddbResolveOperand(parts[0], item, names, values)
			if err != nil {
				return nil, err
			}
			b, err := ddbResolveOperand(parts[1], item, names, values)
			if err != nil {
				return nil, err
			}
			an, bn := ddbToNumber(a), ddbToNumber(b)
			if op == '+' {
				return ddbNumberValue(an + bn), nil
			}
			return ddbNumberValue(an - bn), nil
		}
	}
	return ddbResolveOperand(rhs, item, names, values)
}

func ddbToNumber(v any) float64 {
	if m, ok := v.(map[string]any); ok {
		if n, ok := m["N"].(string); ok {
			f, _ := strconv.ParseFloat(n, 64)
			return f
		}
	}
	return 0
}

func ddbNumberValue(f float64) map[string]any {
	return map[string]any{"N": strconv.FormatFloat(f, 'f', -1, 64)}
}

// ddbAddValues implements ADD: numeric increment, or string/number-set union.
func ddbAddValues(cur, operand any) any {
	om, _ := operand.(map[string]any)
	if om == nil {
		return cur
	}
	for _, st := range []string{"SS", "NS", "BS"} {
		if add, ok := om[st].([]any); ok {
			existing := map[string]bool{}
			var union []any
			if cm, ok := cur.(map[string]any); ok {
				if curSet, ok := cm[st].([]any); ok {
					for _, e := range curSet {
						existing[fmt.Sprintf("%v", e)] = true
						union = append(union, e)
					}
				}
			}
			for _, e := range add {
				if !existing[fmt.Sprintf("%v", e)] {
					existing[fmt.Sprintf("%v", e)] = true
					union = append(union, e)
				}
			}
			return map[string]any{st: union}
		}
	}
	// number increment (ADD on a missing attribute starts from 0)
	return ddbNumberValue(ddbToNumber(cur) + ddbToNumber(operand))
}

// ddbDeleteSetElems implements DELETE: remove the operand's elements from a set.
func ddbDeleteSetElems(cur, operand any) any {
	cm, _ := cur.(map[string]any)
	om, _ := operand.(map[string]any)
	if cm == nil || om == nil {
		return cur
	}
	for _, st := range []string{"SS", "NS", "BS"} {
		curSet, ok1 := cm[st].([]any)
		delSet, ok2 := om[st].([]any)
		if !ok1 || !ok2 {
			continue
		}
		remove := map[string]bool{}
		for _, e := range delSet {
			remove[fmt.Sprintf("%v", e)] = true
		}
		var kept []any
		for _, e := range curSet {
			if !remove[fmt.Sprintf("%v", e)] {
				kept = append(kept, e)
			}
		}
		if len(kept) == 0 {
			return nil // emptying a set removes the attribute
		}
		return map[string]any{st: kept}
	}
	return cur
}
