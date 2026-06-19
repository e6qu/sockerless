package main

import (
	"strconv"
	"strings"
)

// DynamoDB condition / filter / key-condition expression language — a faithful
// implementation of the real grammar, replacing the earlier "=/AND-only" subset:
//
//	expr     = or
//	or       = and { OR and }
//	and      = not { AND not }
//	not      = [NOT] term
//	term     = "(" expr ")" | function | comparison | between | in
//	function = attribute_exists(path) | attribute_not_exists(path)
//	         | attribute_type(path, operand) | begins_with(path, operand)
//	         | contains(path, operand)
//	comparison = operand (= | <> | < | <= | > | >=) operand
//	between  = operand BETWEEN operand AND operand
//	in       = operand IN "(" operand { "," operand } ")"
//	operand  = path | ":valueRef" | size(path)
//	path     = name { "." name | "[" index "]" }       (names may be #aliases)
//
// Operands resolve to DynamoDB attribute values ({"S":…}/{"N":…}/{"M":…}/…);
// conditions resolve to bool. A parse error degrades to a non-match (the safe
// default for a filter/condition), surfaced via the bool return of ddbEvalExpr.

// ddbEvalExpr evaluates a condition/filter expression against an item.
// `exists` reports whether the item currently exists (for attribute_exists /
// attribute_not_exists on a Put condition where item may be empty).
func ddbEvalExpr(item map[string]any, exists bool, expr string, names map[string]string, values map[string]any) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}
	p := &ddbExprParser{toks: ddbExprTokenize(expr)}
	node := p.parseOr()
	if node == nil || p.peek().kind != ddbTokEOF {
		// Unparseable / trailing garbage → treat as a non-match.
		return false
	}
	ev := &ddbEvalCtx{item: item, exists: exists, names: names, values: values}
	return node.eval(ev)
}

type ddbEvalCtx struct {
	item   map[string]any
	exists bool
	names  map[string]string
	values map[string]any
}

// ── AST ────────────────────────────────────────────────────────────────────

type ddbCond interface{ eval(*ddbEvalCtx) bool }

type ddbCondOr struct{ l, r ddbCond }

func (n ddbCondOr) eval(c *ddbEvalCtx) bool { return n.l.eval(c) || n.r.eval(c) }

type ddbCondAnd struct{ l, r ddbCond }

func (n ddbCondAnd) eval(c *ddbEvalCtx) bool { return n.l.eval(c) && n.r.eval(c) }

type ddbCondNot struct{ inner ddbCond }

func (n ddbCondNot) eval(c *ddbEvalCtx) bool { return !n.inner.eval(c) }

type ddbCondFalse struct{}

func (ddbCondFalse) eval(*ddbEvalCtx) bool { return false }

type ddbCondCompare struct {
	l, r ddbOperand
	op   string
}

func (n ddbCondCompare) eval(c *ddbEvalCtx) bool {
	lv, lok := n.l.eval(c)
	rv, rok := n.r.eval(c)
	if !lok || !rok {
		return false
	}
	return ddbCompare(lv, rv, n.op)
}

type ddbCondBetween struct{ v, lo, hi ddbOperand }

func (n ddbCondBetween) eval(c *ddbEvalCtx) bool {
	v, ok := n.v.eval(c)
	lo, ok2 := n.lo.eval(c)
	hi, ok3 := n.hi.eval(c)
	if !ok || !ok2 || !ok3 {
		return false
	}
	return ddbCompare(v, lo, ">=") && ddbCompare(v, hi, "<=")
}

type ddbCondIn struct {
	v    ddbOperand
	list []ddbOperand
}

func (n ddbCondIn) eval(c *ddbEvalCtx) bool {
	v, ok := n.v.eval(c)
	if !ok {
		return false
	}
	for _, o := range n.list {
		if ov, ok := o.eval(c); ok && ddbAttrValuesEqual(v, ov) {
			return true
		}
	}
	return false
}

type ddbCondFunc struct {
	name string
	path string     // path argument
	arg  ddbOperand // optional second argument (begins_with/contains/attribute_type)
}

func (n ddbCondFunc) eval(c *ddbEvalCtx) bool {
	val, present := ddbResolvePath(c.item, n.path, c.names)
	switch n.name {
	case "attribute_exists":
		return c.exists && present && val != nil
	case "attribute_not_exists":
		return !c.exists || !present || val == nil
	case "begins_with":
		want, ok := n.arg.eval(c)
		return ok && present && strings.HasPrefix(ddbScalarString(val), ddbScalarString(want))
	case "contains":
		want, ok := n.arg.eval(c)
		if !ok || !present {
			return false
		}
		if lst, isL := val.(map[string]any)["L"].([]any); isL {
			for _, e := range lst {
				if ddbAttrValuesEqual(e, want) {
					return true
				}
			}
			return false
		}
		return strings.Contains(ddbScalarString(val), ddbScalarString(want))
	case "attribute_type":
		want, ok := n.arg.eval(c)
		if !ok || !present {
			return false
		}
		return ddbAttrTypeCode(val) == ddbScalarString(want)
	}
	return false
}

