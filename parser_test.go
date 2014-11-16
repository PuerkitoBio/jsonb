package jsonb

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestParser(t *testing.T) {
	cases := []struct {
		in    string
		toks  []Token
		bytes []string
		err   error
	}{
		{in: ""},

		// true, false and null literal names
		{in: "z", toks: []Token{Invalid}, bytes: []string{""}, err: &SyntaxError{Char: 'z', typ: begVal}},
		{in: "null", toks: []Token{Null}, bytes: []string{"null"}},
		{in: "nall", toks: []Token{Invalid}, bytes: []string{"n"}, err: &LiteralError{want: 'u', got: 'a', tok: Null}},
		{in: "t", toks: []Token{Invalid}, bytes: []string{"t"}, err: &LiteralError{want: 'r', got: -1, tok: True}},
		{in: "tue", toks: []Token{Invalid}, bytes: []string{"t"}, err: &LiteralError{want: 'r', got: 'u', tok: True}},
		{in: "true", toks: []Token{True}, bytes: []string{"true"}},
		{in: "fa", toks: []Token{Invalid}, bytes: []string{"fa"}, err: &LiteralError{want: 'l', got: -1, tok: False}},
		{in: "fz", toks: []Token{Invalid}, bytes: []string{"f"}, err: &LiteralError{want: 'a', got: 'z', tok: False}},
		{in: "fals", toks: []Token{Invalid}, bytes: []string{"fals"}, err: &LiteralError{want: 'e', got: -1, tok: False}},
		{in: "false", toks: []Token{False}, bytes: []string{"false"}},
		{in: "falsez", toks: []Token{Invalid}, bytes: []string{"false"}, err: &SyntaxError{Char: 'z', typ: endLit}},
		{in: "truez", toks: []Token{Invalid}, bytes: []string{"true"}, err: &SyntaxError{Char: 'z', typ: endLit}},
		{in: "nullz", toks: []Token{Invalid}, bytes: []string{"null"}, err: &SyntaxError{Char: 'z', typ: endLit}},

		// string literals
		{in: `""`, toks: []Token{String}, bytes: []string{`""`}},
		{in: `"a"`, toks: []Token{String}, bytes: []string{`"a"`}},
		{in: `"a b 1"`, toks: []Token{String}, bytes: []string{`"a b 1"`}},
		{in: `"\n"`, toks: []Token{String}, bytes: []string{`"\n"`}},
		{in: `"\"\\\/\b\f\n\r\t"`, toks: []Token{String}, bytes: []string{`"\"\\\/\b\f\n\r\t"`}},
		{in: `"\u001b"`, toks: []Token{String}, bytes: []string{`"\u001b"`}},
		{in: `"\uAbC9"`, toks: []Token{String}, bytes: []string{`"\uAbC9"`}},
		{in: `"\udEfF"`, toks: []Token{String}, bytes: []string{`"\udEfF"`}},
		{in: `"\z"`, toks: []Token{Invalid}, bytes: []string{`"\`}, err: &SyntaxError{Char: 'z', typ: chrEsc}},
		{in: `"\uab_e"`, toks: []Token{Invalid}, bytes: []string{`"\uab`}, err: &SyntaxError{Char: '_', typ: hexEsc}},

		// number literals
		{in: `0`, toks: []Token{Number}, bytes: []string{`0`}},
		{in: `1234567890`, toks: []Token{Number}, bytes: []string{`1234567890`}},
		{in: `-1234567890`, toks: []Token{Number}, bytes: []string{`-1234567890`}},
		{in: `-01`, toks: []Token{Invalid}, bytes: []string{`-0`}, err: &SyntaxError{Char: '1', typ: zroLit}},
		{in: `01`, toks: []Token{Invalid}, bytes: []string{`0`}, err: &SyntaxError{Char: '1', typ: zroLit}},
		{in: `0a`, toks: []Token{Invalid}, bytes: []string{`0`}, err: &SyntaxError{Char: 'a', typ: endLit}},
		{in: `1a`, toks: []Token{Invalid}, bytes: []string{`1`}, err: &SyntaxError{Char: 'a', typ: endLit}},
		{in: `1.2`, toks: []Token{Number}, bytes: []string{`1.2`}},
		{in: `0.2`, toks: []Token{Number}, bytes: []string{`0.2`}},
		{in: `-0.123`, toks: []Token{Number}, bytes: []string{`-0.123`}},
		{in: `-4567890.123`, toks: []Token{Number}, bytes: []string{`-4567890.123`}},
		{in: `1.2.3`, toks: []Token{Invalid}, bytes: []string{`1.2`}, err: &SyntaxError{Char: '.', typ: endLit}},
		{in: `-0.123e+124`, toks: []Token{Number}, bytes: []string{`-0.123e+124`}},
		{in: `-0.123E-001`, toks: []Token{Number}, bytes: []string{`-0.123E-001`}},
		{in: `123E+2`, toks: []Token{Number}, bytes: []string{`123E+2`}},
		{in: `123E+2e`, toks: []Token{Invalid}, bytes: []string{`123E+2`}, err: &SyntaxError{Char: 'e', typ: endLit}},
		{in: `123E+-1`, toks: []Token{Invalid}, bytes: []string{`123E+`}, err: &SyntaxError{Char: '-', typ: endLit}},
		{in: `-`, toks: []Token{Invalid}, bytes: []string{`-`}, err: &SyntaxError{Char: -1, typ: endLit}},
		{in: `123.`, toks: []Token{Invalid}, bytes: []string{`123.`}, err: &SyntaxError{Char: -1, typ: endLit}},
		{in: `123.4e`, toks: []Token{Invalid}, bytes: []string{`123.4e`}, err: &SyntaxError{Char: -1, typ: endLit}},
		{in: `123.4e-`, toks: []Token{Invalid}, bytes: []string{`123.4e-`}, err: &SyntaxError{Char: -1, typ: endLit}},

		// array
		{in: `[]`, toks: []Token{ArrayStart, ArrayEnd}, bytes: []string{"[", "]"}},
		{in: `[true]`, toks: []Token{ArrayStart, True, ArrayEnd}, bytes: []string{"[", "true", "]"}},
		{in: `[true, 1, "a"]`, toks: []Token{ArrayStart, True, Number, String, ArrayEnd}, bytes: []string{"[", "true", "1", `"a"`, "]"}},
		{in: `[true, , 1]`, toks: []Token{ArrayStart, True, Invalid}, bytes: []string{"[", "true", ""}, err: &SyntaxError{Char: ',', typ: begVal}},
		//{in: `[,1]`, toks: []Token{ArrayStart, Invalid}, bytes: []string{"[", ""}, err: &SyntaxError{Char: ',', typ: begVal}},
		{in: `true, , 1]`, toks: []Token{True, Invalid}, bytes: []string{"true", ""}, err: &SyntaxError{Char: ',', typ: begVal}},
		{in: `[true, 1, "a",  [  false, 2, "b" ],   null]`, toks: []Token{ArrayStart, True, Number, String,
			ArrayStart, False, Number, String, ArrayEnd, Null, ArrayEnd}, bytes: []string{"[", "true", "1", `"a"`,
			"[", "false", "2", `"b"`, "]", "null", "]"}},
	}

	p := NewParser(nil)
	for i, c := range cases {
		p.Reset(strings.NewReader(c.in))

		var j int
		for p.Next() {
			gott := p.Token()
			if j >= len(c.toks) {
				t.Errorf("%d (%s): unexpected token %s at index %d", i, c.in, gott, j)
			} else if gott != c.toks[j] {
				t.Errorf("%d (%s): want %s, got %s at index %d (%q)", i, c.in, c.toks[j], gott, j, string(p.Bytes()))
			} else {
				gotb := p.Bytes()
				if bytes.Compare(gotb, []byte(c.bytes[j])) != 0 {
					t.Errorf("%d (%s): want %s, got %s at index %d", i, c.in, c.bytes[j], string(gotb), j)
				}
			}

			j++
		}

		if err := p.Err(); !reflect.DeepEqual(c.err, err) {
			t.Errorf("%d (%s): want %v, got error %v", i, c.in, c.err, err)
		}
	}
}
