package bleephub

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ExprContext holds the evaluation context for GitHub Actions expressions.
type ExprContext struct {
	// DepResults maps needed-job key → result string ("success", "failure",
	// "cancelled", "skipped"); status functions evaluate against it.
	DepResults map[string]string
	// WorkflowCancelled indicates the workflow was cancelled (cancelled()).
	WorkflowCancelled bool
	// Contexts maps root context names ("github", "needs", "vars",
	// "inputs", ...) to their values: nested
	// map[string]interface{} / []interface{} / string / float64 / bool / nil.
	Contexts map[string]interface{}
}

// EvalExprErr evaluates a GitHub Actions expression to a boolean.
// The empty expression is true (an absent `if:` always runs).
func EvalExprErr(expr string, ctx *ExprContext) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}
	v, err := EvalExprValue(expr, ctx)
	if err != nil {
		return false, err
	}
	return exprTruthy(v), nil
}

// EvalExprValue evaluates a GitHub Actions expression to its value.
// A surrounding ${{ ... }} wrapper is stripped first (job-level `if:`
// accepts both bare and wrapped forms).
func EvalExprValue(expr string, ctx *ExprContext) (interface{}, error) {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "${{") && strings.HasSuffix(expr, "}}") {
		expr = strings.TrimSpace(expr[3 : len(expr)-2])
	}
	if expr == "" {
		return true, nil
	}
	toks, err := lexExpr(expr)
	if err != nil {
		return nil, err
	}
	p := &exprParser{toks: toks, ctx: ctx}
	v, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokEOF {
		return nil, fmt.Errorf("unexpected %q in expression %q", p.peek().text, expr)
	}
	return v, nil
}

// EvalTemplate replaces every ${{ ... }} occurrence in s with the
// stringified value of the inner expression — the server-side subset of
// workflow-file template expansion (concurrency groups, run names,
// workflow_call inputs).
func EvalTemplate(s string, ctx *ExprContext) (string, error) {
	var out strings.Builder
	for {
		start := strings.Index(s, "${{")
		if start < 0 {
			out.WriteString(s)
			return out.String(), nil
		}
		end := strings.Index(s[start:], "}}")
		if end < 0 {
			return "", fmt.Errorf("unterminated ${{ in %q", s)
		}
		out.WriteString(s[:start])
		inner := s[start+3 : start+end]
		v, err := EvalExprValue(inner, ctx)
		if err != nil {
			return "", err
		}
		out.WriteString(exprToString(v))
		s = s[start+end+2:]
	}
}

// ExprContainsStatusFunction checks if an expression contains always() or failure()
// which would override default dependency-failure skip behavior.
func ExprContainsStatusFunction(expr string) (hasAlways, hasFailure bool) {
	lower := strings.ToLower(expr)
	hasAlways = strings.Contains(lower, "always()")
	hasFailure = strings.Contains(lower, "failure()")
	return
}

// ── Lexer ───────────────────────────────────────────────────────────

type tokKind int

const (
	tokEOF tokKind = iota
	tokIdent
	tokNumber
	tokString
	tokLParen
	tokRParen
	tokLBracket
	tokRBracket
	tokDot
	tokComma
	tokStar
	tokNot
	tokEq
	tokNe
	tokLt
	tokLe
	tokGt
	tokGe
	tokAnd
	tokOr
)

type exprToken struct {
	kind tokKind
	text string
	num  float64
}

