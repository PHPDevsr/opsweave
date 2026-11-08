package workflow

import "strings"

// ValidateCondition checks the bounded boolean grammar and returns referenced
// step IDs in encounter order. It never evaluates a condition or invokes code.
func ValidateCondition(source string) ([]string, error) {
	if len(source) == 0 || len(source) > 4096 {
		return nil, &ValidationError{Code: "invalid_condition", Path: "if"}
	}
	p := conditionParser{source: source}
	if !p.expression(0) || p.space() != len(source) {
		return nil, &ValidationError{Code: "invalid_condition", Path: "if"}
	}
	return p.references, nil
}

type conditionParser struct {
	source     string
	pos        int
	tokens     int
	references []string
}

func (p *conditionParser) space() int {
	for p.pos < len(p.source) && strings.ContainsRune(" \t\n", rune(p.source[p.pos])) {
		p.pos++
	}
	return p.pos
}

func (p *conditionParser) take(token string) bool {
	p.space()
	if p.tokens >= 1024 || !strings.HasPrefix(p.source[p.pos:], token) {
		return false
	}
	p.pos += len(token)
	p.tokens++
	return true
}

func (p *conditionParser) expression(depth int) bool {
	if !p.and(depth) {
		return false
	}
	for p.take("||") {
		if !p.and(depth) {
			return false
		}
	}
	return true
}

func (p *conditionParser) and(depth int) bool {
	if !p.unary(depth) {
		return false
	}
	for p.take("&&") {
		if !p.unary(depth) {
			return false
		}
	}
	return true
}

func (p *conditionParser) unary(depth int) bool {
	if p.take("!") {
		return depth < 32 && p.unary(depth+1)
	}
	if p.take("(") {
		return depth < 32 && p.expression(depth+1) && p.take(")")
	}
	if p.take("true") || p.take("false") {
		return true
	}
	if !p.take("steps.") {
		return false
	}
	start := p.pos
	for p.pos < len(p.source) && (p.source[p.pos] >= 'a' && p.source[p.pos] <= 'z' || p.source[p.pos] >= '0' && p.source[p.pos] <= '9' || p.source[p.pos] == '-') {
		p.pos++
	}
	id := p.source[start:p.pos]
	if !identifier(id) || !p.take(".") {
		return false
	}
	for _, state := range []string{"success", "failed", "skipped", "cancelled", "timed_out"} {
		if p.take(state) {
			p.references = append(p.references, id)
			return true
		}
	}
	return false
}
