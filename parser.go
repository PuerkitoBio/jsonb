package jsonb

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"unicode"
)

const DefaultChunkSize = 32 << 10 // 32K

const minChunkSize = 5

const (
	begVal = iota
	strLit
	hexEsc
	chrEsc
	endLit
	zroLit
	comExp
)

type SyntaxError struct {
	Char rune
	typ  int
}

func (s *SyntaxError) Error() string {
	var suffix string

	switch s.typ {
	case begVal:
		suffix = " looking for beginning of value"
	case strLit:
		suffix = " in string literal"
	case hexEsc:
		suffix = " in \\u hexadecimal character escape"
	case chrEsc:
		suffix = " in string escape code"
	case endLit:
		suffix = " after top-level value"
	case zroLit:
		suffix = " after top-level value 0"
	case comExp:
		suffix = " looking for a comma"
	}
	return fmt.Sprintf("invalid character %q"+suffix, s.Char)
}

type LiteralError struct {
	want, got rune
	tok       Token
}

func (l *LiteralError) Error() string {
	return fmt.Sprintf("invalid character %q in literal %s (expecting %q)", l.got, l.tok, l.want)
}

type Token int

const (
	Invalid Token = iota - 1
	Null
	False
	True
	String
	Number
	ObjectEnd
	ArrayEnd
	ArrayStart
	ObjectStart
)

var (
	tokenString = map[Token]string{
		Invalid:     "<invalid>",
		Null:        "null",
		False:       "false",
		True:        "true",
		String:      "string",
		Number:      "number",
		ArrayStart:  "[",
		ArrayEnd:    "]",
		ObjectStart: "{",
		ObjectEnd:   "}",
	}
)

func (t Token) String() string {
	return tokenString[t]
}

var (
	nullLiteral  = []byte{'u', 'l', 'l'}
	trueLiteral  = []byte{'r', 'u', 'e'}
	falseLiteral = []byte{'a', 'l', 's', 'e'}
)

type state byte

const (
	stArray state = iota
	stObjKey
	stObjVal
)

type Parser struct {
	// "JSON text is a sequence of Unicode code points."
	// Therefore, the parser uses a rune reader. If it finds
	// an invalid rune, it is a syntax error in the JSON document.
	r io.RuneReader

	// If a single raw value spans more than the specified size,
	// the value is parsed in multiple chunks of at most size bytes.
	// The minimum size allowed is 5 bytes, so that true, false and null
	// can be parsed without chunks.
	size int64

	ch    rune         // current rune
	err   error        // first error encountered
	buf   bytes.Buffer // internal buffer
	tok   Token        // current token
	chunk bool         // in a chunk
	stack []state
}

func NewParser(r io.Reader) *Parser {
	return NewParserSize(r, DefaultChunkSize)
}

func NewParserSize(r io.Reader, size int64) *Parser {
	if size < minChunkSize {
		size = minChunkSize
	}
	return &Parser{
		r:    getRuneReader(r),
		size: size,
		ch:   -1,
		tok:  Invalid,
	}
}

func (p *Parser) Reset(r io.Reader) {
	p.r = getRuneReader(r)
	p.ch = -1
	p.err = nil
	p.buf.Reset()
	p.tok = Invalid
	p.chunk = false
}

func (p *Parser) Next() bool {
	if p.err == nil && p.ch == -1 {
		// initial call, position the parser on the first non-whitespace rune
		p.next(true)
	}
	return p.parseValue()
}

func (p *Parser) Token() Token {
	return p.tok
}

func (p *Parser) Bytes() []byte {
	return p.buf.Bytes()
}

func (p *Parser) Err() error {
	if p.err == io.EOF {
		return nil
	}
	return p.err
}

// getRuneReader makes sure the parser has a RuneReader at his disposition,
// creating a bufio.Reader if required.
func getRuneReader(r io.Reader) io.RuneReader {
	if rr, ok := r.(io.RuneReader); ok {
		return rr
	}
	return bufio.NewReader(r)
}

func (p *Parser) push(st state) {
	p.stack = append(p.stack, st)
}

func (p *Parser) pop(st state) bool {
	l := len(p.stack)
	if l == 0 {
		p.error(&SyntaxError{Char: p.ch, typ: begVal})
		return false
	}

	got := p.stack[l-1]
	if got != st {
		// TODO : better error reporting, see what stdlib does
		p.error(&SyntaxError{Char: p.ch, typ: begVal})
		return false
	}
	p.stack = p.stack[:l-1]
	return true
}