func lexExpr(input string) ([]exprToken, error) {
	var toks []exprToken
	i := 0
	for i < len(input) {
		c := input[i]
		switch {
		case unicode.IsSpace(rune(c)):
			i++
		case c == '(':
			toks = append(toks, exprToken{kind: tokLParen, text: "("})
			i++
		case c == ')':
			toks = append(toks, exprToken{kind: tokRParen, text: ")"})
			i++
		case c == '[':
			toks = append(toks, exprToken{kind: tokLBracket, text: "["})
			i++
		case c == ']':
			toks = append(toks, exprToken{kind: tokRBracket, text: "]"})
			i++
		case c == '.' && (i+1 >= len(input) || !isDigit(input[i+1]) || len(toks) == 0 || !startsNumberContext(toks)):
			toks = append(toks, exprToken{kind: tokDot, text: "."})
			i++
		case c == ',':
			toks = append(toks, exprToken{kind: tokComma, text: ","})
			i++
		case c == '*':
			toks = append(toks, exprToken{kind: tokStar, text: "*"})
			i++
		case c == '!':
			if i+1 < len(input) && input[i+1] == '=' {
				toks = append(toks, exprToken{kind: tokNe, text: "!="})
				i += 2
			} else {
				toks = append(toks, exprToken{kind: tokNot, text: "!"})
				i++
			}
		case c == '=':
			if i+1 < len(input) && input[i+1] == '=' {
				toks = append(toks, exprToken{kind: tokEq, text: "=="})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected '=' (did you mean '==') in expression")
			}
		case c == '<':
			if i+1 < len(input) && input[i+1] == '=' {
				toks = append(toks, exprToken{kind: tokLe, text: "<="})
				i += 2
			} else {
				toks = append(toks, exprToken{kind: tokLt, text: "<"})
				i++
			}
		case c == '>':
			if i+1 < len(input) && input[i+1] == '=' {
				toks = append(toks, exprToken{kind: tokGe, text: ">="})
				i += 2
			} else {
				toks = append(toks, exprToken{kind: tokGt, text: ">"})
				i++
			}
		case c == '&':
			if i+1 < len(input) && input[i+1] == '&' {
				toks = append(toks, exprToken{kind: tokAnd, text: "&&"})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected '&' in expression")
			}
		case c == '|':
			if i+1 < len(input) && input[i+1] == '|' {
				toks = append(toks, exprToken{kind: tokOr, text: "||"})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected '|' in expression")
			}
		case c == '\'':
			// Single-quoted string; '' escapes a literal quote.
			var sb strings.Builder
			i++
			for {
				if i >= len(input) {
					return nil, fmt.Errorf("unterminated string literal")
				}
				if input[i] == '\'' {
					if i+1 < len(input) && input[i+1] == '\'' {
						sb.WriteByte('\'')
						i += 2
						continue
					}
					i++
					break
				}
				sb.WriteByte(input[i])
				i++
			}
			toks = append(toks, exprToken{kind: tokString, text: sb.String()})
		case isDigit(c) || (c == '-' && i+1 < len(input) && isDigit(input[i+1])):
			start := i
			if c == '-' {
				i++
			}
			if i+1 < len(input) && input[i] == '0' && (input[i+1] == 'x' || input[i+1] == 'X') {
				i += 2
				for i < len(input) && isHexDigit(input[i]) {
					i++
				}
				hex := strings.TrimPrefix(input[start:i], "-")
				u, err := strconv.ParseUint(hex[2:], 16, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid hex literal %q", input[start:i])
				}
				f := float64(u)
				if input[start] == '-' {
					f = -f
				}
				toks = append(toks, exprToken{kind: tokNumber, num: f, text: input[start:i]})
				continue
			}
			for i < len(input) && (isDigit(input[i]) || input[i] == '.' || input[i] == 'e' || input[i] == 'E' ||
				((input[i] == '+' || input[i] == '-') && (input[i-1] == 'e' || input[i-1] == 'E'))) {
				i++
			}
			f, err := strconv.ParseFloat(input[start:i], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid number literal %q", input[start:i])
			}
			toks = append(toks, exprToken{kind: tokNumber, num: f, text: input[start:i]})
		case isIdentStart(c):
			start := i
			for i < len(input) && isIdentChar(input[i]) {
				i++
			}
			toks = append(toks, exprToken{kind: tokIdent, text: input[start:i]})
		default:
			return nil, fmt.Errorf("unexpected character %q in expression", string(c))
		}
	}
	toks = append(toks, exprToken{kind: tokEOF})
	return toks, nil
}

// startsNumberContext reports whether a '.' at the current position could
// begin a number rather than a property dereference: only when the
// previous token cannot end a dereferencable value.
func startsNumberContext(toks []exprToken) bool {
	last := toks[len(toks)-1]
	switch last.kind {
	case tokIdent, tokRParen, tokRBracket, tokString, tokNumber, tokStar:
		return false
	}
	return true
}