// ── operands ───────────────────────────────────────────────────────────────

type ddbOperand interface {
	eval(*ddbEvalCtx) (any, bool)
}

type ddbOperandValue struct{ ref string }

func (o ddbOperandValue) eval(c *ddbEvalCtx) (any, bool) {
	v, ok := c.values[o.ref]
	return v, ok
}

type ddbOperandPath struct{ path string }

func (o ddbOperandPath) eval(c *ddbEvalCtx) (any, bool) {
	return ddbResolvePath(c.item, o.path, c.names)
}

type ddbOperandSize struct{ path string }

func (o ddbOperandSize) eval(c *ddbEvalCtx) (any, bool) {
	v, ok := ddbResolvePath(c.item, o.path, c.names)
	if !ok {
		return nil, false
	}
	return map[string]any{"N": strconv.Itoa(ddbAttrSize(v))}, true
}

// ── tokenizer ──────────────────────────────────────────────────────────────

type ddbTokKind int

const (
	ddbTokEOF ddbTokKind = iota
	ddbTokLParen
	ddbTokRParen
	ddbTokComma
	ddbTokOp
	ddbTokAnd
	ddbTokOr
	ddbTokNot
	ddbTokBetween
	ddbTokIn
	ddbTokWord
)

type ddbExprTok struct {
	kind ddbTokKind
	text string
}

func ddbExprTokenize(s string) []ddbExprTok {
	var toks []ddbExprTok
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case ' ', '\t', '\n':
			i++
		case '(':
			toks = append(toks, ddbExprTok{ddbTokLParen, "("})
			i++
		case ')':
			toks = append(toks, ddbExprTok{ddbTokRParen, ")"})
			i++
		case ',':
			toks = append(toks, ddbExprTok{ddbTokComma, ","})
			i++
		case '=', '<', '>':
			op := string(c)
			if i+1 < len(s) && (s[i+1] == '=' || s[i+1] == '>') {
				op += string(s[i+1])
				i++
			}
			toks = append(toks, ddbExprTok{ddbTokOp, op})
			i++
		default:
			start := i
			for i < len(s) {
				ch := s[i]
				if ch == ' ' || ch == '\t' || ch == '\n' || ch == '(' || ch == ')' || ch == ',' ||
					ch == '=' || ch == '<' || ch == '>' {
					break
				}
				i++
			}
			word := s[start:i]
			switch strings.ToUpper(word) {
			case "AND":
				toks = append(toks, ddbExprTok{ddbTokAnd, word})
			case "OR":
				toks = append(toks, ddbExprTok{ddbTokOr, word})
			case "NOT":
				toks = append(toks, ddbExprTok{ddbTokNot, word})
			case "BETWEEN":
				toks = append(toks, ddbExprTok{ddbTokBetween, word})
			case "IN":
				toks = append(toks, ddbExprTok{ddbTokIn, word})
			default:
				toks = append(toks, ddbExprTok{ddbTokWord, word})
			}
		}
	}
	return append(toks, ddbExprTok{ddbTokEOF, ""})
}

// ── parser ─────────────────────────────────────────────────────────────────

type ddbExprParser struct {
	toks []ddbExprTok
	pos  int
}

func (p *ddbExprParser) peek() ddbExprTok { return p.toks[p.pos] }
func (p *ddbExprParser) next() ddbExprTok { t := p.toks[p.pos]; p.pos++; return t }

func (p *ddbExprParser) parseOr() ddbCond {
	left := p.parseAnd()
	for p.peek().kind == ddbTokOr {
		p.next()
		left = ddbCondOr{left, p.parseAnd()}
	}
	return left
}

func (p *ddbExprParser) parseAnd() ddbCond {
	left := p.parseNot()
	for p.peek().kind == ddbTokAnd {
		p.next()
		left = ddbCondAnd{left, p.parseNot()}
	}
	return left
}

func (p *ddbExprParser) parseNot() ddbCond {
	if p.peek().kind == ddbTokNot {
		p.next()
		return ddbCondNot{p.parseNot()}
	}
	return p.parseTerm()
}

func (p *ddbExprParser) parseTerm() ddbCond {
	if p.peek().kind == ddbTokLParen {
		p.next()
		inner := p.parseOr()
		if p.peek().kind == ddbTokRParen {
			p.next()
		}
		return inner
	}
	// Boolean functions.
	if p.peek().kind == ddbTokWord {
		switch strings.ToLower(p.peek().text) {
		case "attribute_exists", "attribute_not_exists":
			fn := strings.ToLower(p.next().text)
			path := p.parseParenPath()
			return ddbCondFunc{name: fn, path: path}
		case "begins_with", "contains", "attribute_type":
			fn := strings.ToLower(p.next().text)
			path, arg := p.parseParenPathArg()
			return ddbCondFunc{name: fn, path: path, arg: arg}
		}
	}
	// operand <comparator|BETWEEN|IN> ...
	left := p.parseOperand()
	switch p.peek().kind {
	case ddbTokOp:
		op := p.next().text
		return ddbCondCompare{l: left, r: p.parseOperand(), op: op}
	case ddbTokBetween:
		p.next()
		lo := p.parseOperand()
		if p.peek().kind == ddbTokAnd {
			p.next()
		}
		hi := p.parseOperand()
		return ddbCondBetween{v: left, lo: lo, hi: hi}
	case ddbTokIn:
		p.next()
		var list []ddbOperand
		if p.peek().kind == ddbTokLParen {
			p.next()
			for p.peek().kind != ddbTokRParen && p.peek().kind != ddbTokEOF {
				list = append(list, p.parseOperand())
				if p.peek().kind == ddbTokComma {
					p.next()
				}
			}
			if p.peek().kind == ddbTokRParen {
				p.next()
			}
		}
		return ddbCondIn{v: left, list: list}
	}
	return ddbCondFalse{}
}

