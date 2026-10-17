package parser

import (
	"testing"

	"degree-explorer/parser/grammar"
	"degree-explorer/types"
)

func TestLink(t *testing.T) {
	ast := &types.Course{Code: "AST430H1",
		Prerequisites: grammar.ParseRequisite("AST320H1. Course should not be taken concurrently with AST425Y1.")}
	mat := &types.Course{Code: "MAT309H1", Prerequisites: grammar.ParseRequisite("MAT246H1/ (MAT1581, MAT159H1)")}
	courses := []*types.Course{ast, mat,
		{Code: "MAT158H1"}, {Code: "MAT159H1"}, {Code: "MAT246H1"}, {Code: "AST320H1"}, {Code: "AST425Y1"}}
	Link(courses, nil)

	assertJSON(t, ast.Exclusions, `{"raw":"","items":[{"courses":["AST425Y1"],"concurrent":true}],"notes":["Course should not be taken concurrently with AST425Y1"]}`)
	assertJSON(t, mat.Prerequisites.Req, jOr(jC("MAT246H1"), jAnd(jC("MAT158H1"), jC("MAT159H1"))))
	if mat.Prerequisites.Raw != "MAT246H1/ (MAT1581, MAT159H1)" {
		t.Errorf("raw changed: %q", mat.Prerequisites.Raw)
	}
}
