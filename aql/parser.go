package aql

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type TokenType int

const (
	TOKEN_ILLEGAL TokenType = iota
	TOKEN_EOF
	TOKEN_IDENT
	TOKEN_EQUALS
	TOKEN_LESS
	TOKEN_GREATER
	TOKEN_PREFIX
	TOKEN_LPAREN
	TOKEN_RPAREN
	TOKEN_LBRACE
	TOKEN_RBRACE
	TOKEN_STRING
	TOKEN_COMMA
)

func tokenName(i TokenType) string {
	switch i {
	case TOKEN_EOF:
		return "EOF"
	case TOKEN_IDENT:
		return "IDENT"
	case TOKEN_EQUALS:
		return "EQUALS"
	case TOKEN_LESS:
		return "LESS"
	case TOKEN_GREATER:
		return "GREATER"
	case TOKEN_PREFIX:
		return "PREFIX"
	case TOKEN_LPAREN:
		return "LPAREN"
	case TOKEN_RPAREN:
		return "RPAREN"
	case TOKEN_LBRACE:
		return "LBRACE"
	case TOKEN_RBRACE:
		return "RBRACE"
	case TOKEN_STRING:
		return "STRING"
	case TOKEN_COMMA:
		return "COMMA"
	}
	return "ILLEGAL"
}

type Token struct {
	Type    TokenType
	Literal string
}

type Lexer struct {
	input        string
	position     int
	readPosition int
	ch           byte
}

