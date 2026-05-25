package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var clockNow = time.Now

type undefinedType struct{}

type intervalVal struct {
	years, months, days int
	dur                 time.Duration
}

// ── Tokens ───────────────────────────────────────────────────────────────────

type tokenKind int

const (
	tkNum      tokenKind = iota // numeric literal
	tkStr                       // string literal
	tkIdent                     // identifier
	tkPlus                      // +
	tkMinus                     // -
	tkStar                      // *
	tkSlash                     // /
	tkEq                        // ==
	tkNeq                       // !=
	tkLt                        // <
	tkGt                        // >
	tkLte                       // <=
	tkGte                       // >=
	tkAnd                       // &&
	tkOr                        // ||
	tkNot                       // !
	tkLParen                    // (
	tkRParen                    // )
	tkLBracket                  // [
	tkRBracket                  // ]
	tkDot                       // .
	tkComma                     // ,
	tkEOF                       // EOF
)

type token struct {
	kind   tokenKind
	sval   string
	numval float64
}

// ── Lexer ─────────────────────────────────────────────────────────────────────

type lexer struct {
	tokens []token
	pos    int
}

func lex(src string) (*lexer, error) {
	l := &lexer{}
	i := 0
	for i < len(src) {
		if unicode.IsSpace(rune(src[i])) {
			i++
			continue
		}
		c := src[i]

		// Numbers - must start with a digit (use 0.8 not .8)
		if c >= '0' && c <= '9' {
			j := i
			for j < len(src) && (src[j] >= '0' && src[j] <= '9' || src[j] == '.') {
				j++
			}
			f, err := strconv.ParseFloat(src[i:j], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid number %q", src[i:j])
			}
			l.tokens = append(l.tokens, token{kind: tkNum, numval: f})
			i = j
			continue
		}

		// Strings - double or single quoted, backslash escapes
		if c == '"' || c == '\'' {
			q := c
			i++
			var sb strings.Builder
			for i < len(src) && src[i] != q {
				if src[i] == '\\' && i+1 < len(src) {
					i++
					switch src[i] {
					case 'n':
						sb.WriteByte('\n')
					case 't':
						sb.WriteByte('\t')
					default:
						sb.WriteByte(src[i])
					}
				} else {
					sb.WriteByte(src[i])
				}
				i++
			}
			if i >= len(src) {
				return nil, fmt.Errorf("unterminated string literal")
			}
			l.tokens = append(l.tokens, token{kind: tkStr, sval: sb.String()})
			i++ // skip closing quote
			continue
		}

		// Identifiers and keywords
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			j := i
			for j < len(src) {
				ch := src[j]
				if ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
					j++
				} else {
					break
				}
			}
			l.tokens = append(l.tokens, token{kind: tkIdent, sval: src[i:j]})
			i = j
			continue
		}

		// Two-character operators
		if i+1 < len(src) {
			switch src[i : i+2] {
			case "==":
				l.tokens = append(l.tokens, token{kind: tkEq, sval: "=="})
				i += 2
				continue
			case "!=", "<>":
				l.tokens = append(l.tokens, token{kind: tkNeq, sval: src[i : i+2]})
				i += 2
				continue
			case "<=":
				l.tokens = append(l.tokens, token{kind: tkLte, sval: "<="})
				i += 2
				continue
			case ">=":
				l.tokens = append(l.tokens, token{kind: tkGte, sval: ">="})
				i += 2
				continue
			case "&&":
				l.tokens = append(l.tokens, token{kind: tkAnd, sval: "&&"})
				i += 2
				continue
			case "||":
				l.tokens = append(l.tokens, token{kind: tkOr, sval: "||"})
				i += 2
				continue
			}
		}

		// Single-character operators
		var k tokenKind
		switch c {
		case '+':
			k = tkPlus
		case '-':
			k = tkMinus
		case '*':
			k = tkStar
		case '/':
			k = tkSlash
		case '<':
			k = tkLt
		case '>':
			k = tkGt
		case '!':
			k = tkNot
		case '(':
			k = tkLParen
		case ')':
			k = tkRParen
		case '[':
			k = tkLBracket
		case ']':
			k = tkRBracket
		case '.':
			k = tkDot
		case ',':
			k = tkComma
		default:
			return nil, fmt.Errorf("unexpected character %q", c)
		}
		l.tokens = append(l.tokens, token{kind: k, sval: string(c)})
		i++
	}
	l.tokens = append(l.tokens, token{kind: tkEOF})
	return l, nil
}