func isDigit(c byte) bool    { return c >= '0' && c <= '9' }
func isHexDigit(c byte) bool { return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') }
func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isIdentChar(c byte) bool { return isIdentStart(c) || isDigit(c) || c == '-' }

// ── Parser / evaluator ──────────────────────────────────────────────
//
// Precedence (loosest to tightest), per the GitHub Actions expression
// spec: || , && , (== !=) , (< <= > >=) , ! , dereference, primary.

type exprParser struct {
	toks []exprToken
	pos  int
	ctx  *ExprContext
}

func (p *exprParser) peek() exprToken { return p.toks[p.pos] }
func (p *exprParser) next() exprToken { t := p.toks[p.pos]; p.pos++; return t }

func (p *exprParser) parseOr() (interface{}, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		// Value-preserving short-circuit semantics (like JavaScript):
		// the first truthy operand wins.
		if !exprTruthy(left) {
			left = right
		}
	}
	return left, nil
}

func (p *exprParser) parseAnd() (interface{}, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokAnd {
		p.next()
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		if exprTruthy(left) {
			left = right
		}
	}
	return left, nil
}

func (p *exprParser) parseEquality() (interface{}, error) {
	left, err := p.parseRelational()
	if err != nil {
		return nil, err
	}
	for {
		k := p.peek().kind
		if k != tokEq && k != tokNe {
			return left, nil
		}
		p.next()
		right, err := p.parseRelational()
		if err != nil {
			return nil, err
		}
		eq := exprLooseEqual(left, right)
		if k == tokEq {
			left = eq
		} else {
			left = !eq
		}
	}
}

func (p *exprParser) parseRelational() (interface{}, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		k := p.peek().kind
		if k != tokLt && k != tokLe && k != tokGt && k != tokGe {
			return left, nil
		}
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		ln, rn := exprToNumber(left), exprToNumber(right)
		if math.IsNaN(ln) || math.IsNaN(rn) {
			left = false
			continue
		}
		switch k {
		case tokLt:
			left = ln < rn
		case tokLe:
			left = ln <= rn
		case tokGt:
			left = ln > rn
		case tokGe:
			left = ln >= rn
		}
	}
}

func (p *exprParser) parseUnary() (interface{}, error) {
	if p.peek().kind == tokNot {
		p.next()
		v, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return !exprTruthy(v), nil
	}
	return p.parsePostfix()
}

// parsePostfix parses a primary value followed by any chain of
// dereferences: .name, ['key'], [index].
func (p *exprParser) parsePostfix() (interface{}, error) {
	v, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		switch p.peek().kind {
		case tokDot:
			p.next()
			t := p.next()
			if t.kind == tokStar {
				return nil, fmt.Errorf("object filters (.*) are not supported in server-evaluated expressions")
			}
			if t.kind != tokIdent {
				return nil, fmt.Errorf("expected property name after '.', got %q", t.text)
			}
			v = exprDeref(v, t.text)
		case tokLBracket:
			p.next()
			idx, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			if p.peek().kind != tokRBracket {
				return nil, fmt.Errorf("expected ']'")
			}
			p.next()
			v = exprIndex(v, idx)
		default:
			return v, nil
		}
	}
}

func (p *exprParser) parsePrimary() (interface{}, error) {
	t := p.next()
	switch t.kind {
	case tokLParen:
		v, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokRParen {
			return nil, fmt.Errorf("expected ')'")
		}
		p.next()
		return v, nil
	case tokString:
		return t.text, nil
	case tokNumber:
		return t.num, nil
	case tokIdent:
		// Function call?
		if p.peek().kind == tokLParen {
			p.next()
			var args []interface{}
			if p.peek().kind != tokRParen {
				for {
					a, err := p.parseOr()
					if err != nil {
						return nil, err
					}
					args = append(args, a)
					if p.peek().kind != tokComma {
						break
					}
					p.next()
				}
			}
			if p.peek().kind != tokRParen {
				return nil, fmt.Errorf("expected ')' after arguments to %s", t.text)
			}
			p.next()
			return p.callFunction(t.text, args)
		}
		switch strings.ToLower(t.text) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "null":
			return nil, nil
		case "infinity":
			return math.Inf(1), nil
		case "nan":
			return math.NaN(), nil
		}
		// Root context lookup (case-insensitive, like all context keys).
		if p.ctx != nil && p.ctx.Contexts != nil {
			for k, v := range p.ctx.Contexts {
				if strings.EqualFold(k, t.text) {
					return v, nil
				}
			}
		}
		return nil, fmt.Errorf("unrecognized named value %q", t.text)
	default:
		return nil, fmt.Errorf("unexpected token %q", t.text)
	}
}

