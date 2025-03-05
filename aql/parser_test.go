package aql

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestLexer(t *testing.T) {
	tests := []struct {
		input    string
		expected []Token
	}{
		{
			"Book",
			[]Token{
				{TOKEN_IDENT, "Book"},
				{TOKEN_EOF, ""},
			},
		},
		{
			"Book(key=val)",
			[]Token{
				{TOKEN_IDENT, "Book"},
				{TOKEN_LPAREN, "("},
				{TOKEN_IDENT, "key"},
				{TOKEN_EQUALS, "="},
				{TOKEN_IDENT, "val"},
				{TOKEN_RPAREN, ")"},
				{TOKEN_EOF, ""},
			},
		},
		{
			"Book { Author }",
			[]Token{
				{TOKEN_IDENT, "Book"},
				{TOKEN_LBRACE, "{"},
				{TOKEN_IDENT, "Author"},
				{TOKEN_RBRACE, "}"},
				{TOKEN_EOF, ""},
			},
		},
		{
			`Book(id=123) { author(active=true,,) { name } }`,
			[]Token{
				{TOKEN_IDENT, "Book"},
				{TOKEN_LPAREN, "("},
				{TOKEN_IDENT, "id"},
				{TOKEN_EQUALS, "="},
				{TOKEN_IDENT, "123"},
				{TOKEN_RPAREN, ")"},
				{TOKEN_LBRACE, "{"},
				{TOKEN_IDENT, "author"},
				{TOKEN_LPAREN, "("},
				{TOKEN_IDENT, "active"},
				{TOKEN_EQUALS, "="},
				{TOKEN_IDENT, "true"},
				{TOKEN_COMMA, ","},
				{TOKEN_COMMA, ","},
				{TOKEN_RPAREN, ")"},
				{TOKEN_LBRACE, "{"},
				{TOKEN_IDENT, "name"},
				{TOKEN_RBRACE, "}"},
				{TOKEN_RBRACE, "}"},
				{TOKEN_EOF, ""},
			},
		},
		{
			`com.bob.Book { Author(active=true) { Name } }`,
			[]Token{
				{TOKEN_IDENT, "com.bob.Book"},
				{TOKEN_LBRACE, "{"},
				{TOKEN_IDENT, "Author"},
				{TOKEN_LPAREN, "("},
				{TOKEN_IDENT, "active"},
				{TOKEN_EQUALS, "="},
				{TOKEN_IDENT, "true"},
				{TOKEN_RPAREN, ")"},
				{TOKEN_LBRACE, "{"},
				{TOKEN_IDENT, "Name"},
				{TOKEN_RBRACE, "}"},
				{TOKEN_RBRACE, "}"},
				{TOKEN_EOF, ""},
			},
		},
		{
			// Test parameters
			"Book(id=?)",
			[]Token{
				{TOKEN_IDENT, "Book"},
				{TOKEN_LPAREN, "("},
				{TOKEN_IDENT, "id"},
				{TOKEN_EQUALS, "="},
				{TOKEN_PARAM, "?"},
				{TOKEN_RPAREN, ")"},
				{TOKEN_EOF, ""},
			},
		},
	}

	for i, tt := range tests {
		l := NewLexer(tt.input)
		tokens := []Token{}
		for {
			tok := l.NextToken()
			tokens = append(tokens, tok)
			if tok.Type == TOKEN_EOF {
				break
			}
		}

		if !reflect.DeepEqual(tokens, tt.expected) {
			t.Errorf("test %d: wrong tokens.\nexpected=%+v\ngot=%+v",
				i, tt.expected, tokens)
		}
	}
}

func TestParser(t *testing.T) {
	tests := []struct {
		input       string
		expected    *Query
		shouldError bool
	}{
		{
			input:       "com.Book",
			shouldError: false,
			expected: &Query{
				Type: "com.Book",
			},
		},
		{
			input:       "Book {",
			shouldError: true,
		},
		{
			input:       "Book(val)",
			shouldError: false,
			expected: &Query{
				Type: "Book",
				Filter: map[string]interface{}{
					"val": nil,
				},
			},
		},
		{
			input:       "Book(val val.a)",
			shouldError: false,
			expected: &Query{
				Type: "Book",
				Filter: map[string]interface{}{
					"val":   nil,
					"val.a": nil,
				},
			},
		},
		{
			input:       "Book(key=val",
			shouldError: true,
		},
		{
			input:       "Book { Author",
			shouldError: true,
		},
		{
			input: "Book { Author }",
			expected: &Query{
				Type: "Book",
				Links: []*Query{
					{
						Type: "Author",
					},
				},
			},
			shouldError: false,
		},
		{
			input: `Book(id=123) { Author }`,
			expected: &Query{
				Type: "Book",
				Filter: map[string]interface{}{
					"id": float64(123),
				},
				Links: []*Query{
					{
						Type: "Author",
					},
				},
			},
			shouldError: false,
		},
		{
			input: `com.bob.Book { Author(active=true) { Name } }`,
			expected: &Query{
				Type: "com.bob.Book",
				Links: []*Query{
					{
						Type: "Author",
						Filter: map[string]interface{}{
							"active": true,
						},
						Links: []*Query{
							{
								Type: "Name",
							},
						},
					},
				},
			},
			shouldError: false,
		},
		{
			input: `Book(name="test" count=42 ,,,, enabled=true)`,
			expected: &Query{
				Type: "Book",
				Filter: map[string]interface{}{
					"name":    "test",
					"count":   float64(42),
					"enabled": true,
				},
			},
			shouldError: false,
		},
	}

	for i, tt := range tests {
		l := NewLexer(tt.input)
		p := NewParser(l)
		actual, err := p.ParseQuery()

		if tt.shouldError {
			if err == nil {
				t.Errorf("test %d: expected error but got none", i)
			}
			continue
		}

		if err != nil {
			t.Errorf("test %d: unexpected error: %v", i, err)
			continue
		}

		if !reflect.DeepEqual(actual, tt.expected) {
			a, _ := json.Marshal(actual)
			b, _ := json.Marshal(tt.expected)
			t.Errorf("test %d: wrong query.\nexpected=%s\ngot=%s",
				i, b, a)
		}
	}
}

