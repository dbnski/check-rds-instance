package eval

import (
	"fmt"
	"strings"
)

type exprNode interface{ exprNode() }

type (
	numLit       struct{ val float64 }
	strLit       struct{ val string }
	boolLit      struct{ val bool }
	nullLit      struct{}
	identExpr    struct{ name string }
	fieldExpr    struct{ obj exprNode; field string }
	indexExpr    struct{ obj, idx exprNode }
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
