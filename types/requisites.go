package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type NodeType string

const (
	NodeAnd         NodeType = "and"
	NodeOr          NodeType = "or"
	NodeChoose      NodeType = "choose" // at least Count of Items
	NodeCourse      NodeType = "course"
	NodeCredits     NodeType = "credits"
	NodeGPA         NodeType = "gpa"
	NodeAverage     NodeType = "average"     // average grade over the courses matching credit filters
	NodeProgram     NodeType = "program"     // enrolled in any of Programs
	NodeRequirement NodeType = "requirement" // completion: any of Requirements is Met
	NodeProfile     NodeType = "profile"     // a fact about the student other than their courses (Attr, Value)
	NodeNot         NodeType = "not"         // the one item in Items must not be met
	NodeText        NodeType = "text"
)

type TextKind string

const (
	TextPermission  TextKind = "permission"
	TextMembership  TextKind = "membership"
	TextPlacement   TextKind = "placement"
	TextPreparation TextKind = "preparation"
	TextOther       TextKind = "other"
	TextAdmission   TextKind = "admission" // decided by the department: a variable cutoff, an application, interview, audition, portfolio or test
	TextAmbiguity   TextKind = "ambiguity" // the calendar supports two readings; this node marks the branch only the lenient one accepts
)

// ReqNode is one node of a requirement tree.
type ReqNode struct {
	Type NodeType `json:"type"`

	Items []*ReqNode `json:"items,omitempty"` // and, or, choose, not (one item); average (filters, below)
	Count int        `json:"count,omitempty"` // choose

	Code     string `json:"code,omitempty"`      // course
	MinGrade int    `json:"min_grade,omitempty"` // course, credits (0 = none); average: the minimum average

	// credits: a course counts if it is in Courses or has one of the
	// Designators (the sources: when both are set, either is enough), and
	// also passes every filter that is set: Levels, Breadth, Lists,
	// Requirements, Exclude, MinGrade, Transfer and CrNcr.
	//
	// average: uses the same filters (Courses, Designators, Levels, Breadth,
	// Lists, Requirements, Exclude). The credit-weighted average percentage
	// over the matching courses must be at least MinGrade. With Amount set,
	// the student also needs Amount matching credits, and the text doesn't
	// say which of them are averaged: the average over all matching courses
	// meeting MinGrade is Met, and only the best Amount credits meeting it is
	// Review. With Items (credits nodes used only as filters), a course
	// matching any item is averaged: "2.0 credits in CRI courses at the
	// 300+-level" and "1.0 credit from CRI205H1, ..." averaged together.
	Amount      float64  `json:"amount,omitempty"`
	Designators []string `json:"designators,omitempty"`
	Levels      []int    `json:"levels,omitempty"`
	Breadth     []int    `json:"breadth,omitempty"`
	Courses     []string `json:"courses,omitempty"`
	Exclude     []string `json:"exclude,omitempty"`
	// Lists (credits, average): the course must be on one of these named
	// lists (Completion.Lists); several names are a union. A union of pools
	// that need different filters ("POL101H1/... or first-year seminars") is
	// a list whose items carry those filters.
	Lists []string `json:"lists,omitempty"`
	// Requirements: in credits and average, the course must be in the pool
	// of one of these requirements ("from Requirements 1, 3, 4"): it matches
	// the filters of that requirement's body, whether or not the requirement
	// claimed it. In a requirement node, one of these requirements must be
	// Met.
	Requirements []int `json:"requirements,omitempty"`
	// Transfer, CrNcr (credits): match only transfer credits, or only
	// courses taken CR/NCR ("CR/NCR courses cannot count" is a cap of 0).
	Transfer bool `json:"transfer,omitempty"`
	CrNcr    bool `json:"cr_ncr,omitempty"`
	// Best (average with Amount): the student's best grades are the ones
	// averaged ("the grade average will be based on the higher course
	// grades"), so the best choice of Amount credits decides.
	Best bool `json:"best,omitempty"`
	// MaxAmount (credits, nil = no limit): at most MaxAmount credits may
	// match the filters ("no more than 14.0 credits"). With both set,
	// Amount <= sum <= MaxAmount. A pointer, so that a cap of 0 is stored;
	// replace the pointer, never write through it.
	MaxAmount *float64 `json:"max_amount,omitempty"`
	// Weight (credits, list items): the credit a matched course contributes,
	// at most its own weight ("HIS208Y1 counts 0.5 toward each area"). The
	// rest of a split course can count elsewhere.
	Weight float64 `json:"weight,omitempty"`

	MinGPA float64 `json:"min_gpa,omitempty"` // gpa
	Scope  string  `json:"scope,omitempty"`   // gpa

	Programs []string `json:"programs,omitempty"` // program: catalogue codes, e.g. ASMAJ1618

	// profile: what the checker must know about the student, beyond their
	// courses and programs, to evaluate it. Attr names the fact and Value is
	// what it must be; Text is the phrase as written. The attributes in use
	// are listed in docs/schema.md ("admission_category": "Computer Science",
	// "high_school_course": "Grade 12 Calculus", ...). The name says how
	// Value is compared: a noun must equal it, has_* is a boolean ("true"),
	// min_*/max_* is a numeric limit, *_by is a date. Review until the
	// checker knows the fact.
	Attr  string `json:"attr,omitempty"`
	Value string `json:"value,omitempty"`

	Kind TextKind `json:"kind,omitempty"` // text
	Text string   `json:"text,omitempty"` // text, program, profile (the phrase as written)

	// Concurrent (any node): within this subtree a course in progress counts
	// as taken, as it does in a corequisite. "MAT257Y1 can be taken
	// concurrently", "if they are concurrently enrolled in ENG110Y1".
	Concurrent bool `json:"concurrent,omitempty"`
}

// Requisite is the stored form of a prerequisites or corequisites field.
type Requisite struct {
	Req   *ReqNode `json:"req"`
	Notes []string `json:"notes,omitempty"`
	Raw   string   `json:"raw"`
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
//   - a course set: the student is excluded if they took all of Courses;
//   - a filter (Designators set, no Courses): the student is excluded if
//     they took any course with one of the Designators that is at one of the
//     Levels (any level when empty) and is not in Exclude;
//   - a text item that can't be checked automatically.
//
// A course set or filter with a Note only gives Review when it matches. A
// Concurrent course set ("may not be taken concurrently with FSL312H1")
// matches only while its courses are in progress, not once completed.
type ExclusionItem struct {
	Courses     []string `json:"courses,omitempty"`
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
	Items []ExclusionItem `json:"items"`
	Notes []string        `json:"notes,omitempty"`
	Raw   string          `json:"raw"`
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
