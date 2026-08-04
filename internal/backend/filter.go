
package backend

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokNumber
	tokString
	tokIdent
	tokLParen
	tokRParen
	tokEq
	tokNe
	tokLt
	tokGt
	tokLe
	tokGe
	tokAnd
	tokOr
	tokAmp
	tokLike
)

type token struct {
	kind tokenKind
	val  string
	num  int64
}

type lexer struct {
	input string
	pos   int
	start int
}

func newLexer(input string) *lexer {
	return &lexer{input: input}
}

func (l *lexer) next() (token, error) {
	l.start = l.pos
	l.skipSpace()
	if l.pos >= len(l.input) {
		return token{kind: tokEOF}, nil
	}

	ch := l.input[l.pos]
	switch ch {
	case '(':
		l.pos++
		return token{kind: tokLParen, val: "("}, nil
	case ')':
		l.pos++
		return token{kind: tokRParen, val: ")"}, nil
	case '&':
		l.pos++
		return token{kind: tokAmp, val: "&"}, nil
	case '=':
		l.pos++
		return token{kind: tokEq, val: "="}, nil
	case '!':
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '=' {
			l.pos += 2
			return token{kind: tokNe, val: "!="}, nil
		}
		return token{}, fmt.Errorf("unexpected character %q at %d", ch, l.pos)
	case '<':
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '=' {
			l.pos += 2
			return token{kind: tokLe, val: "<="}, nil
		}
		l.pos++
		return token{kind: tokLt, val: "<"}, nil
	case '>':
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '=' {
			l.pos += 2
			return token{kind: tokGe, val: ">="}, nil
		}
		l.pos++
		return token{kind: tokGt, val: ">"}, nil
	case '\'':
		return l.readQuoted('\'')
	case '"':
		return l.readQuoted('"')
	}

	if ch == '-' && l.pos+1 < len(l.input) && unicode.IsDigit(rune(l.input[l.pos+1])) {
		return l.readNumber(true)
	}
	if unicode.IsDigit(rune(ch)) {
		return l.readNumber(false)
	}
	if unicode.IsLetter(rune(ch)) || ch == '_' {
		return l.readIdent()
	}

	return token{}, fmt.Errorf("unexpected character %q at %d", ch, l.pos)
}

func (l *lexer) skipSpace() {
	for l.pos < len(l.input) && unicode.IsSpace(rune(l.input[l.pos])) {
		l.pos++
	}
}

func (l *lexer) readQuoted(quote byte) (token, error) {
	l.pos++
	start := l.pos
	for l.pos < len(l.input) && l.input[l.pos] != quote {
		l.pos++
	}
	if l.pos >= len(l.input) {
		return token{}, fmt.Errorf("unterminated string at %d", start)
	}
	val := l.input[start:l.pos]
	l.pos++
	return token{kind: tokString, val: val}, nil
}

func (l *lexer) readNumber(negative bool) (token, error) {
	start := l.pos
	if negative {
		l.pos++
	}
	for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
		l.pos++
	}
	raw := l.input[start:l.pos]
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return token{}, fmt.Errorf("invalid number %q", raw)
	}
	return token{kind: tokNumber, val: raw, num: n}, nil
}

func (l *lexer) mark() int {
	return l.start
}

func (l *lexer) reset(pos int) {
	l.pos = pos
}

func (l *lexer) readIdent() (token, error) {
	start := l.pos
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)) || ch == '_' {
			l.pos++
			continue
		}
		break
	}
	val := l.input[start:l.pos]
	lower := strings.ToLower(val)
	switch lower {
	case "and":
		return token{kind: tokAnd, val: lower}, nil
	case "or":
		return token{kind: tokOr, val: lower}, nil
	case "like":
		return token{kind: tokLike, val: "LIKE"}, nil
	}
	return token{kind: tokIdent, val: val}, nil
}

// CompiledFilter is a parsed GameSpy filter ready for repeated evaluation.
type CompiledFilter struct {
	root boolExpr
}

// CompileFilter parses a GameSpy server-browser filter for repeated evaluation.
func CompileFilter(filter string) (*CompiledFilter, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return &CompiledFilter{root: trueExpr{}}, nil
	}
	p, err := newCompileParser(filter)
	if err != nil {
		return nil, err
	}
	root, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.cur.kind != tokEOF {
		return nil, fmt.Errorf("unexpected token %q after expression", p.cur.val)
	}
	return &CompiledFilter{root: root}, nil
}

