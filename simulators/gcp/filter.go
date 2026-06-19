package main

import (
	"strconv"
	"strings"
)

// GCP list `filter` expression language — a faithful subset of AIP-160 / the
// Google API "filter" query parameter grammar:
//
//	filter      = expression
//	expression  = sequence { OR sequence }            (OR lowest precedence)
//	sequence    = factor { [AND] factor }             (AND explicit OR implicit/adjacent)
//	factor      = [ NOT | "-" ] term
//	term        = "(" expression ")" | comparison | restriction
//	comparison  = member operator value               operator ∈ = != < <= > >= :
//	restriction = member                              (bare member → truthy/has)
//	member      = name { "." name }                   (dotted field path)
//	value       = STRING | NUMBER | BOOL | name
//
// Evaluated against a resource rendered as a map[string]any (its JSON form), so
// it works for any resource type. A parse error yields a match-everything filter
// (the sim never rejects a list on a filter it can't model).

type gcpFilterNode interface{ eval(m map[string]any) bool }

type gcpFilterTrue struct{}

func (gcpFilterTrue) eval(map[string]any) bool { return true }

type gcpFilterOr struct{ l, r gcpFilterNode }

func (n gcpFilterOr) eval(m map[string]any) bool { return n.l.eval(m) || n.r.eval(m) }

type gcpFilterAnd struct{ l, r gcpFilterNode }

func (n gcpFilterAnd) eval(m map[string]any) bool { return n.l.eval(m) && n.r.eval(m) }

type gcpFilterNot struct{ inner gcpFilterNode }

func (n gcpFilterNot) eval(m map[string]any) bool { return !n.inner.eval(m) }

type gcpFilterCmp struct {
	field, op, value string
}

func (n gcpFilterCmp) eval(m map[string]any) bool {
	actual, present := gcpFieldLookup(m, n.field)
	switch n.op {
	case "":
		// Bare restriction: the member exists and is truthy/non-empty.
		return present && actual != "" && actual != "false" && actual != "0"
	case "=":
		return actual == n.value
	case "!=":
		return actual != n.value
	case ":":
		// "has": substring for scalars, key/element presence for maps/lists.
		if n.value == "*" {
			return present
		}
		return strings.Contains(actual, n.value)
	case ">", "<", ">=", "<=":
		return gcpFilterNumCompare(actual, n.op, n.value)
	}
	return true
}

// ── tokenizer ──────────────────────────────────────────────────────────────

type gcpTokKind int

const (
	tokEOF gcpTokKind = iota
	tokLParen
	tokRParen
	tokAnd
	tokOr
	tokNot
	tokOp     // = != < <= > >= :
	tokWord   // identifier / field path / bare value
	tokString // quoted value
)

type gcpTok struct {
	kind gcpTokKind
	text string
}

func gcpTokenize(s string) []gcpTok {
	var toks []gcpTok
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n':
			i++
		case c == '(':
			toks = append(toks, gcpTok{tokLParen, "("})
			i++
		case c == ')':
			toks = append(toks, gcpTok{tokRParen, ")"})
			i++
		case c == '-' && (len(toks) == 0 || toks[len(toks)-1].kind == tokLParen || toks[len(toks)-1].kind == tokAnd || toks[len(toks)-1].kind == tokOr || toks[len(toks)-1].kind == tokNot):
			toks = append(toks, gcpTok{tokNot, "-"})
			i++
		case c == '=' || c == '!' || c == '<' || c == '>' || c == ':':
			op := string(c)
			if c != ':' && i+1 < len(s) && s[i+1] == '=' {
				op += "="
				i++
			}
			toks = append(toks, gcpTok{tokOp, op})
			i++
		case c == '"' || c == '\'':
			quote := c
			i++
			start := i
			var b strings.Builder
			for i < len(s) && s[i] != quote {
				if s[i] == '\\' && i+1 < len(s) {
					i++
				}
				b.WriteByte(s[i])
				i++
			}
			_ = start
			if i < len(s) {
				i++ // closing quote
			}
			toks = append(toks, gcpTok{tokString, b.String()})
		default:
			start := i
			for i < len(s) {
				ch := s[i]
				if ch == ' ' || ch == '\t' || ch == '\n' || ch == '(' || ch == ')' ||
					ch == '=' || ch == '!' || ch == '<' || ch == '>' || ch == ':' {
					break
				}
				i++
			}
			word := s[start:i]
			switch strings.ToUpper(word) {
			case "AND":
				toks = append(toks, gcpTok{tokAnd, word})
			case "OR":
				toks = append(toks, gcpTok{tokOr, word})
			case "NOT":
				toks = append(toks, gcpTok{tokNot, word})
			default:
				toks = append(toks, gcpTok{tokWord, word})
			}
		}
	}
	toks = append(toks, gcpTok{tokEOF, ""})
	return toks
}