func (p *ddbExprParser) parseOperand() ddbOperand {
	if p.peek().kind == ddbTokWord && strings.ToLower(p.peek().text) == "size" {
		// size(path)
		p.next()
		path := p.parseParenPath()
		return ddbOperandSize{path: path}
	}
	if p.peek().kind != ddbTokWord {
		if p.peek().kind != ddbTokEOF {
			p.next()
		}
		return ddbOperandValue{ref: ""}
	}
	w := p.next().text
	if strings.HasPrefix(w, ":") {
		return ddbOperandValue{ref: w}
	}
	return ddbOperandPath{path: w}
}

// parseParenPath consumes "( path )" and returns the path text.
func (p *ddbExprParser) parseParenPath() string {
	if p.peek().kind == ddbTokLParen {
		p.next()
	}
	path := ""
	if p.peek().kind == ddbTokWord {
		path = p.next().text
	}
	if p.peek().kind == ddbTokRParen {
		p.next()
	}
	return path
}

// parseParenPathArg consumes "( path , operand )".
func (p *ddbExprParser) parseParenPathArg() (string, ddbOperand) {
	if p.peek().kind == ddbTokLParen {
		p.next()
	}
	path := ""
	if p.peek().kind == ddbTokWord {
		path = p.next().text
	}
	if p.peek().kind == ddbTokComma {
		p.next()
	}
	arg := p.parseOperand()
	if p.peek().kind == ddbTokRParen {
		p.next()
	}
	return path, arg
}

// ── path / attribute-value helpers ─────────────────────────────────────────

// ddbResolvePath walks a document path ("a.b[0].c", names #aliased) into the
// item, descending through M (map) and L (list) attribute values.
func ddbResolvePath(item map[string]any, path string, names map[string]string) (any, bool) {
	segs := ddbSplitPath(path)
	if len(segs) == 0 {
		return nil, false
	}
	cur, ok := item[ddbResolveAttrName(segs[0], names)]
	if !ok {
		return nil, false
	}
	for _, seg := range segs[1:] {
		av, isMap := cur.(map[string]any)
		if !isMap {
			return nil, false
		}
		if strings.HasPrefix(seg, "[") {
			idx, err := strconv.Atoi(strings.Trim(seg, "[]"))
			if err != nil {
				return nil, false
			}
			lst, ok := av["L"].([]any)
			if !ok || idx < 0 || idx >= len(lst) {
				return nil, false
			}
			cur = lst[idx]
			continue
		}
		m, ok := av["M"].(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[ddbResolveAttrName(seg, names)]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// ddbSplitPath splits "a.b[0].c" into ["a","b","[0]","c"].
func ddbSplitPath(path string) []string {
	var segs []string
	cur := strings.Builder{}
	flush := func() {
		if cur.Len() > 0 {
			segs = append(segs, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '.':
			flush()
		case '[':
			flush()
			j := strings.IndexByte(path[i:], ']')
			if j < 0 {
				return segs
			}
			segs = append(segs, path[i:i+j+1])
			i += j
		default:
			cur.WriteByte(path[i])
		}
	}
	flush()
	return segs
}

// ddbAttrTypeCode returns the DynamoDB type code (S/N/B/BOOL/NULL/M/L/SS/NS/BS)
// of an attribute value, for attribute_type().
func ddbAttrTypeCode(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	for _, code := range []string{"S", "N", "B", "BOOL", "NULL", "M", "L", "SS", "NS", "BS"} {
		if _, ok := m[code]; ok {
			return code
		}
	}
	return ""
}

// ddbAttrSize returns the size() of an attribute value: string/binary length,
// or element count for sets, lists and maps.
func ddbAttrSize(v any) int {
	m, ok := v.(map[string]any)
	if !ok {
		return 0
	}
	if s, ok := m["S"].(string); ok {
		return len(s)
	}
	if b, ok := m["B"].(string); ok {
		return len(b)
	}
	if l, ok := m["L"].([]any); ok {
		return len(l)
	}
	if mm, ok := m["M"].(map[string]any); ok {
		return len(mm)
	}
	for _, code := range []string{"SS", "NS", "BS"} {
		if set, ok := m[code].([]any); ok {
			return len(set)
		}
	}
	return 0
}