// Match evaluates the compiled filter against server fields.
func (c *CompiledFilter) Match(server map[string]string) (bool, error) {
	if c == nil || c.root == nil {
		return true, nil
	}
	return c.root.eval(server)
}

// MatchFilter evaluates a GameSpy server-browser filter against server fields.
func MatchFilter(server map[string]string, filter string) (bool, error) {
	c, err := CompileFilter(filter)
	if err != nil {
		return false, err
	}
	return c.Match(server)
}

type valueKind int

const (
	valueNumber valueKind = iota
	valueString
)

type value struct {
	kind valueKind
	num  int64
	str  string
	raw  string
}

func (v value) asInt() (int64, error) {
	if v.kind == valueNumber {
		return v.num, nil
	}
	if v.kind == valueString {
		n, err := strconv.ParseInt(v.str, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot convert %q to number", v.str)
		}
		return n, nil
	}
	return 0, fmt.Errorf("cannot convert %q to number", v.raw)
}

func (v value) asString() string {
	switch v.kind {
	case valueString:
		return v.str
	case valueNumber:
		return strconv.FormatInt(v.num, 10)
	default:
		return v.raw
	}
}

type boolExpr interface {
	eval(server map[string]string) (bool, error)
}

type trueExpr struct{}

func (trueExpr) eval(map[string]string) (bool, error) { return true, nil }

type orExpr struct {
	left, right boolExpr
}

func (e orExpr) eval(server map[string]string) (bool, error) {
	left, err := e.left.eval(server)
	if err != nil {
		return false, err
	}
	if left {
		return true, nil
	}
	return e.right.eval(server)
}

type andExpr struct {
	left, right boolExpr
}

func (e andExpr) eval(server map[string]string) (bool, error) {
	left, err := e.left.eval(server)
	if err != nil {
		return false, err
	}
	if !left {
		return false, nil
	}
	return e.right.eval(server)
}

type comparisonExpr struct {
	op          tokenKind
	left, right valueExpr
}

func (e comparisonExpr) eval(server map[string]string) (bool, error) {
	left, err := e.left.eval(server)
	if err != nil {
		return false, err
	}
	right, err := e.right.eval(server)
	if err != nil {
		return false, err
	}
	return compareValues(e.op, left, right)
}

type valueExpr interface {
	eval(server map[string]string) (value, error)
}

type numberExpr struct {
	num int64
	raw string
}

func (e numberExpr) eval(map[string]string) (value, error) {
	return value{kind: valueNumber, num: e.num, raw: e.raw}, nil
}

type stringExpr struct {
	str string
}

func (e stringExpr) eval(map[string]string) (value, error) {
	return value{kind: valueString, str: e.str, raw: e.str}, nil
}

type fieldExpr struct {
	name string
}

func (e fieldExpr) eval(server map[string]string) (value, error) {
	fieldVal, ok := server[e.name]
	if !ok {
		return value{}, fmt.Errorf("unknown field %q", e.name)
	}
	if n, err := strconv.ParseInt(fieldVal, 10, 64); err == nil {
		return value{kind: valueNumber, num: n, raw: fieldVal}, nil
	}
	return value{kind: valueString, str: fieldVal, raw: fieldVal}, nil
}

type bitwiseAndExpr struct {
	left, right valueExpr
}

func (e bitwiseAndExpr) eval(server map[string]string) (value, error) {
	left, err := e.left.eval(server)
	if err != nil {
		return value{}, err
	}
	right, err := e.right.eval(server)
	if err != nil {
		return value{}, err
	}
	ln, err := left.asInt()
	if err != nil {
		return value{}, err
	}
	rn, err := right.asInt()
	if err != nil {
		return value{}, err
	}
	return value{kind: valueNumber, num: ln & rn}, nil
}

type groupedValueExpr struct {
	inner valueExpr
}

func (e groupedValueExpr) eval(server map[string]string) (value, error) {
	return e.inner.eval(server)
}

type compileParser struct {
	lex *lexer
	cur token
}

func newCompileParser(filter string) (*compileParser, error) {
	p := &compileParser{lex: newLexer(filter)}
	tok, err := p.lex.next()
	if err != nil {
		return nil, err
	}
	p.cur = tok
	return p, nil
}

func (p *compileParser) advance() error {
	tok, err := p.lex.next()
	if err != nil {
		return err
	}
	p.cur = tok
	return nil
}

func (p *compileParser) retokenize() error {
	tok, err := p.lex.next()
	if err != nil {
		return err
	}
	p.cur = tok
	return nil
}

