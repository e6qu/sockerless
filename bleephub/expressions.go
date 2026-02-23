package bleephub

import (
	"strings"
	"unicode"
)

// ExprContext holds the evaluation context for GitHub Actions expressions.
type ExprContext struct {
	// DepResults maps job key → result string (e.g., "success", "failure", "cancelled", "skipped")
	DepResults map[string]string
	// Values holds context data for dot-notation access (e.g., "github.event_name" → "push")
	Values map[string]string
	// WorkflowCancelled indicates the workflow was cancelled
	WorkflowCancelled bool
}

// EvalExpr evaluates a GitHub Actions expression string.
// Supports: success(), failure(), always(), cancelled(),
// string comparison (==, !=), boolean operators (&&, ||, !),
// context access (github.event_name), and parentheses.
func EvalExpr(expr string, ctx *ExprContext) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}
	// Strip ${{ ... }} wrapper if present
	if strings.HasPrefix(expr, "${{") && strings.HasSuffix(expr, "}}") {
		expr = strings.TrimSpace(expr[3 : len(expr)-2])
	}
	p := &exprParser{input: expr, ctx: ctx}
	result := p.parseOr()
	return result
}

type exprParser struct {
	input string
	pos   int
	ctx   *ExprContext
}

func (p *exprParser) skipWhitespace() {
	for p.pos < len(p.input) && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
}

func (p *exprParser) peek() byte {
	p.skipWhitespace()
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *exprParser) parseOr() bool {
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

func (p *exprParser) parseAnd() bool {
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

func (p *exprParser) parseComparison() bool {
	leftVal := p.parseUnary()
	p.skipWhitespace()

	if p.pos+1 < len(p.input) {
		op := p.input[p.pos : p.pos+2]
		if op == "==" || op == "!=" {
			p.pos += 2
			rightVal := p.parseUnary()
			if op == "==" {
				return leftVal == rightVal
			}
			return leftVal != rightVal
		}
	}

	// Truthy: non-empty string, "true" → true; "false", "" → false
	return isTruthy(leftVal)
}

func (p *exprParser) parseUnary() string {
	p.skipWhitespace()
	if p.pos < len(p.input) && p.input[p.pos] == '!' {
		p.pos++
		val := p.parsePrimary()
		if isTruthy(val) {
			return "false"
		}
		return "true"
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() string {
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

	// String literal
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

	// Identifier or function call
	start := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == '(' || c == ')' || c == '!' || c == '=' || c == '&' || c == '|' || c == '\'' || unicode.IsSpace(rune(c)) {
			break
		}
		p.pos++
	}
	ident := p.input[start:p.pos]

	// Function call?
	p.skipWhitespace()
	if p.pos < len(p.input) && p.input[p.pos] == '(' {
		p.pos++ // skip (
		p.skipWhitespace()
		if p.pos < len(p.input) && p.input[p.pos] == ')' {
			p.pos++ // skip )
		}
		return p.evalFunction(ident)
	}

	// Boolean literals
	if ident == "true" {
		return "true"
	}
	if ident == "false" {
		return "false"
	}

	// Context access (dot-notation)
	if p.ctx != nil && p.ctx.Values != nil {
		if val, ok := p.ctx.Values[ident]; ok {
			return val
		}
	}

	return ident
}

func (p *exprParser) evalFunction(name string) string {
	if p.ctx == nil {
		return "false"
	}

	switch name {
	case "success":
		// success() is true if no dep failed (or no deps)
		for _, r := range p.ctx.DepResults {
			if r != "success" && r != "skipped" {
				return "false"
			}
		}
		return "true"
	case "failure":
		for _, r := range p.ctx.DepResults {
			if r == "failure" {
				return "true"
			}
		}
		return "false"
	case "always":
		return "true"
	case "cancelled":
		if p.ctx.WorkflowCancelled {
			return "true"
		}
		return "false"
	}
	return "false"
}

func isTruthy(val string) bool {
	return val != "" && val != "false" && val != "0"
}

// ExprContainsStatusFunction checks if an expression contains always() or failure()
// which would override default dependency-failure skip behavior.
func ExprContainsStatusFunction(expr string) (hasAlways, hasFailure bool) {
	lower := strings.ToLower(expr)
	hasAlways = strings.Contains(lower, "always()")
	hasFailure = strings.Contains(lower, "failure()")
	return
}