func (p *exprParser) callFunction(name string, args []interface{}) (interface{}, error) {
	switch strings.ToLower(name) {
	case "success":
		if p.ctx == nil {
			return true, nil
		}
		if p.ctx.WorkflowCancelled {
			return false, nil
		}
		for _, r := range p.ctx.DepResults {
			if r != "success" && r != "skipped" {
				return false, nil
			}
		}
		return true, nil
	case "failure":
		if p.ctx == nil {
			return false, nil
		}
		for _, r := range p.ctx.DepResults {
			if r == "failure" {
				return true, nil
			}
		}
		return false, nil
	case "cancelled":
		return p.ctx != nil && p.ctx.WorkflowCancelled, nil
	case "always":
		return true, nil
	case "contains":
		if err := wantArgs("contains", args, 2); err != nil {
			return nil, err
		}
		if arr, ok := args[0].([]interface{}); ok {
			for _, item := range arr {
				if exprLooseEqual(item, args[1]) {
					return true, nil
				}
			}
			return false, nil
		}
		return strings.Contains(strings.ToLower(exprToString(args[0])), strings.ToLower(exprToString(args[1]))), nil
	case "startswith":
		if err := wantArgs("startsWith", args, 2); err != nil {
			return nil, err
		}
		return strings.HasPrefix(strings.ToLower(exprToString(args[0])), strings.ToLower(exprToString(args[1]))), nil
	case "endswith":
		if err := wantArgs("endsWith", args, 2); err != nil {
			return nil, err
		}
		return strings.HasSuffix(strings.ToLower(exprToString(args[0])), strings.ToLower(exprToString(args[1]))), nil
	case "format":
		if len(args) < 1 {
			return nil, fmt.Errorf("format() needs a format string")
		}
		return exprFormat(exprToString(args[0]), args[1:])
	case "join":
		if len(args) < 1 || len(args) > 2 {
			return nil, fmt.Errorf("join() takes 1 or 2 arguments, got %d", len(args))
		}
		sep := ","
		if len(args) == 2 {
			sep = exprToString(args[1])
		}
		arr, ok := args[0].([]interface{})
		if !ok {
			return exprToString(args[0]), nil
		}
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			parts = append(parts, exprToString(item))
		}
		return strings.Join(parts, sep), nil
	case "tojson":
		if err := wantArgs("toJSON", args, 1); err != nil {
			return nil, err
		}
		b, err := json.MarshalIndent(args[0], "", "  ")
		if err != nil {
			return nil, fmt.Errorf("toJSON: %w", err)
		}
		return string(b), nil
	case "fromjson":
		if err := wantArgs("fromJSON", args, 1); err != nil {
			return nil, err
		}
		var v interface{}
		if err := json.Unmarshal([]byte(exprToString(args[0])), &v); err != nil {
			return nil, fmt.Errorf("fromJSON: %w", err)
		}
		return v, nil
	case "hashfiles":
		// hashFiles inspects the runner workspace, which doesn't exist
		// server-side; real GitHub only evaluates it on the runner.
		return nil, fmt.Errorf("hashFiles() is not available in server-evaluated expressions")
	default:
		return nil, fmt.Errorf("unrecognized function %q", name)
	}
}

func wantArgs(name string, args []interface{}, n int) error {
	if len(args) != n {
		return fmt.Errorf("%s() takes %d arguments, got %d", name, n, len(args))
	}
	return nil
}

