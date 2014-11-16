package jsonb

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"testing"
)

var jsonTpl = `{
	"field_a": "test",
	"field_b": 123,
	"field_c": true,
	"field_d": null,
	"field_e": %q
}`

var (
	jsonEEmpty []byte
	jsonE1K    []byte
	jsonE1M    []byte
)

func init() {
	jsonEEmpty = []byte(fmt.Sprintf(jsonTpl, ""))

	lr := &io.LimitedReader{rand.Reader, 1 << 10}
	e1k, err := ioutil.ReadAll(lr)
	if err != nil {
		panic(err)
	}
	jsonE1K = []byte(fmt.Sprintf(jsonTpl, base64.StdEncoding.EncodeToString(e1k)))

	lr.N = 1 << 20
	e1m, err := ioutil.ReadAll(lr)
	if err != nil {
		panic(err)
	}
	jsonE1M = []byte(fmt.Sprintf(jsonTpl, base64.StdEncoding.EncodeToString(e1m)))
}

type fieldsAD struct {
	A string `json:"field_a"`
	B int    `json:"field_b"`
	C bool   `json:"field_c"`
	D string `json:"field_d"`
}

type fieldsAE struct {
	fieldsAD
	E string `json:"field_e"`
}

func BenchmarkAD(b *testing.B) {
	var dst fieldsAD

	for i := 0; i < b.N; i++ {
		err := json.Unmarshal(jsonEEmpty, &dst)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAEIgnoreE(b *testing.B) {
	var dst fieldsAD

	for i := 0; i < b.N; i++ {
		err := json.Unmarshal(jsonE1M, &dst)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAE1K(b *testing.B) {
	var dst fieldsAE

	for i := 0; i < b.N; i++ {
		err := json.Unmarshal(jsonE1K, &dst)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAE1M(b *testing.B) {
	var dst fieldsAE

	for i := 0; i < b.N; i++ {
		err := json.Unmarshal(jsonE1M, &dst)
		if err != nil {
			b.Fatal(err)
		}
	}
}