func NewLexer(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' || l.ch == '.' || l.ch == '-' {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readNumber() string {
	position := l.position
	for isDigit(l.ch) {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readString() (string, error) {
	position := l.position + 1
	for {
		l.readChar()
		if l.ch == 0 {
			return "", errors.New("unterminated string")
		}
		if l.ch == '"' {
			break
		}
	}
	return l.input[position:l.position], nil
}

func (l *Lexer) NextToken() Token {
	var tok Token

	l.skipWhitespace()

	switch l.ch {
	case '=':
		tok = Token{TOKEN_EQUALS, string(l.ch)}
	case '<':
		tok = Token{TOKEN_LESS, string(l.ch)}
	case '>':
		tok = Token{TOKEN_GREATER, string(l.ch)}
	case '^':
		tok = Token{TOKEN_PREFIX, string(l.ch)}
	case '(':
		tok = Token{TOKEN_LPAREN, string(l.ch)}
	case ')':
		tok = Token{TOKEN_RPAREN, string(l.ch)}
	case '{':
		tok = Token{TOKEN_LBRACE, string(l.ch)}
	case '}':
		tok = Token{TOKEN_RBRACE, string(l.ch)}
	case ',':
		tok = Token{TOKEN_COMMA, string(l.ch)}
	case '"':
		if str, err := l.readString(); err == nil {
			tok = Token{TOKEN_STRING, str}
		} else {
			tok = Token{TOKEN_ILLEGAL, ""}
		}
	case 0:
		tok = Token{TOKEN_EOF, ""}
	default:
		if isLetter(l.ch) || l.ch == '_' {
			tok.Literal = l.readIdentifier()
			tok.Type = TOKEN_IDENT
			return tok
		} else if isDigit(l.ch) {
			tok.Literal = l.readNumber()
			tok.Type = TOKEN_IDENT // Numbers are treated as identifiers
			return tok
		} else {
			tok = Token{TOKEN_ILLEGAL, string(l.ch)}
		}
	}

	l.readChar()
	return tok
}

type Query struct {
	Type   string
	Filter map[string]interface{}
	Links  []*Query
	Cursor *string
}

func (q *Query) String() string {
	var parts []string
	parts = append(parts, q.Type)

	if len(q.Filter) > 0 {
		filters := make([]string, 0)
		for k, v := range q.Filter {
			// Determine operator
			var operator string = "="
			var actualKey string = k
			
			if strings.HasSuffix(k, "<") {
				operator = "<"
				actualKey = strings.TrimSuffix(k, "<")
			} else if strings.HasSuffix(k, ">") {
				operator = ">"
				actualKey = strings.TrimSuffix(k, ">")
			} else if strings.HasSuffix(k, "^") {
				operator = "^"
				actualKey = strings.TrimSuffix(k, "^")
			}
			
			switch val := v.(type) {
			case string:
				filters = append(filters, fmt.Sprintf(`%s%s"%s"`, actualKey, operator, val))
			case float64:
				filters = append(filters, fmt.Sprintf(`%s%s%g`, actualKey, operator, val))
			case bool:
				filters = append(filters, fmt.Sprintf(`%s%s%v`, actualKey, operator, val))
			}
		}
		parts = append(parts, fmt.Sprintf("(%s)", strings.Join(filters, " ")))
	}

	if len(q.Links) > 0 {
		var nested []string
		for _, link := range q.Links {
			nested = append(nested, link.String())
		}
		parts = append(parts, fmt.Sprintf("{ %s }", strings.Join(nested, " ")))
	}

	return strings.Join(parts, " ")
}

type Parser struct {
	l        *Lexer
	curToken Token
}

func NewParser(l *Lexer) *Parser {
	p := &Parser{l: l}
	p.nextToken()
	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.l.NextToken()
}

func (p *Parser) ParseQuery() (*Query, error) {

	var opts = make(map[string]interface{})

	for {
		if p.curToken.Type != TOKEN_LPAREN {
			break
		}
		opts2, err := p.parseFilter()
		if err != nil {
			return nil, err
		}
		for k, v := range opts2 {
			if _, exists := opts[k]; exists {
				return nil, fmt.Errorf("opt %s specified twice", k)
			}
			opts[k] = v
		}
	}

	if p.curToken.Type != TOKEN_IDENT {
		return nil, fmt.Errorf("expected identifier, got %s", tokenName(p.curToken.Type))
	}

	query := &Query{
		Type: p.curToken.Literal,
	}
	if cursor, ok := opts["cursor"].(string); ok {
		query.Cursor = &cursor
	}

	p.nextToken()

	if p.curToken.Type == TOKEN_LPAREN {
		filter, err := p.parseFilter()
		if err != nil {
			return nil, err
		}
		query.Filter = filter
	}

	if p.curToken.Type == TOKEN_LBRACE {
		links, err := p.parseNested()
		if err != nil {
			return nil, err
		}
		query.Links = links
	}

	return query, nil
}

func (p *Parser) parseFilter() (map[string]interface{}, error) {
	filter := make(map[string]interface{})

	p.nextToken() // consume (

	for p.curToken.Type != TOKEN_RPAREN && p.curToken.Type != TOKEN_EOF {

		for p.curToken.Type == TOKEN_COMMA {
			p.nextToken()
		}

		if p.curToken.Type != TOKEN_IDENT {
			return nil, fmt.Errorf("expected identifier in filter, got %s", tokenName(p.curToken.Type))
		}

		key := p.curToken.Literal
		p.nextToken()

		if p.curToken.Type == TOKEN_RPAREN || p.curToken.Type == TOKEN_IDENT {
			filter[key] = nil
			continue
		}

		var operator TokenType = p.curToken.Type
		if operator != TOKEN_EQUALS && operator != TOKEN_LESS && operator != TOKEN_GREATER && operator != TOKEN_PREFIX {
			return nil, fmt.Errorf("expected =, <, >, or ^ in filter, got %s", tokenName(p.curToken.Type))
		}
		p.nextToken()

		// Parse the value
		if p.curToken.Type != TOKEN_IDENT && p.curToken.Type != TOKEN_STRING {
			return nil, fmt.Errorf("expected identifier or string as value, got %s", tokenName(p.curToken.Type))
		}

		value := p.curToken.Literal

		// Process the value based on token type and operator
		var processedValue interface{}
		if p.curToken.Type == TOKEN_IDENT {
			if value == "true" {
				processedValue = true
			} else if value == "false" {
				processedValue = false
			} else if num, err := strconv.ParseFloat(value, 64); err == nil {
				processedValue = num
			} else {
				processedValue = value
			}
		} else if p.curToken.Type == TOKEN_STRING {
			processedValue = value
		} else {
			return nil, fmt.Errorf("expected value, got %s", tokenName(p.curToken.Type))
		}

		// Store value with appropriate operator metadata
		switch operator {
		case TOKEN_EQUALS:
			filter[key] = processedValue
		case TOKEN_LESS:
			filter[key+"<"] = processedValue
		case TOKEN_GREATER:
			filter[key+">"] = processedValue
		case TOKEN_PREFIX:
			filter[key+"^"] = processedValue
		}

		p.nextToken()
	}

	if p.curToken.Type != TOKEN_RPAREN {
		return nil, errors.New("expected )")
	}
	p.nextToken()

	return filter, nil
}

func (p *Parser) parseNested() ([]*Query, error) {
	var links []*Query

	p.nextToken() // consume {

	for p.curToken.Type != TOKEN_RBRACE && p.curToken.Type != TOKEN_EOF {
		if p.curToken.Type != TOKEN_IDENT {
			p.nextToken()
			continue
		}

		nestedQuery, err := p.ParseQuery()
		if err != nil {
			return nil, err
		}

		if links == nil {
			links = make([]*Query, 0)
		}
		links = append(links, nestedQuery)
	}

	if p.curToken.Type != TOKEN_RBRACE {
		return nil, errors.New("expected }")
	}
	p.nextToken()

	return links, nil
}

func Parse(input string) (*Query, error) {
	l := NewLexer(input)
	p := NewParser(l)
	return p.ParseQuery()
}

func isLetter(ch byte) bool {
	return unicode.IsLetter(rune(ch))
}

func isDigit(ch byte) bool {
	return unicode.IsDigit(rune(ch))
}
