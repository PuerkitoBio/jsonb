// Package jsonb implements a memory-efficient JSON parser based on the
// ECMA-404 1st edition specification [1].
//
// All valid bytes up to an error are guaranteed to be returned by calls to
// Parser.Bytes. An invalid byte causes an error an is not returned in the call
// to Parser.Bytes, but bytes up to the error are returned, along with the Token
// type Invalid, before Parser.Next returns false.
//
// [1] http://www.ecma-international.org/publications/files/ECMA-ST/ECMA-404.pdf.
package jsonb
