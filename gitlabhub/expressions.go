package gitlabhub

import (
	"regexp"
	"strings"
	"unicode"
)

// nullSentinel is a special value representing an undefined variable.
// Using a non-printable string that cannot appear in normal CI values.
const nullSentinel = "\x00NULL"

// ExprContext holds CI variables for expression evaluation.
type ExprContext struct {
	Variables map[string]string
}

// EvalGitLabExpr evaluates a GitLab CI expression string.
// Supports: $VAR / ${VAR} expansion, == / != comparison,
// =~ / !~ regex matching with /pattern/ syntax,
// && / || boolean operators, parentheses, null checks.
func EvalGitLabExpr(expr string, ctx *ExprContext) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}
	p := &glExprParser{input: expr, ctx: ctx}
	result := p.parseOr()
	return result
}

type glExprParser struct {
	input string
	pos   int
	ctx   *ExprContext
}

func (p *glExprParser) skipWhitespace() {
	for p.pos < len(p.input) && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
}

// parseOr handles || (logical OR).
func (p *glExprParser) parseOr() bool {
	left := p.parseAnd()
	for {
		p.skipWhitespace()
		if p.pos+1 < len(p.input) && p.input[p.pos:p.pos+2] == "||" {
			p.pos += 2
			right := p.parseAnd()
			left = left || right
		} else {
			break
		}
	}
	return left
}

// parseAnd handles && (logical AND).
func (p *glExprParser) parseAnd() bool {
	left := p.parseComparison()
	for {
		p.skipWhitespace()
		if p.pos+1 < len(p.input) && p.input[p.pos:p.pos+2] == "&&" {
			p.pos += 2
			right := p.parseComparison()
			left = left && right
		} else {
			break
		}
	}
	return left
}

// parseComparison handles ==, !=, =~, !~ operators.
func (p *glExprParser) parseComparison() bool {
	leftVal := p.parseUnary()
	p.skipWhitespace()

	if p.pos+1 < len(p.input) {
		op := p.input[p.pos : p.pos+2]
		switch op {
		case "==":
			p.pos += 2
			rightVal := p.parseUnary()
			return leftVal == rightVal
		case "!=":
			p.pos += 2
			rightVal := p.parseUnary()
			return leftVal != rightVal
		case "=~":
			p.pos += 2
			pattern := p.parseRegex()
			if leftVal == nullSentinel {
				return false
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return false
			}
			return re.MatchString(leftVal)
		case "!~":
			p.pos += 2
			pattern := p.parseRegex()
			if leftVal == nullSentinel {
				return true
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return true
			}
			return !re.MatchString(leftVal)
		}
	}

	// No comparison operator: truthy check
	return glIsTruthy(leftVal)
}

// parseRegex parses a /pattern/ regex literal.
func (p *glExprParser) parseRegex() string {
	p.skipWhitespace()
	if p.pos >= len(p.input) || p.input[p.pos] != '/' {
		// Not a regex literal, try to parse as a primary value
		return p.parsePrimary()
	}
	p.pos++ // skip opening /
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != '/' {
		if p.input[p.pos] == '\\' && p.pos+1 < len(p.input) {
			p.pos++ // skip escaped character
		}
		p.pos++
	}
	pattern := p.input[start:p.pos]
	if p.pos < len(p.input) {
		p.pos++ // skip closing /
	}
	return pattern
}

// parseUnary handles ! (negation).
func (p *glExprParser) parseUnary() string {
	p.skipWhitespace()
	if p.pos < len(p.input) && p.input[p.pos] == '!' {
		// Make sure it's not != or !~
		if p.pos+1 < len(p.input) && (p.input[p.pos+1] == '=' || p.input[p.pos+1] == '~') {
			// This is a comparison operator, not negation — return to caller
			return p.parsePrimary()
		}
		p.pos++ // skip !
		val := p.parsePrimary()
		if glIsTruthy(val) {
			return "false"
		}
		return "true"
	}
	return p.parsePrimary()
}

// parsePrimary handles parentheses, string literals, variables, null, true/false.
func (p *glExprParser) parsePrimary() string {
	p.skipWhitespace()
	if p.pos >= len(p.input) {
		return ""
	}

	ch := p.input[p.pos]

	// Parenthesized expression
	if ch == '(' {
		p.pos++
		result := p.parseOr()
		p.skipWhitespace()
		if p.pos < len(p.input) && p.input[p.pos] == ')' {
			p.pos++
		}
		if result {
			return "true"
		}
		return "false"
	}

	// String literal (double-quoted)
	if ch == '"' {
		p.pos++
		start := p.pos
		for p.pos < len(p.input) && p.input[p.pos] != '"' {
			if p.input[p.pos] == '\\' && p.pos+1 < len(p.input) {
				p.pos++ // skip escaped character
			}
			p.pos++
		}
		val := p.input[start:p.pos]
		if p.pos < len(p.input) {
			p.pos++ // skip closing quote
		}
		return val
	}

	// Single-quoted string literal (for compatibility)
	if ch == '\'' {
		p.pos++
		start := p.pos
		for p.pos < len(p.input) && p.input[p.pos] != '\'' {
			p.pos++
		}
		val := p.input[start:p.pos]
		if p.pos < len(p.input) {
			p.pos++ // skip closing quote
		}
		return val
	}

	// Variable reference: $VAR or ${VAR}
	if ch == '$' {
		p.pos++ // skip $
		if p.pos < len(p.input) && p.input[p.pos] == '{' {
			// ${VAR} syntax
			p.pos++ // skip {
			start := p.pos
			for p.pos < len(p.input) && p.input[p.pos] != '}' {
				p.pos++
			}
			varName := p.input[start:p.pos]
			if p.pos < len(p.input) {
				p.pos++ // skip }
			}
			return p.resolveVar(varName)
		}
		// $VAR syntax
		start := p.pos
		for p.pos < len(p.input) {
			c := p.input[p.pos]
			if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				p.pos++
			} else {
				break
			}
		}
		varName := p.input[start:p.pos]
		return p.resolveVar(varName)
	}

	// Keyword or bare identifier
	start := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == '(' || c == ')' || c == '!' || c == '=' || c == '&' ||
			c == '|' || c == '"' || c == '\'' || c == '~' || c == '$' ||
			unicode.IsSpace(rune(c)) {
			break
		}
		p.pos++
	}
	ident := p.input[start:p.pos]

	switch ident {
	case "null":
		return nullSentinel
	case "true":
		return "true"
	case "false":
		return "false"
	}

	return ident
}

// resolveVar looks up a variable name in the context.
func (p *glExprParser) resolveVar(name string) string {
	if p.ctx != nil && p.ctx.Variables != nil {
		if val, ok := p.ctx.Variables[name]; ok {
			return val
		}
	}
	return nullSentinel
}

// glIsTruthy returns true for non-empty, non-null, non-"false", non-"0" values.
func glIsTruthy(val string) bool {
	return val != "" && val != nullSentinel && val != "false" && val != "0"
}