func TestEdgeCases(t *testing.T) {
	tests := []struct {
		input       string
		shouldError bool
	}{
		{
			input:       "",
			shouldError: true,
		},
		{
			input:       "Book(key==val)",
			shouldError: true,
		},
		{
			input:       `Book(key="val)`,
			shouldError: true,
		},
		{
			input:       "Book { Author { } }",
			shouldError: false,
		},
		{
			input:       "Book.SubType { Author }",
			shouldError: false,
		},
		{
			input:       "Book-Type { Author }",
			shouldError: false,
		},
		{
			input:       `Book(invalid=json{)`,
			shouldError: true,
		},
		{
			// This should error because we're not providing any parameters
			input:       "Book(name=?)",
			shouldError: true,
		},
	}

	for i, tt := range tests {
		_, err := Parse(tt.input)
		if tt.shouldError && err == nil {
			t.Errorf("test %d: expected error for input '%s' but got none", i, tt.input)
		} else if !tt.shouldError && err != nil {
			t.Errorf("test %d: unexpected error for input '%s': %v", i, tt.input, err)
		}
	}
}

func TestParameterizedQueries(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		params      []interface{}
		expected    *Query
		shouldError bool
	}{
		{
			name:  "Single string parameter",
			input: `Book(name=?)`,
			params: []interface{}{
				"Harry Potter",
			},
			expected: &Query{
				Type: "Book",
				Filter: map[string]interface{}{
					"name": "Harry Potter",
				},
			},
			shouldError: false,
		},
		{
			name:  "Multiple parameters of different types",
			input: `Book(name=? count=? available=?)`,
			params: []interface{}{
				"Game of Thrones",
				float64(42),
				true,
			},
			expected: &Query{
				Type: "Book",
				Filter: map[string]interface{}{
					"name":      "Game of Thrones",
					"count":     float64(42),
					"available": true,
				},
			},
			shouldError: false,
		},
		{
			name:  "Different operators with parameters",
			input: `Book(title=? price<? popularity>? prefix^?)`,
			params: []interface{}{
				"The Hobbit",
				float64(20),
				float64(4),
				"Lord",
			},
			expected: &Query{
				Type: "Book",
				Filter: map[string]interface{}{
					"title":       "The Hobbit",
					"price<":      float64(20),
					"popularity>": float64(4),
					"prefix^":     "Lord",
				},
			},
			shouldError: false,
		},
		{
			name:  "Nested queries with parameters",
			input: `Book(id=?) { Author(age>?) }`,
			params: []interface{}{
				float64(123),
				float64(30),
			},
			expected: &Query{
				Type: "Book",
				Filter: map[string]interface{}{
					"id": float64(123),
				},
				Links: []*Query{
					{
						Type: "Author",
						Filter: map[string]interface{}{
							"age>": float64(30),
						},
					},
				},
			},
			shouldError: false,
		},
		{
			name:        "Not enough parameters",
			input:       `Book(id=? title=?)`,
			params:      []interface{}{float64(123)},
			shouldError: true,
		},
		{
			name:   "Too many parameters",
			input:  `Book(id=?)`,
			params: []interface{}{float64(123), "extra", "params"},
			expected: &Query{
				Type: "Book",
				Filter: map[string]interface{}{
					"id": float64(123),
				},
			},
			shouldError: false, // Extra params are ignored
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := Parse(tt.input, tt.params...)

			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(actual, tt.expected) {
				a, _ := json.Marshal(actual)
				b, _ := json.Marshal(tt.expected)
				t.Errorf("wrong query.\nexpected=%s\ngot=%s", b, a)
			}
		})
	}
}

func newStr(i string) *string {
	return &i
}