// ── parser (recursive descent) ─────────────────────────────────────────────

type gcpFilterParser struct {
	toks []gcpTok
	pos  int
}

func gcpParseFilterExpr(s string) gcpFilterNode {
	s = strings.TrimSpace(s)
	if s == "" {
		return gcpFilterTrue{}
	}
	p := &gcpFilterParser{toks: gcpTokenize(s)}
	node := p.parseOr()
	if node == nil {
		return gcpFilterTrue{}
	}
	return node
}

func (p *gcpFilterParser) peek() gcpTok { return p.toks[p.pos] }
func (p *gcpFilterParser) next() gcpTok { t := p.toks[p.pos]; p.pos++; return t }

func (p *gcpFilterParser) parseOr() gcpFilterNode {
	left := p.parseAnd()
	for p.peek().kind == tokOr {
		p.next()
		right := p.parseAnd()
		left = gcpFilterOr{left, right}
	}
	return left
}

func (p *gcpFilterParser) parseAnd() gcpFilterNode {
	left := p.parseFactor()
	for {
		k := p.peek().kind
		if k == tokAnd {
			p.next()
			left = gcpFilterAnd{left, p.parseFactor()}
			continue
		}
		// Implicit AND: another factor starts (word, "(", NOT) with no OR/AND between.
		if k == tokWord || k == tokString || k == tokLParen || k == tokNot {
			left = gcpFilterAnd{left, p.parseFactor()}
			continue
		}
		break
	}
	return left
}

func (p *gcpFilterParser) parseFactor() gcpFilterNode {
	if p.peek().kind == tokNot {
		p.next()
		return gcpFilterNot{p.parseFactor()}
	}
	if p.peek().kind == tokLParen {
		p.next()
		inner := p.parseOr()
		if p.peek().kind == tokRParen {
			p.next()
		}
		return inner
	}
	return p.parseComparison()
}

func (p *gcpFilterParser) parseComparison() gcpFilterNode {
	if p.peek().kind != tokWord && p.peek().kind != tokString {
		// Unexpected token — consume it and treat as match-all so we never hang.
		if p.peek().kind != tokEOF {
			p.next()
		}
		return gcpFilterTrue{}
	}
	field := p.next().text
	if p.peek().kind == tokOp {
		op := p.next().text
		value := ""
		if p.peek().kind == tokWord || p.peek().kind == tokString {
			value = p.next().text
		}
		return gcpFilterCmp{field: field, op: op, value: value}
	}
	// Bare member restriction.
	return gcpFilterCmp{field: field, op: ""}
}

// ── evaluation helpers ─────────────────────────────────────────────────────

func gcpFieldLookup(m map[string]any, path string) (value string, present bool) {
	var cur any = m
	for _, seg := range strings.Split(path, ".") {
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
	return gcpScalarString(cur), true
}

func gcpScalarString(v any) string {
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
		return gcpFieldString(map[string]any{"_": t}, "_")
	}
}

func gcpFilterNumCompare(a, op, b string) bool {
	af, aerr := strconv.ParseFloat(a, 64)
	bf, berr := strconv.ParseFloat(b, 64)
	if aerr != nil || berr != nil {
		switch op {
		case ">":
			return a > b
		case "<":
			return a < b
		case ">=":
			return a >= b
		case "<=":
			return a <= b
		}
		return false
	}
	switch op {
	case ">":
		return af > bf
	case "<":
		return af < bf
	case ">=":
		return af >= bf
	case "<=":
		return af <= bf
	}
	return false
}
