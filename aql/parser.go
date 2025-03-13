package aql

import (
	"errors"
	"fmt"
	openapi "github.com/aep/apogy/api/go"
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
	TOKEN_PARAM // New token type for parameter placeholders
	TOKEN_AND   // Logical AND operator (& or &&)
	TOKEN_SKIP  // Skip operator ($)
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
	case TOKEN_PARAM:
		return "PARAM"
	case TOKEN_AND:
		return "AND"
	case TOKEN_SKIP:
		return "SKIP"
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

// Helper function to check if current position contains & or &&
func (l *Lexer) isAmpersandOperator() bool {
	if l.ch != '&' {
		return false
	}

	// Check if it's a double ampersand
	if l.readPosition < len(l.input) && l.input[l.readPosition] == '&' {
		return true
	}

	// Single ampersand is also valid
	return true
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
	case '$':
		tok = Token{TOKEN_SKIP, string(l.ch)}
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
	case '?':
		tok = Token{TOKEN_PARAM, string(l.ch)}
	case '&':
		if l.readPosition < len(l.input) && l.input[l.readPosition] == '&' {
			literal := "&&"
			l.readChar() // consume the second &
			tok = Token{TOKEN_AND, literal}
		} else {
			tok = Token{TOKEN_AND, string(l.ch)}
		}
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
	Filter []openapi.Filter
	Links  []*Query
}

func (q *Query) String() string {
	var parts []string
	parts = append(parts, q.Type)

	if len(q.Filter) > 0 {
		filters := make([]string, 0)
		for _, filter := range q.Filter {
			var operator string = "="
			var value interface{} = nil
			var skipValue interface{} = nil

			if filter.Equal != nil {
				operator = "="
				value = *filter.Equal
			} else if filter.Less != nil {
				operator = "<"
				value = *filter.Less
			} else if filter.Greater != nil {
				operator = ">"
				value = *filter.Greater
			} else if filter.Prefix != nil {
				operator = "^"
				value = *filter.Prefix

				// Check for Skip value
				if filter.Skip != nil {
					skipValue = *filter.Skip
				}
			}

			if value == nil {
				filters = append(filters, filter.Key)
				continue
			}

			var filterStr string
			switch val := value.(type) {
			case string:
				filterStr = fmt.Sprintf(`%s%s"%s"`, filter.Key, operator, val)
			case float64:
				filterStr = fmt.Sprintf(`%s%s%g`, filter.Key, operator, val)
			case bool:
				filterStr = fmt.Sprintf(`%s%s%v`, filter.Key, operator, val)
			default:
				filterStr = fmt.Sprintf(`%s%s%v`, filter.Key, operator, val)
			}

			// Append skip part if it exists
			if skipValue != nil {
				switch skipVal := skipValue.(type) {
				case string:
					filterStr += fmt.Sprintf(`$"%s"`, skipVal)
				case float64:
					filterStr += fmt.Sprintf(`$%g`, skipVal)
				case bool:
					filterStr += fmt.Sprintf(`$%v`, skipVal)
				default:
					filterStr += fmt.Sprintf(`$%v`, skipVal)
				}
			}

			filters = append(filters, filterStr)
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
	l              *Lexer
	curToken       Token
	params         []interface{} // Stores the parameter values
	paramIndex     int           // Current parameter index
	collectingOnly bool          // Whether we're only collecting parameter positions without substituting
}

func NewParser(l *Lexer, params ...interface{}) *Parser {
	p := &Parser{
		l:              l,
		params:         params,
		paramIndex:     0,
		collectingOnly: len(params) == 0,
	}
	p.nextToken()
	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.l.NextToken()
}

func (p *Parser) ParseQuery() (*Query, error) {

	if p.curToken.Type != TOKEN_IDENT {
		return nil, fmt.Errorf("expected identifier, got %s", tokenName(p.curToken.Type))
	}

	query := &Query{
		Type: p.curToken.Literal,
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

func (p *Parser) parseFilter() ([]openapi.Filter, error) {
	filters := make([]openapi.Filter, 0)

	p.nextToken() // consume (

	for p.curToken.Type != TOKEN_RPAREN && p.curToken.Type != TOKEN_EOF {

		// Skip filter separators (comma or logical AND)
		for p.curToken.Type == TOKEN_COMMA || p.curToken.Type == TOKEN_AND {
			p.nextToken()
		}

		if p.curToken.Type != TOKEN_IDENT {
			return nil, fmt.Errorf("expected identifier in filter, got %s", tokenName(p.curToken.Type))
		}

		key := p.curToken.Literal
		p.nextToken()

		if p.curToken.Type == TOKEN_RPAREN || p.curToken.Type == TOKEN_IDENT {
			// Create a filter with just a key (no operator/value)
			filter := openapi.Filter{
				Key: key,
			}
			filters = append(filters, filter)
			continue
		}

		var operator TokenType = p.curToken.Type
		if operator != TOKEN_EQUALS && operator != TOKEN_LESS && operator != TOKEN_GREATER && operator != TOKEN_PREFIX {
			return nil, fmt.Errorf("expected =, <, >, or ^ in filter, got %s", tokenName(p.curToken.Type))
		}
		p.nextToken()

		// Parse the value
		if p.curToken.Type != TOKEN_IDENT && p.curToken.Type != TOKEN_STRING && p.curToken.Type != TOKEN_PARAM {
			return nil, fmt.Errorf("expected identifier, string, or parameter placeholder as value, got %s", tokenName(p.curToken.Type))
		}

		var processedValue interface{}

		if p.curToken.Type == TOKEN_PARAM {
			// Handle parameter placeholder
			if p.collectingOnly {
				// Just increment the parameter count during collection phase
				p.paramIndex++
				processedValue = nil
			} else {
				// Check if we have enough parameters
				if p.paramIndex >= len(p.params) {
					return nil, fmt.Errorf("not enough parameters provided, needed at least %d", p.paramIndex+1)
				}
				processedValue = p.params[p.paramIndex]
				p.paramIndex++
			}
		} else {
			// Handle literal values
			value := p.curToken.Literal

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
		}

		// Create filter with appropriate operator
		filter := openapi.Filter{
			Key: key,
		}

		// Set the operator-specific field
		switch operator {
		case TOKEN_EQUALS:
			filter.Equal = &processedValue
		case TOKEN_LESS:
			filter.Less = &processedValue
		case TOKEN_GREATER:
			filter.Greater = &processedValue
		case TOKEN_PREFIX:
			filter.Prefix = &processedValue

			// Check for SKIP operation after a PREFIX
			p.nextToken()

			if p.curToken.Type == TOKEN_SKIP {
				// We found a skip operation, advance and get the skip value
				p.nextToken()

				if p.curToken.Type != TOKEN_IDENT && p.curToken.Type != TOKEN_STRING && p.curToken.Type != TOKEN_PARAM {
					return nil, fmt.Errorf("expected identifier, string, or parameter placeholder as skip value, got %s", tokenName(p.curToken.Type))
				}

				var skipValue interface{}

				if p.curToken.Type == TOKEN_PARAM {
					// Handle parameter placeholder for skip
					if p.collectingOnly {
						p.paramIndex++
						skipValue = nil
					} else {
						if p.paramIndex >= len(p.params) {
							return nil, fmt.Errorf("not enough parameters provided for skip, needed at least %d", p.paramIndex+1)
						}
						skipValue = p.params[p.paramIndex]
						p.paramIndex++
					}
				} else {
					// Handle literal skip values
					skip := p.curToken.Literal

					if p.curToken.Type == TOKEN_IDENT {
						if skip == "true" {
							skipValue = true
						} else if skip == "false" {
							skipValue = false
						} else if num, err := strconv.ParseFloat(skip, 64); err == nil {
							skipValue = num
						} else {
							skipValue = skip
						}
					} else if p.curToken.Type == TOKEN_STRING {
						skipValue = skip
					}
				}

				// Set the Skip field in the filter
				filter.Skip = &skipValue

				// Add the filter with both Prefix and Skip to the filters slice
				filters = append(filters, filter)

				p.nextToken()
				continue // Skip the next token advancement since we've already done it
			} else {
				// No skip operation, add the filter with just the prefix
				filters = append(filters, filter)
				continue // Skip the p.nextToken() at the end of the loop since we already advanced
			}
		default:
			// For other operators, proceed normally
		}

		filters = append(filters, filter)
		p.nextToken()
	}

	if p.curToken.Type != TOKEN_RPAREN {
		return nil, errors.New("expected )")
	}
	p.nextToken()

	return filters, nil
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

func Parse(input string, params ...interface{}) (*Query, error) {
	// First, parse the query once to validate and count parameters
	l := NewLexer(input)
	p := NewParser(l)
	_, err := p.ParseQuery()
	if err != nil {
		return nil, err
	}

	// If parameters are expected, validate that we have enough
	if p.paramIndex > 0 && len(params) < p.paramIndex {
		return nil, fmt.Errorf("query contains %d parameter placeholders but only %d values were provided",
			p.paramIndex, len(params))
	}

	// Parse again with actual parameter values
	l = NewLexer(input)
	p = NewParser(l, params...)
	return p.ParseQuery()
}

func isLetter(ch byte) bool {
	return unicode.IsLetter(rune(ch))
}

func isDigit(ch byte) bool {
	return unicode.IsDigit(rune(ch))
}
