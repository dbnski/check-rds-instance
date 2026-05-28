package eval

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

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