func (l *lexer) peek() token {
	if l.pos < len(l.tokens) {
		return l.tokens[l.pos]
	}
	return token{kind: tkEOF}
}

func (l *lexer) next() token {
	t := l.peek()
	if l.pos < len(l.tokens) {
		l.pos++
	}
	return t
}

func (l *lexer) expect(k tokenKind) (token, error) {
	t := l.next()
	if t.kind != k {
		return t, fmt.Errorf("expected %d, got %q", k, t.sval)
	}
	return t, nil
}

// ── AST ──────────────────────────────────────────────────────────────────────

type exprNode interface{ exprNode() }

type (
	numLit    struct{ val float64 }
	strLit    struct{ val string }
	boolLit   struct{ val bool }
	nullLit   struct{}
	identExpr struct{ name string }
	fieldExpr struct{ obj exprNode; field string }
	indexExpr struct{ obj, idx exprNode }
	binExpr      struct{ op string; left, right exprNode }
	unaryExpr    struct{ op string; operand exprNode }
	callExpr     struct{ fn string; args []exprNode }
	intervalExpr struct{ amount exprNode; unit string }
)

func (*numLit) exprNode()       {}
func (*strLit) exprNode()       {}
func (*boolLit) exprNode()      {}
func (*nullLit) exprNode()      {}
func (*identExpr) exprNode()    {}
func (*fieldExpr) exprNode()    {}
func (*indexExpr) exprNode()    {}
func (*binExpr) exprNode()      {}
func (*unaryExpr) exprNode()    {}
func (*callExpr) exprNode()     {}
func (*intervalExpr) exprNode() {}

// ── Parser ────────────────────────────────────────────────────────────────────

type parser struct{ l *lexer }

func parseExpr(src string) (exprNode, error) {
	l, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{l: l}
	n, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.l.peek().kind != tkEOF {
		return nil, fmt.Errorf("unexpected token %q", p.l.peek().sval)
	}
	return n, nil
}