func (p *Parser) parseValue() bool {
	if p.err != nil {
		return false
	}

	p.buf.Reset()
	comma := false
	wantComma := p.wantComma()
	wantValue := false

try:
	switch p.ch {
	case '{':
		if wantComma {
			p.error(&SyntaxError{Char: p.ch, typ: comExp})
			return false
		}

	case '}':
		if wantValue {
			p.error(&SyntaxError{Char: p.ch, typ: begVal})
			return false
		}

	case ':':
		if wantComma {
			p.error(&SyntaxError{Char: p.ch, typ: comExp})
			return false
		}
		if wantValue {
			p.error(&SyntaxError{Char: p.ch, typ: begVal})
			return false
		}

	case '[':
		if wantComma {
			p.error(&SyntaxError{Char: p.ch, typ: comExp})
			return false
		}

		p.tok = ArrayStart
		p.push(stArray)
		p.store()
		p.next(true) // always make progress
		return true

	case ']':
		if wantValue {
			p.error(&SyntaxError{Char: p.ch, typ: begVal})
			return false
		}

		p.tok = ArrayEnd
		if !p.pop(stArray) {
			return false
		}
		p.store()
		p.next(true) // always make progress
		return true

	case ',':
		if comma || !wantComma {
			p.error(&SyntaxError{Char: p.ch, typ: begVal})
			return false
		}

		comma = true
		wantComma = false
		wantValue = true // a value must follow the comma
		p.next(true)
		goto try

	case 't':
		if wantComma {
			p.error(&SyntaxError{Char: p.ch, typ: comExp})
			return false
		}

		p.tok = True
		p.parseLiteral(trueLiteral)

	case 'f':
		if wantComma {
			p.error(&SyntaxError{Char: p.ch, typ: comExp})
			return false
		}

		p.tok = False
		p.parseLiteral(falseLiteral)

	case 'n':
		if wantComma {
			p.error(&SyntaxError{Char: p.ch, typ: comExp})
			return false
		}

		p.tok = Null
		p.parseLiteral(nullLiteral)

	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		if wantComma {
			p.error(&SyntaxError{Char: p.ch, typ: comExp})
			return false
		}

		p.tok = Number
		p.parseNumber()

	case '"':
		if wantComma {
			p.error(&SyntaxError{Char: p.ch, typ: comExp})
			return false
		}

		p.tok = String
		p.parseString()

	default:
		p.error(&SyntaxError{Char: p.ch, typ: begVal})
	}

	return true
}

func (p *Parser) parseLiteral(exp []byte) {
	p.store()
	for _, r := range exp {
		p.next(false)
		if rune(r) != p.ch {
			p.error(&LiteralError{want: rune(r), got: p.ch, tok: p.tok})
			return
		}
		p.store()
	}

	// check if next rune is a separator
	p.next(false)
	if !isSeparator(p.ch) {
		p.error(&SyntaxError{Char: p.ch, typ: endLit})
		return
	}

	if isWhitespace(p.ch) {
		p.next(true)
	}
}

func (p *Parser) parseEscape() bool {
	p.store() // reverse solidus
	p.next(false)

	switch p.ch {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		p.store()

	case 'u':
		p.store()
		for i := 0; i < 4; i++ {
			p.next(false)
			if !isHexadecimal(p.ch) {
				p.error(&SyntaxError{Char: p.ch, typ: hexEsc})
				return false
			}
			p.store()
		}

	default:
		p.error(&SyntaxError{Char: p.ch, typ: chrEsc})
		return false
	}

	return true
}

func (p *Parser) parseString() {
	p.store() // starting double-quote

loop:
	for p.next(false) {
		switch p.ch {
		case '"':
			// unescaped double-quote, end of the string literal
			p.store()
			break loop

		case '\\':
			// parse escape sequence
			if !p.parseEscape() {
				return
			}

		default:
			// check if the rune is valid in a string literal
			if isInvalidInString(p.ch) {
				p.error(&SyntaxError{Char: p.ch, typ: strLit})
				return
			}
			p.store()
		}
	}

	// position the parser on the next rune
	p.next(true)
}

func (p *Parser) parseMantissa() {
	p.store() // the 'e' or 'E'
	sign := false
	lastIsDigit := false

loop:
	for p.next(false) {
		switch p.ch {
		case '+', '-':
			if sign {
				p.error(&SyntaxError{Char: p.ch, typ: endLit})
				return
			}
			sign = true
			lastIsDigit = false

		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			lastIsDigit = true

		default:
			if isSeparator(p.ch) {
				break loop
			}
			p.error(&SyntaxError{Char: p.ch, typ: endLit})
			return
		}
		p.store()
	}

	if !lastIsDigit {
		p.error(&SyntaxError{Char: p.ch, typ: endLit})
	}

	if isWhitespace(p.ch) {
		p.next(true)
	}
}