func (p *compileParser) expect(kind tokenKind) error {
	if p.cur.kind != kind {
		return fmt.Errorf("expected token %d, got %d (%q)", kind, p.cur.kind, p.cur.val)
	}
	return p.advance()
}

func (p *compileParser) parseOr() (boolExpr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.cur.kind == tokOr {
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = orExpr{left: left, right: right}
	}
	return left, nil
}

func (p *compileParser) parseAnd() (boolExpr, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	for p.cur.kind == tokAnd {
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		left = andExpr{left: left, right: right}
	}
	return left, nil
}

func (p *compileParser) parseFactor() (boolExpr, error) {
	if p.cur.kind != tokLParen {
		return p.parseComparison()
	}

	mark := p.lex.mark()
	if err := p.advance(); err != nil {
		return nil, err
	}
	if _, err := p.parseBitwise(); err == nil && p.cur.kind == tokRParen {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if isCompareToken(p.cur.kind) {
			p.lex.reset(mark)
			if err := p.retokenize(); err != nil {
				return nil, err
			}
			return p.parseComparison()
		}
	}
	p.lex.reset(mark)
	if err := p.retokenize(); err != nil {
		return nil, err
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	inner, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if err := p.expect(tokRParen); err != nil {
		return nil, err
	}
	return inner, nil
}

func isCompareToken(kind tokenKind) bool {
	switch kind {
	case tokEq, tokNe, tokLt, tokGt, tokLe, tokGe, tokLike:
		return true
	default:
		return false
	}
}

func (p *compileParser) parseComparison() (boolExpr, error) {
	left, err := p.parseBitwise()
	if err != nil {
		return nil, err
	}
	if !isCompareToken(p.cur.kind) {
		return nil, fmt.Errorf("expected comparison, got %q", p.cur.val)
	}
	op := p.cur.kind
	if err := p.advance(); err != nil {
		return nil, err
	}
	right, err := p.parseBitwise()
	if err != nil {
		return nil, err
	}
	return comparisonExpr{op: op, left: left, right: right}, nil
}

func (p *compileParser) parseBitwise() (valueExpr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.cur.kind == tokAmp {
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = bitwiseAndExpr{left: left, right: right}
	}
	return left, nil
}

func (p *compileParser) parseUnary() (valueExpr, error) {
	if p.cur.kind == tokNumber && strings.HasPrefix(p.cur.val, "-") {
		v := numberExpr{num: p.cur.num, raw: p.cur.val}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return v, nil
	}
	return p.parsePrimary()
}

func (p *compileParser) parsePrimary() (valueExpr, error) {
	switch p.cur.kind {
	case tokNumber:
		v := numberExpr{num: p.cur.num, raw: p.cur.val}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return v, nil
	case tokString:
		v := stringExpr{str: p.cur.val}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return v, nil
	case tokIdent:
		name := p.cur.val
		if err := p.advance(); err != nil {
			return nil, err
		}
		return fieldExpr{name: name}, nil
	case tokLParen:
		if err := p.advance(); err != nil {
			return nil, err
		}
		inner, err := p.parseBitwise()
		if err != nil {
			return nil, err
		}
		if err := p.expect(tokRParen); err != nil {
			return nil, err
		}
		return groupedValueExpr{inner: inner}, nil
	default:
		return nil, fmt.Errorf("unexpected token %q", p.cur.val)
	}
}

func compareValues(op tokenKind, left, right value) (bool, error) {
	if op == tokLike {
		return strings.EqualFold(left.asString(), right.asString()), nil
	}

	ln, lErr := left.asInt()
	rn, rErr := right.asInt()
	if lErr == nil && rErr == nil {
		switch op {
		case tokEq:
			return ln == rn, nil
		case tokNe:
			return ln != rn, nil
		case tokLt:
			return ln < rn, nil
		case tokGt:
			return ln > rn, nil
		case tokLe:
			return ln <= rn, nil
		case tokGe:
			return ln >= rn, nil
		}
	}

	ls := left.asString()
	rs := right.asString()
	switch op {
	case tokEq:
		return ls == rs, nil
	case tokNe:
		return ls != rs, nil
	case tokLt:
		return ls < rs, nil
	case tokGt:
		return ls > rs, nil
	case tokLe:
		return ls <= rs, nil
	case tokGe:
		return ls >= rs, nil
	default:
		return false, fmt.Errorf("unknown comparison operator")
	}
}
