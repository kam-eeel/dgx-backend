package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type NodeType string

const (
	NodeAnd     NodeType = "and"
	NodeOr      NodeType = "or"
	NodeChoose  NodeType = "choose" // at least Count of Items
	NodeCourse  NodeType = "course"
	NodeCredits NodeType = "credits"
	NodeGPA     NodeType = "gpa"
	NodeAverage NodeType = "average" // average grade over the courses matching credit filters
	NodeProgram NodeType = "program" // enrolled in any of Programs
	NodeText    NodeType = "text"
)

type TextKind string

const (
	KindPermission  TextKind = "permission"
	KindProgram     TextKind = "program"
	KindPlacement   TextKind = "placement"
	KindPreparation TextKind = "preparation"
	KindOther       TextKind = "other"
)

// ReqNode is one node of a requirement tree.
type ReqNode struct {
	Type NodeType `json:"type"`

	Items []*ReqNode `json:"items,omitempty"` // and, or, choose
	Count int        `json:"count,omitempty"` // choose

	Code     string `json:"code,omitempty"`      // course
	MinGrade int    `json:"min_grade,omitempty"` // course, credits (0 = none); average: the minimum average

	// credits: a course counts if it is in Courses or has one of the
	// Designators (when both are set, either is enough), and also passes
	// Levels, Breadth, Exclude and MinGrade.
	//
	// average: uses the same filters (Courses, Designators, Levels, Breadth,
	// Exclude). The credit-weighted average percentage over the matching
	// courses must be at least MinGrade. With Amount set, the student also
	// needs Amount matching credits, and the text doesn't say which of them
	// are averaged: the average over all matching courses meeting MinGrade is
	// Met, and only the best Amount credits meeting it is Review.
	Amount      float64  `json:"amount,omitempty"`
	Designators []string `json:"designators,omitempty"`
	Levels      []int    `json:"levels,omitempty"`
	Breadth     []int    `json:"breadth,omitempty"`
	Courses     []string `json:"courses,omitempty"`
	Exclude     []string `json:"exclude,omitempty"`
	// Max (credits, 0 = no limit): at most Max credits may match the filters
	// ("no more than 14.0 credits"). With both set, Amount <= sum <= Max.
	Max float64 `json:"max,omitempty"`

	Min   float64 `json:"min,omitempty"`   // gpa
	Scope string  `json:"scope,omitempty"` // gpa

	Programs []string `json:"programs,omitempty"` // program: catalogue codes, e.g. ASMAJ1618

	Kind TextKind `json:"kind,omitempty"` // text
	Text string   `json:"text,omitempty"` // text, program (the phrase as written)

	// Concurrent (any node): within this subtree a course in progress counts
	// as taken, as it does in a corequisite. "MAT257Y1 can be taken
	// concurrently", "if they are concurrently enrolled in ENG110Y1".
	Concurrent bool `json:"concurrent,omitempty"`
}

// Requisite is the stored form of a prerequisites or corequisites field.
type Requisite struct {
	Raw   string   `json:"raw"`
	Req   *ReqNode `json:"req"`
	Notes []string `json:"notes,omitempty"`
}

func (r Requisite) Value() (driver.Value, error) {
	b, err := json.Marshal(r)
	return string(b), err
}

func (r *Requisite) Scan(src any) error {
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, r)
	case string:
		return json.Unmarshal([]byte(v), r)
	}
	return fmt.Errorf("Requisite: unsupported type %T", src)
}

// ExclusionItem is one of:
//   - a codes set: the student is excluded if they took all of Codes;
//   - a filter (Designators set, no Codes): the student is excluded if they
//     took any course with one of the Designators that is at one of the
//     Levels (any level when empty) and is not in Exclude;
//   - a text item that can't be checked automatically.
//
// A codes set or filter with a Note only gives Review when it matches. A
// Concurrent codes set ("may not be taken concurrently with FSL312H1")
// matches only while its courses are in progress, not once completed.
type ExclusionItem struct {
	Codes       []string `json:"codes,omitempty"`
	Designators []string `json:"designators,omitempty"`
	Levels      []int    `json:"levels,omitempty"`
	Exclude     []string `json:"exclude,omitempty"`
	Concurrent  bool     `json:"concurrent,omitempty"`
	Note        string   `json:"note,omitempty"`
	Kind        TextKind `json:"kind,omitempty"`
	Text        string   `json:"text,omitempty"`
}

// Exclusions is the stored form of an exclusions field. Raw is empty when
// the field was empty and its items came from a timing restriction in the
// requisites ("should not be taken concurrently with AST425Y1").
type Exclusions struct {
	Raw   string          `json:"raw"`
	Items []ExclusionItem `json:"items"`
	Notes []string        `json:"notes,omitempty"`
}

func (e Exclusions) Value() (driver.Value, error) {
	b, err := json.Marshal(e)
	return string(b), err
}

func (e *Exclusions) Scan(src any) error {
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, e)
	case string:
		return json.Unmarshal([]byte(v), e)
	}
	return fmt.Errorf("Exclusions: unsupported type %T", src)
}