func (p *Parser) parseNumber() {
	p.store() // starting char (negative sign or digit)
	digit0 := p.ch
	dot := false
	lastIsDigit := digit0 != '-'

loop:
	for p.next(false) {
		switch p.ch {
		case '0':
			switch digit0 {
			case '-':
				// this is the first digit
				digit0 = p.ch
			case '0':
				// 00, invalid
				p.error(&SyntaxError{Char: p.ch, typ: zroLit})
				return
			}
			p.store()
			lastIsDigit = true

		case '1', '2', '3', '4', '5', '6', '7', '8', '9':
			if digit0 == '0' && !dot {
				p.error(&SyntaxError{Char: p.ch, typ: zroLit})
				return
			}
			p.store()
			lastIsDigit = true

		case '.':
			if dot {
				p.error(&SyntaxError{Char: p.ch, typ: endLit})
				return
			}
			dot = true
			p.store()
			lastIsDigit = false

		case 'e', 'E':
			p.parseMantissa()
			return

		default:
			if isSeparator(p.ch) {
				break loop
			}
			p.error(&SyntaxError{Char: p.ch, typ: endLit})
			return
		}
	}

	if !lastIsDigit {
		p.error(&SyntaxError{Char: p.ch, typ: endLit})
	}

	if isWhitespace(p.ch) {
		p.next(true)
	}
}

// store saves the current rune in the internal buffer.
func (p *Parser) store() bool {
	_, err := p.buf.WriteRune(p.ch)
	if err != nil {
		p.error(err)
		return false
	}
	return true
}

// error sets the error on the parser, if it is the first error encountered.
func (p *Parser) error(err error) {
	if p.err == nil || (p.err == io.EOF && err != io.EOF) {
		p.err = err
		p.ch = -1
		if err != io.EOF {
			p.tok = Invalid
		}
	}
}

// next advances the parser on the next rune.
func (p *Parser) next(skipWhite bool) bool {
	if p.err != nil {
		return false
	}

	var r rune
	var err error

	for {
		r, _, err = p.r.ReadRune()
		if err != nil {
			p.error(err)
			return false
		}
		if r == unicode.ReplacementChar {
			// invalid unicode code point
			p.error(errors.New("jsonb: invalid unicde code point"))
			return false
		}

		if !skipWhite || !isWhitespace(r) {
			break
		}
	}
	p.ch = r
	return true
}

func (p *Parser) wantComma() bool {
	l := len(p.stack)
	if l == 0 {
		return false
	}
	st := p.stack[l-1]
	return (st == stArray || st == stObjVal) &&
		p.tok >= Null && p.tok <= ArrayEnd
}

// isWhitespace returns true if the rune is a JSON whitespace.
//
// "Insignificant whitespace is allowed before or after any token.
// The whitespace characters are: character tabulation (U+0009), line
// feed (U+000A), carriage return (U+000D), and space (U+0020).
// Whitespace is not allowed within any token, except that space
// is allowed in strings."
func isWhitespace(r rune) bool {
	return r == '\n' || r == ' ' || r == '\t' || r == '\r'
}

// isInvalidInString returns true if the rune is invalid in a string
// literal.
//
// "All characters may be placed within the quotation marks except for
// the characters that must be escaped: quotation mark (U+0022),
// reverse solidus (U+005C), and the control characters U+0000 to U+001F."
//
// Even though the quotation mark and reverse solidus are handled elsewhere
// by the parser, they are checked by this function for completeness' sake.
func isInvalidInString(r rune) bool {
	return (0x0 <= r && r <= 0x1F) ||
		r == '"' ||
		r == '\\'
}

// isValidHexadecimal returns true if the rune is a valid hexadecimal
// character.
func isHexadecimal(r rune) bool {
	return ('0' <= r && r <= '9') ||
		('a' <= r && r <= 'f') ||
		('A' <= r && r <= 'F')
}

// isSeparator returns true if the rune is a valid value separator.
func isSeparator(r rune) bool {
	return isWhitespace(r) ||
		r == ':' ||
		r == ',' ||
		r == '[' ||
		r == ']' ||
		r == '{' ||
		r == '}' ||
		r == -1 // special value when an error is encountered
}