// exprFormat implements format('{0} {1}', ...): {N} placeholders,
// '{{' and '}}' escape literal braces.
func exprFormat(f string, args []interface{}) (string, error) {
	var out strings.Builder
	for i := 0; i < len(f); i++ {
		switch {
		case f[i] == '{' && i+1 < len(f) && f[i+1] == '{':
			out.WriteByte('{')
			i++
		case f[i] == '}' && i+1 < len(f) && f[i+1] == '}':
			out.WriteByte('}')
			i++
		case f[i] == '{':
			end := strings.IndexByte(f[i:], '}')
			if end < 0 {
				return "", fmt.Errorf("format(): unmatched '{' in %q", f)
			}
			idx, err := strconv.Atoi(f[i+1 : i+end])
			if err != nil || idx < 0 || idx >= len(args) {
				return "", fmt.Errorf("format(): invalid placeholder {%s}", f[i+1:i+end])
			}
			out.WriteString(exprToString(args[idx]))
			i += end
		case f[i] == '}':
			return "", fmt.Errorf("format(): unmatched '}' in %q", f)
		default:
			out.WriteByte(f[i])
		}
	}
	return out.String(), nil
}

// ── Value semantics ─────────────────────────────────────────────────

// exprDeref resolves a property access (case-insensitively, like GitHub
// context keys). Missing properties yield null, not an error.
func exprDeref(v interface{}, key string) interface{} {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	if direct, ok := m[key]; ok {
		return direct
	}
	// Deterministic case-insensitive fallback: pick the smallest matching
	// key so map iteration order can't flip the result.
	var keys []string
	for k := range m {
		if strings.EqualFold(k, key) {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	return m[keys[0]]
}

// exprIndex resolves an ['key'] or [N] access.
func exprIndex(v, idx interface{}) interface{} {
	if arr, ok := v.([]interface{}); ok {
		n := exprToNumber(idx)
		if math.IsNaN(n) || n < 0 || int(n) >= len(arr) {
			return nil
		}
		return arr[int(n)]
	}
	if s, ok := idx.(string); ok {
		return exprDeref(v, s)
	}
	return nil
}

// exprTruthy implements GitHub's truthiness: false, 0, -0, "", and null
// are falsy; everything else (including NaN per the JS-like rules GitHub
// documents, where NaN is falsy) follows.
func exprTruthy(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case float64:
		return t != 0 && !math.IsNaN(t)
	case string:
		return t != ""
	default:
		return true
	}
}

// exprLooseEqual implements GitHub's loose equality: same-type strings
// compare case-insensitively; mixed scalar types coerce to number;
// arrays/objects equal only the same instance (compared as never-equal
// here — GitHub compares by reference).
func exprLooseEqual(a, b interface{}) bool {
	as, aIsStr := a.(string)
	bs, bIsStr := b.(string)
	if aIsStr && bIsStr {
		return strings.EqualFold(as, bs)
	}
	switch a.(type) {
	case []interface{}, map[string]interface{}:
		return false
	}
	switch b.(type) {
	case []interface{}, map[string]interface{}:
		return false
	}
	an, bn := exprToNumber(a), exprToNumber(b)
	if math.IsNaN(an) || math.IsNaN(bn) {
		return false
	}
	return an == bn
}

// exprToNumber implements GitHub's number coercion: null→0, bool→0/1,
// strings parse as numbers (” → 0, unparseable → NaN), arrays/objects → NaN.
func exprToNumber(v interface{}) float64 {
	switch t := v.(type) {
	case nil:
		return 0
	case bool:
		if t {
			return 1
		}
		return 0
	case float64:
		return t
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0
		}
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			if u, err := strconv.ParseUint(s[2:], 16, 64); err == nil {
				return float64(u)
			}
			return math.NaN()
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return math.NaN()
		}
		return f
	default:
		return math.NaN()
	}
}

// exprToString renders a value the way GitHub interpolates it into
// strings: null→”, bools→true/false, numbers in shortest form,
// arrays/objects as the literal words GitHub uses.
func exprToString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		if math.IsNaN(t) {
			return "NaN"
		}
		if math.IsInf(t, 1) {
			return "Infinity"
		}
		if math.IsInf(t, -1) {
			return "-Infinity"
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case string:
		return t
	case []interface{}:
		return "Array"
	case map[string]interface{}:
		return "Object"
	default:
		return fmt.Sprintf("%v", t)
	}
}