func (p *parser) parseOr() (exprNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.l.peek().kind == tkOr {
		p.l.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &binExpr{op: "||", left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (exprNode, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.l.peek().kind == tkAnd {
		p.l.next()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &binExpr{op: "&&", left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseNot() (exprNode, error) {
	if p.l.peek().kind == tkNot {
		p.l.next()
		operand, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &unaryExpr{op: "!", operand: operand}, nil
	}
	return p.parseCmp()
}

func (p *parser) parseCmp() (exprNode, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	switch p.l.peek().kind {
	case tkEq, tkNeq, tkLt, tkGt, tkLte, tkGte:
		op := p.l.next().sval
		right, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		return &binExpr{op: op, left: left, right: right}, nil
	}
	return left, nil
}

func (p *parser) parseAdd() (exprNode, error) {
	left, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for {
		switch p.l.peek().kind {
		case tkPlus:
			p.l.next()
			right, err := p.parseMul()
			if err != nil {
				return nil, err
			}
			left = &binExpr{op: "+", left: left, right: right}
		case tkMinus:
			p.l.next()
			right, err := p.parseMul()
			if err != nil {
				return nil, err
			}
			left = &binExpr{op: "-", left: left, right: right}
		default:
			return left, nil
		}
	}
}

func (p *parser) parseMul() (exprNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		switch p.l.peek().kind {
		case tkStar:
			p.l.next()
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = &binExpr{op: "*", left: left, right: right}
		case tkSlash:
			p.l.next()
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = &binExpr{op: "/", left: left, right: right}
		default:
			return left, nil
		}
	}
}

func (p *parser) parseUnary() (exprNode, error) {
	if p.l.peek().kind == tkMinus {
		p.l.next()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &unaryExpr{op: "-", operand: operand}, nil
	}
	return p.parsePostfix()
}

// parsePostfix handles dot-access and bracket-index on any primary expression.
func (p *parser) parsePostfix() (exprNode, error) {
	n, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		switch p.l.peek().kind {
		case tkDot:
			p.l.next()
			tok, err := p.l.expect(tkIdent)
			if err != nil {
				return nil, err
			}
			n = &fieldExpr{obj: n, field: tok.sval}
		case tkLBracket:
			p.l.next()
			idx, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			if _, err := p.l.expect(tkRBracket); err != nil {
				return nil, err
			}
			n = &indexExpr{obj: n, idx: idx}
		default:
			return n, nil
		}
	}
}

func (p *parser) parsePrimary() (exprNode, error) {
	tok := p.l.peek()
	switch tok.kind {
	case tkNum:
		p.l.next()
		return &numLit{val: tok.numval}, nil

	case tkStr:
		p.l.next()
		return &strLit{val: tok.sval}, nil

	case tkIdent:
		switch tok.sval {
		case "true":
			p.l.next()
			return &boolLit{val: true}, nil
		case "false":
			p.l.next()
			return &boolLit{val: false}, nil
		case "null":
			p.l.next()
			return &nullLit{}, nil
		}
		if strings.ToUpper(tok.sval) == "INTERVAL" {
			p.l.next() // consume INTERVAL keyword
			amount, err := p.parseAdd()
			if err != nil {
				return nil, err
			}
			unitTok, err := p.l.expect(tkIdent)
			if err != nil {
				return nil, fmt.Errorf("INTERVAL: expected time unit")
			}
			unit := strings.ToUpper(unitTok.sval)
			switch unit {
			case "SECOND", "SECONDS", "MINUTE", "MINUTES", "HOUR", "HOURS",
				"DAY", "DAYS", "WEEK", "WEEKS", "MONTH", "MONTHS", "YEAR", "YEARS":
			default:
				return nil, fmt.Errorf("INTERVAL: unknown unit %q", unitTok.sval)
			}
			return &intervalExpr{amount: amount, unit: unit}, nil
		}
		p.l.next()
		// Function call?
		if p.l.peek().kind == tkLParen {
			p.l.next()
			var args []exprNode
			if p.l.peek().kind != tkRParen {
				arg, err := p.parseOr()
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
				for p.l.peek().kind == tkComma {
					p.l.next()
					arg, err := p.parseOr()
					if err != nil {
						return nil, err
					}
					args = append(args, arg)
				}
			}
			if _, err := p.l.expect(tkRParen); err != nil {
				return nil, err
			}
			return &callExpr{fn: tok.sval, args: args}, nil
		}
		return &identExpr{name: tok.sval}, nil

	case tkLParen:
		p.l.next()
		n, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if _, err := p.l.expect(tkRParen); err != nil {
			return nil, err
		}
		return n, nil
	}
	return nil, fmt.Errorf("unexpected token %q", tok.sval)
}

// ── Evaluator ─────────────────────────────────────────────────────────────────

type evaluator struct{ ctx map[string]interface{} }

func Evaluate(expr string, ctx map[string]interface{}) (interface{}, error) {
	n, err := parseExpr(expr)
	if err != nil {
		return nil, err
	}
	return (&evaluator{ctx: ctx}).eval(n)
}

func (e *evaluator) eval(n exprNode) (interface{}, error) {
	switch v := n.(type) {
	case *numLit:
		return v.val, nil
	case *strLit:
		return v.val, nil
	case *boolLit:
		return v.val, nil
	case *nullLit:
		return nil, nil

	case *identExpr:
		val, ok := e.ctx[v.name]
		if !ok {
			return undefinedType{}, nil
		}
		return val, nil

	case *fieldExpr:
		obj, err := e.eval(v.obj)
		if err != nil {
			return nil, err
		}
		if _, undef := obj.(undefinedType); undef || obj == nil {
			return undefinedType{}, nil
		}
		m, ok := obj.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("cannot access field %q on %T", v.field, obj)
		}
		val, exists := m[v.field]
		if !exists {
			return undefinedType{}, nil
		}
		return val, nil

	case *indexExpr:
		obj, err := e.eval(v.obj)
		if err != nil {
			return nil, err
		}
		if _, undef := obj.(undefinedType); undef || obj == nil {
			return undefinedType{}, nil
		}
		arr, ok := obj.([]interface{})
		if !ok {
			return nil, fmt.Errorf("cannot index %T with []", obj)
		}
		idxVal, err := e.eval(v.idx)
		if err != nil {
			return nil, err
		}
		f, ok := toFloat64(idxVal)
		if !ok {
			return nil, fmt.Errorf("array index must be numeric, got %T", idxVal)
		}
		i := int(f)
		if i < 0 || i >= len(arr) {
			return undefinedType{}, nil
		}
		return arr[i], nil

	case *unaryExpr:
		return e.evalUnary(v)
	case *binExpr:
		return e.evalBin(v)
	case *callExpr:
		return e.evalCall(v)
	case *intervalExpr:
		return e.evalInterval(v)
	}
	return nil, fmt.Errorf("unknown node type %T", n)
}

func (e *evaluator) evalInterval(n *intervalExpr) (interface{}, error) {
	amtVal, err := e.eval(n.amount)
	if err != nil {
		return nil, err
	}
	f, ok := toFloat64(amtVal)
	if !ok {
		return nil, fmt.Errorf("INTERVAL amount must be numeric, got %T", amtVal)
	}
	whole := int(f)
	switch n.unit {
	case "SECOND", "SECONDS":
		return intervalVal{dur: time.Duration(f * float64(time.Second))}, nil
	case "MINUTE", "MINUTES":
		return intervalVal{dur: time.Duration(f * float64(time.Minute))}, nil
	case "HOUR", "HOURS":
		return intervalVal{dur: time.Duration(f * float64(time.Hour))}, nil
	case "DAY", "DAYS":
		return intervalVal{days: whole}, nil
	case "WEEK", "WEEKS":
		return intervalVal{days: whole * 7}, nil
	case "MONTH", "MONTHS":
		return intervalVal{months: whole}, nil
	case "YEAR", "YEARS":
		return intervalVal{years: whole}, nil
	}
	return nil, fmt.Errorf("INTERVAL: unknown unit %q", n.unit)
}

func (e *evaluator) evalUnary(n *unaryExpr) (interface{}, error) {
	switch n.op {
	case "!":
		val, err := e.eval(n.operand)
		if err != nil {
			return nil, err
		}
		b, err := asBool(val)
		if err != nil {
			return nil, fmt.Errorf("!: %w", err)
		}
		return !b, nil

	case "-":
		val, err := e.eval(n.operand)
		if err != nil {
			return nil, err
		}
		f, ok := toFloat64(val)
		if !ok {
			return nil, fmt.Errorf("unary - requires numeric operand, got %T", val)
		}
		return -f, nil
	}
	return nil, fmt.Errorf("unknown unary op %q", n.op)
}

func (e *evaluator) evalBin(n *binExpr) (interface{}, error) {
	// Short-circuit logical operators
	switch n.op {
	case "&&":
		lv, err := e.eval(n.left)
		if err != nil {
			return nil, err
		}
		lb, err := asBool(lv)
		if err != nil {
			return nil, fmt.Errorf("&&: %w", err)
		}
		if !lb {
			return false, nil
		}
		rv, err := e.eval(n.right)
		if err != nil {
			return nil, err
		}
		rb, err := asBool(rv)
		if err != nil {
			return nil, fmt.Errorf("&&: %w", err)
		}
		return rb, nil

	case "||":
		lv, err := e.eval(n.left)
		if err != nil {
			return nil, err
		}
		lb, err := asBool(lv)
		if err != nil {
			return nil, fmt.Errorf("||: %w", err)
		}
		if lb {
			return true, nil
		}
		rv, err := e.eval(n.right)
		if err != nil {
			return nil, err
		}
		rb, err := asBool(rv)
		if err != nil {
			return nil, fmt.Errorf("||: %w", err)
		}
		return rb, nil
	}

	lv, err := e.eval(n.left)
	if err != nil {
		return nil, err
	}
	rv, err := e.eval(n.right)
	if err != nil {
		return nil, err
	}

	switch n.op {
	case "+":
		// time.Time + INTERVAL
		if t, isTime := lv.(time.Time); isTime {
			if iv, isIV := rv.(intervalVal); isIV {
				return t.AddDate(iv.years, iv.months, iv.days).Add(iv.dur), nil
			}
		}
		// String concatenation takes precedence over numeric addition.
		if ls, ok := lv.(string); ok {
			if rs, ok := rv.(string); ok {
				return ls + rs, nil
			}
		}
		lf, lok := toFloat64(lv)
		rf, rok := toFloat64(rv)
		if !lok || !rok {
			return nil, fmt.Errorf("+: requires numeric or string operands, got %T and %T", lv, rv)
		}
		return lf + rf, nil

	case "-":
		// time.Time - INTERVAL
		if t, isTime := lv.(time.Time); isTime {
			if iv, isIV := rv.(intervalVal); isIV {
				return t.AddDate(-iv.years, -iv.months, -iv.days).Add(-iv.dur), nil
			}
		}
		lf, lok := toFloat64(lv)
		rf, rok := toFloat64(rv)
		if !lok || !rok {
			return nil, fmt.Errorf("-: requires numeric operands, got %T and %T", lv, rv)
		}
		return lf - rf, nil

	case "*":
		lf, lok := toFloat64(lv)
		rf, rok := toFloat64(rv)
		if !lok || !rok {
			return nil, fmt.Errorf("*: requires numeric operands, got %T and %T", lv, rv)
		}
		return lf * rf, nil

	case "/":
		lf, lok := toFloat64(lv)
		rf, rok := toFloat64(rv)
		if !lok || !rok {
			return nil, fmt.Errorf("/: requires numeric operands, got %T and %T", lv, rv)
		}
		if rf == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return lf / rf, nil

	case "==":
		return valEqual(lv, rv), nil
	case "!=", "<>":
		return !valEqual(lv, rv), nil

	case "<":
		c, err := valCompare(lv, rv)
		if err != nil {
			return nil, err
		}
		return c < 0, nil
	case ">":
		c, err := valCompare(lv, rv)
		if err != nil {
			return nil, err
		}
		return c > 0, nil
	case "<=":
		c, err := valCompare(lv, rv)
		if err != nil {
			return nil, err
		}
		return c <= 0, nil
	case ">=":
		c, err := valCompare(lv, rv)
		if err != nil {
			return nil, err
		}
		return c >= 0, nil
	}
	return nil, fmt.Errorf("unknown binary op %q", n.op)
}

func (e *evaluator) evalCall(n *callExpr) (interface{}, error) {
	switch n.fn {
	case "exists":
		if len(n.args) != 1 {
			return nil, fmt.Errorf("exists() takes exactly 1 argument")
		}
		val, err := e.eval(n.args[0])
		if err != nil {
			// A hard evaluation error (e.g. type mismatch) still means it doesn't exist.
			return false, nil
		}
		_, isUndef := val.(undefinedType)
		return !isUndef && val != nil, nil

	case "len":
		if len(n.args) != 1 {
			return nil, fmt.Errorf("len() takes exactly 1 argument")
		}
		val, err := e.eval(n.args[0])
		if err != nil {
			return nil, err
		}
		switch v := val.(type) {
		case []interface{}:
			return float64(len(v)), nil
		case string:
			return float64(len(v)), nil
		case undefinedType, nil:
			return float64(0), nil
		default:
			return nil, fmt.Errorf("len() requires array or string, got %T", v)
		}
	case "ifnull":
		if len(n.args) != 2 {
			return nil, fmt.Errorf("ifnull() takes exactly 2 arguments")
		}
		val, err := e.eval(n.args[0])
		if err != nil {
			return nil, err
		}
		if isNull(val) {
			return e.eval(n.args[1])
		}
		return val, nil

	case "contains":
		if len(n.args) != 2 {
			return nil, fmt.Errorf("contains() takes exactly 2 arguments")
		}
		arrVal, err := e.eval(n.args[0])
		if err != nil {
			return nil, err
		}
		arr, ok := arrVal.([]interface{})
		if !ok {
			return nil, fmt.Errorf("contains() first argument must be an array, got %T", arrVal)
		}
		needle, err := e.eval(n.args[1])
		if err != nil {
			return nil, err
		}
		for _, elem := range arr {
			if valEqual(elem, needle) {
				return true, nil
			}
		}
		return false, nil

	case "all", "any", "count":
		if len(n.args) != 2 {
			return nil, fmt.Errorf("%s() takes exactly 2 arguments", n.fn)
		}
		arrVal, err := e.eval(n.args[0])
		if err != nil {
			return nil, err
		}
		arr, ok := arrVal.([]interface{})
		if !ok {
			return nil, fmt.Errorf("%s() first argument must be an array, got %T", n.fn, arrVal)
		}
		prev, hadPrev := e.ctx["it"]
		var matched float64
		for _, elem := range arr {
			e.ctx["it"] = elem
			res, err := e.eval(n.args[1])
			if err != nil {
				return nil, err
			}
			b, err := asBool(res)
			if err != nil {
				return nil, fmt.Errorf("%s() expression must evaluate to bool: %v", n.fn, err)
			}
			if b {
				matched++
				if n.fn == "any" {
					if hadPrev {
						e.ctx["it"] = prev
					} else {
						delete(e.ctx, "it")
					}
					return true, nil
				}
			} else if n.fn == "all" {
				if hadPrev {
					e.ctx["it"] = prev
				} else {
					delete(e.ctx, "it")
				}
				return false, nil
			}
		}
		if hadPrev {
			e.ctx["it"] = prev
		} else {
			delete(e.ctx, "it")
		}
		switch n.fn {
		case "all":
			return true, nil
		case "any":
			return false, nil
		default: // count
			return matched, nil
		}
	case "now":
		if len(n.args) != 0 {
			return nil, fmt.Errorf("now() takes no arguments")
		}
		return clockNow(), nil

	case "parseTime":
		if len(n.args) != 1 {
			return nil, fmt.Errorf("parseTime() takes exactly 1 argument")
		}
		val, err := e.eval(n.args[0])
		if err != nil {
			return nil, err
		}
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("parseTime() requires a string, got %T", val)
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("parseTime(): %w", err)
		}
		return t, nil
	}
	return nil, fmt.Errorf("unknown function %q", n.fn)
}

// ── Value helpers ─────────────────────────────────────────────────────────────

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case bool:
		if val {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func asBool(v interface{}) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case float64:
		return val != 0, nil
	case string:
		return val != "", nil
	case nil, undefinedType:
		return false, nil
	}
	return false, fmt.Errorf("cannot coerce %T to bool", v)
}

// isNull reports whether v is nil or the undefined sentinel.
func isNull(v interface{}) bool {
	if v == nil {
		return true
	}
	_, undef := v.(undefinedType)
	return undef
}

func valEqual(a, b interface{}) bool {
	if isNull(a) && isNull(b) {
		return true
	}
	if isNull(a) || isNull(b) {
		return false
	}
	if af, aok := toFloat64(a); aok {
		if bf, bok := toFloat64(b); bok {
			return af == bf
		}
	}
	at, atime := a.(time.Time)
	bt, btime := b.(time.Time)
	if atime || btime {
		if !atime {
			var err error
			at, err = tryParseTime(a)
			if err != nil {
				return false
			}
		}
		if !btime {
			var err error
			bt, err = tryParseTime(b)
			if err != nil {
				return false
			}
		}
		return at.Equal(bt)
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func valCompare(a, b interface{}) (int, error) {
	if isNull(a) || isNull(b) {
		return 0, fmt.Errorf("cannot order null values; use == or != for null checks")
	}
	af, aok := toFloat64(a)
	bf, bok := toFloat64(b)
	if aok && bok {
		switch {
		case af < bf:
			return -1, nil
		case af > bf:
			return 1, nil
		default:
			return 0, nil
		}
	}
	at, atime := a.(time.Time)
	bt, btime := b.(time.Time)
	if atime || btime {
		var err error
		if !atime {
			at, err = tryParseTime(a)
			if err != nil {
				return 0, fmt.Errorf("cannot compare %T with time: %v", a, err)
			}
		}
		if !btime {
			bt, err = tryParseTime(b)
			if err != nil {
				return 0, fmt.Errorf("cannot compare %T with time: %v", b, err)
			}
		}
		switch {
		case at.Before(bt):
			return -1, nil
		case at.After(bt):
			return 1, nil
		default:
			return 0, nil
		}
	}
	as, astr := a.(string)
	bs, bstr := b.(string)
	if astr && bstr {
		switch {
		case as < bs:
			return -1, nil
		case as > bs:
			return 1, nil
		default:
			return 0, nil
		}
	}
	return 0, fmt.Errorf("cannot compare %T and %T", a, b)
}

func tryParseTime(v interface{}) (time.Time, error) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("expected string or time.Time, got %T", v)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("cannot parse %q as RFC3339: %w", s, err)
	}
	return t, nil
}
