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
	MinGrade int    `json:"min_grade,omitempty"` // course, credits (0 = none)

	// credits: a course counts if it is in Courses or has one of the
	// Designators (when both are set, either is enough), and also passes
	// Levels, Breadth, Exclude and MinGrade.
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
// A codes set or filter with a Note only gives Review when it matches.
type ExclusionItem struct {
	Codes       []string `json:"codes,omitempty"`
	Designators []string `json:"designators,omitempty"`
	Levels      []int    `json:"levels,omitempty"`
	Exclude     []string `json:"exclude,omitempty"`
	Note        string   `json:"note,omitempty"`
	Kind        TextKind `json:"kind,omitempty"`
	Text        string   `json:"text,omitempty"`
}

// Exclusions is the stored form of an exclusions field.
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
