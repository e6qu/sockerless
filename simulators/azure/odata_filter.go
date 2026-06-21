package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Azure ARM `$filter` (OData) support + the `$top`/`$skiptoken` and `$orderby`
// query options. azureApplyListQuery evaluates `$filter` against each resource's
// JSON, sorts by `$orderby`, then pages via armPage — so a list handler gets the
// full documented query-option surface from one call. Previously $filter was
// ignored (and most lists ignored $top too).
//
// $filter grammar (the ARM/OData subset clients use):
//
//	expr       = or
//	or         = and { "or" and }
//	and        = not { "and" not }
//	not        = ["not"] term
//	term       = "(" expr ")" | function | comparison
//	function   = startswith(field,'v') | endswith(field,'v') | contains(field,'v')
//	           | substringof('v',field)
//	comparison = field (eq|ne|gt|ge|lt|le) value
//	field      = name { "/" name }                 (nested via '/')
//	value      = 'string' | number | true | false | null
func azureApplyListQuery[T any](items []T, r *http.Request) []T {
	filter := strings.TrimSpace(r.URL.Query().Get("$filter"))
	orderby := strings.TrimSpace(r.URL.Query().Get("$orderby"))

	if filter != "" {
		node := azureParseODataFilter(filter)
		kept := make([]T, 0, len(items))
		for _, it := range items {
			var m map[string]any
			if b, err := json.Marshal(it); err == nil {
				_ = json.Unmarshal(b, &m)
			}
			if node.eval(m) {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	if orderby != "" {
		field, desc := azureParseOrderBy(orderby)
		maps := make([]map[string]any, len(items))
		for i, it := range items {
			if b, err := json.Marshal(it); err == nil {
				_ = json.Unmarshal(b, &maps[i])
			}
		}
		idx := make([]int, len(items))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool {
			x, y := azureFieldString(maps[idx[a]], field), azureFieldString(maps[idx[b]], field)
			if desc {
				return x > y
			}
			return x < y
		})
		out := make([]T, len(items))
		for i, j := range idx {
			out[i] = items[j]
		}
		items = out
	}
	return items
}

func azureParseOrderBy(s string) (field string, desc bool) {
	s = strings.TrimSpace(strings.Split(s, ",")[0])
	if strings.HasSuffix(strings.ToLower(s), " desc") {
		return strings.TrimSpace(s[:len(s)-5]), true
	}
	if strings.HasSuffix(strings.ToLower(s), " asc") {
		return strings.TrimSpace(s[:len(s)-4]), false
	}
	return s, false
}

// ── AST ────────────────────────────────────────────────────────────────────

type odataNode interface{ eval(m map[string]any) bool }

type odataTrue struct{}

func (odataTrue) eval(map[string]any) bool { return true }

type odataOr struct{ l, r odataNode }

func (n odataOr) eval(m map[string]any) bool { return n.l.eval(m) || n.r.eval(m) }

type odataAnd struct{ l, r odataNode }

func (n odataAnd) eval(m map[string]any) bool { return n.l.eval(m) && n.r.eval(m) }

type odataNot struct{ inner odataNode }

func (n odataNot) eval(m map[string]any) bool { return !n.inner.eval(m) }

type odataCmp struct{ field, op, value string }

func (n odataCmp) eval(m map[string]any) bool {
	actual, present := azureFieldLookup(m, n.field)
	switch n.op {
	case "eq":
		return present && actual == n.value
	case "ne":
		return !present || actual != n.value
	case "gt", "ge", "lt", "le":
		return present && azureNumCompare(actual, n.op, n.value)
	}
	return false
}

type odataFunc struct{ name, field, value string }

func (n odataFunc) eval(m map[string]any) bool {
	actual, present := azureFieldLookup(m, n.field)
	if !present {
		return false
	}
	switch n.name {
	case "startswith":
		return strings.HasPrefix(actual, n.value)
	case "endswith":
		return strings.HasSuffix(actual, n.value)
	case "contains", "substringof":
		return strings.Contains(actual, n.value)
	}
	return false
}

// ── tokenizer ──────────────────────────────────────────────────────────────

type odataTokKind int

const (
	odataEOF odataTokKind = iota
	odataLParen
	odataRParen
	odataComma
	odataWord
	odataString
)

type odataTok struct {
	kind odataTokKind
	text string
}

func azureODataTokenize(s string) []odataTok {
	var toks []odataTok
	sc := sim.NewScanner(s)
	for !sc.Eof() {
		c := sc.Peek()
		switch c {
		case ' ', '\t', '\n':
			sc.Next()
		case '(':
			toks = append(toks, odataTok{odataLParen, "("})
			sc.Next()
		case ')':
			toks = append(toks, odataTok{odataRParen, ")"})
			sc.Next()
		case ',':
			toks = append(toks, odataTok{odataComma, ","})
			sc.Next()
		case '\'':
			sc.Next()
			var b strings.Builder
			for !sc.Eof() {
				if sc.Peek() == '\'' {
					if sc.PeekAt(1) == '\'' { // '' escape
						b.WriteByte('\'')
						sc.Next()
						sc.Next()
						continue
					}
					break
				}
				b.WriteByte(sc.Next())
			}
			if !sc.Eof() {
				sc.Next()
			}
			toks = append(toks, odataTok{odataString, b.String()})
		default:
			start := sc.Pos()
			for !sc.Eof() {
				ch := sc.Peek()
				if ch == ' ' || ch == '\t' || ch == '\n' || ch == '(' || ch == ')' || ch == ',' || ch == '\'' {
					break
				}
				sc.Next()
			}
			word := sc.Slice(start, sc.Pos())
			// Typed-literal prefix: datetime'…' / guid'…' / X'…' / binary'…'.
			// Real OData wraps a typed value in `<type>'<value>'`; the inner
			// value is what the filter compares against. Recognise the prefix
			// (case-insensitive) immediately followed by a quote and emit a
			// single string token carrying the unwrapped value.
			if sc.Peek() == '\'' && odataIsTypedLiteralPrefix(word) {
				sc.Next() // opening quote
				var b strings.Builder
				for !sc.Eof() {
					if sc.Peek() == '\'' {
						if sc.PeekAt(1) == '\'' {
							b.WriteByte('\'')
							sc.Next()
							sc.Next()
							continue
						}
						break
					}
					b.WriteByte(sc.Next())
				}
				if !sc.Eof() {
					sc.Next() // closing quote
				}
				toks = append(toks, odataTok{odataString, b.String()})
				continue
			}
			// Numeric type suffix: 123L / 1.5f / 2.0d / 9.99m. Strip the suffix
			// and emit the bare number as a word (numeric comparison unwraps it).
			if stripped, ok := odataStripNumericSuffix(word); ok {
				word = stripped
			}
			toks = append(toks, odataTok{odataWord, word})
		}
	}
	return append(toks, odataTok{odataEOF, ""})
}

// odataIsTypedLiteralPrefix reports whether word is one of the OData typed-value
// prefixes that wrap their payload in quotes.
func odataIsTypedLiteralPrefix(word string) bool {
	switch strings.ToLower(word) {
	case "datetime", "datetimeoffset", "guid", "binary", "x", "time", "duration":
		return true
	}
	return false
}

// odataStripNumericSuffix removes a single OData numeric type suffix (L/f/d/m,
// case-insensitive) from an otherwise-numeric literal, returning the bare number.
func odataStripNumericSuffix(word string) (string, bool) {
	if len(word) < 2 {
		return word, false
	}
	last := word[len(word)-1]
	switch last {
	case 'L', 'l', 'f', 'F', 'd', 'D', 'm', 'M':
	default:
		return word, false
	}
	bare := word[:len(word)-1]
	if _, err := strconv.ParseFloat(bare, 64); err != nil {
		return word, false
	}
	return bare, true
}

// ── parser ─────────────────────────────────────────────────────────────────

type odataParser struct {
	toks  []odataTok
	pos   int
	guard *sim.ParseGuard
}

// maxODataParseDepth bounds parenthesis nesting so a pathological $filter can't
// overflow the goroutine stack and crash the sim process (a Go stack-overflow
// fatal error is not recoverable).
const maxODataParseDepth = 1000

func azureParseODataFilter(s string) odataNode {
	s = strings.TrimSpace(s)
	if s == "" {
		return odataTrue{}
	}
	p := &odataParser{toks: azureODataTokenize(s), guard: sim.NewParseGuard(maxODataParseDepth, -1)}
	node := p.parseOr()
	if node == nil {
		return odataTrue{}
	}
	return node
}

func (p *odataParser) peek() odataTok { return p.toks[p.pos] }
func (p *odataParser) next() odataTok { t := p.toks[p.pos]; p.pos++; return t }

func (p *odataParser) isKeyword(kw string) bool {
	return p.peek().kind == odataWord && strings.EqualFold(p.peek().text, kw)
}

func (p *odataParser) parseOr() odataNode {
	left := p.parseAnd()
	for p.isKeyword("or") {
		p.next()
		left = odataOr{left, p.parseAnd()}
	}
	return left
}

func (p *odataParser) parseAnd() odataNode {
	left := p.parseNot()
	for p.isKeyword("and") {
		p.next()
		left = odataAnd{left, p.parseNot()}
	}
	return left
}

func (p *odataParser) parseNot() odataNode {
	if p.isKeyword("not") {
		p.next()
		return odataNot{p.parseNot()}
	}
	return p.parseTerm()
}

func (p *odataParser) parseTerm() odataNode {
	if p.peek().kind == odataLParen {
		p.next()
		if !p.guard.Enter() {
			p.guard.Leave()
			return odataTrue{}
		}
		inner := p.parseOr()
		p.guard.Leave()
		if p.peek().kind == odataRParen {
			p.next()
		}
		return inner
	}
	// Function call: name ( args )
	if p.peek().kind == odataWord {
		switch strings.ToLower(p.peek().text) {
		case "startswith", "endswith", "contains":
			name := strings.ToLower(p.next().text)
			field, value := p.parseFuncFieldValue()
			return odataFunc{name: name, field: field, value: value}
		case "substringof":
			p.next()
			// substringof('value', field)
			value, field := p.parseFuncValueField()
			return odataFunc{name: "substringof", field: field, value: value}
		}
	}
	// comparison: field op value
	if p.peek().kind != odataWord {
		if p.peek().kind != odataEOF {
			p.next()
		}
		return odataTrue{}
	}
	field := p.next().text
	op := ""
	if p.peek().kind == odataWord {
		op = strings.ToLower(p.next().text)
	}
	value := ""
	if p.peek().kind == odataString || p.peek().kind == odataWord {
		value = p.next().text
	}
	return odataCmp{field: field, op: op, value: value}
}

func (p *odataParser) parseFuncFieldValue() (field, value string) {
	if p.peek().kind == odataLParen {
		p.next()
	}
	if p.peek().kind == odataWord {
		field = p.next().text
	}
	if p.peek().kind == odataComma {
		p.next()
	}
	if p.peek().kind == odataString || p.peek().kind == odataWord {
		value = p.next().text
	}
	if p.peek().kind == odataRParen {
		p.next()
	}
	return field, value
}

func (p *odataParser) parseFuncValueField() (value, field string) {
	if p.peek().kind == odataLParen {
		p.next()
	}
	if p.peek().kind == odataString || p.peek().kind == odataWord {
		value = p.next().text
	}
	if p.peek().kind == odataComma {
		p.next()
	}
	if p.peek().kind == odataWord {
		field = p.next().text
	}
	if p.peek().kind == odataRParen {
		p.next()
	}
	return value, field
}

// ── helpers ────────────────────────────────────────────────────────────────

func azureFieldLookup(m map[string]any, path string) (string, bool) {
	// OData paths nest via '/'.
	var cur any = m
	for _, seg := range strings.Split(path, "/") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		v, ok := mm[seg]
		if !ok {
			return "", false
		}
		cur = v
	}
	return azureScalarString(cur), true
}

func azureFieldString(m map[string]any, path string) string {
	v, _ := azureFieldLookup(m, path)
	return v
}

func azureScalarString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func azureNumCompare(a, op, b string) bool {
	af, aerr := strconv.ParseFloat(a, 64)
	bf, berr := strconv.ParseFloat(b, 64)
	if aerr != nil || berr != nil {
		switch op {
		case "gt":
			return a > b
		case "lt":
			return a < b
		case "ge":
			return a >= b
		case "le":
			return a <= b
		}
		return false
	}
	switch op {
	case "gt":
		return af > bf
	case "lt":
		return af < bf
	case "ge":
		return af >= bf
	case "le":
		return af <= bf
	}
	return false
}
