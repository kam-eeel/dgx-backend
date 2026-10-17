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
	NodeAverage     NodeType = "average"     // average grade over the matching courses
	NodeProgram     NodeType = "program"     // enrolled in any of Programs
	NodeRequirement NodeType = "requirement" // any of Requirements is Met
	NodeProfile     NodeType = "profile"     // a non-course fact about the student
	NodeNot         NodeType = "not"         // the one item must not be met
	NodeText        NodeType = "text"
)

type TextKind string

const (
	TextPermission  TextKind = "permission"
	TextMembership  TextKind = "membership"
	TextPlacement   TextKind = "placement"
	TextPreparation TextKind = "preparation"
	TextOther       TextKind = "other"
	TextAdmission   TextKind = "admission" // the department decides (application, audition)
	TextAmbiguity   TextKind = "ambiguity" // the branch only the lenient reading accepts
	TextContent     TextKind = "content"   // course content decides; not in the catalogue
)

// ReqNode is one node of a requirement tree.
type ReqNode struct {
	Type NodeType `json:"type"`

	Items []*ReqNode `json:"items,omitempty"` // and, or, choose, not; average: filters
	Count int        `json:"count,omitempty"` // choose

	Code     string `json:"code,omitempty"`      // course
	MinGrade int    `json:"min_grade,omitempty"` // minimum grade or average (0 = none)

	// credits: a course counts if it is in Courses or has one of Designators
	// and passes every filter set. average: the credit-weighted average of the
	// matching courses (or of any of Items) is at least MinGrade; with Amount,
	// averaging only the best Amount credits is Review.
	Amount      float64  `json:"amount,omitempty"`
	Designators []string `json:"designators,omitempty"`
	Levels      []int    `json:"levels,omitempty"`
	Breadth     []int    `json:"breadth,omitempty"`
	Courses     []string `json:"courses,omitempty"`
	Exclude     []string `json:"exclude,omitempty"`
	// courses with one of these designators never count
	ExcludeDesignators []string `json:"exclude_designators,omitempty"`
	// only courses worth at most this many credits match (0 = none)
	MaxCourseAmount float64 `json:"max_course_amount,omitempty"`
	// the course is on one of these named lists (Completion.Lists)
	Lists []string `json:"lists,omitempty"`
	// in one of these requirements' pools; in a requirement node, one is Met
	Requirements []int `json:"requirements,omitempty"`
	Transfer     bool  `json:"transfer,omitempty"` // only transfer credits
	CrNcr        bool  `json:"cr_ncr,omitempty"`   // only courses taken CR/NCR
	// average the best grades, so the best Amount credits decide
	Best bool `json:"best,omitempty"`
	// at most this many credits (nil = no limit); replace, never write through
	MaxAmount *float64 `json:"max_amount,omitempty"`
	// credit a matched course or an and(course, course) pair item contributes
	Weight float64 `json:"weight,omitempty"`

	MinGPA float64 `json:"min_gpa,omitempty"` // gpa
	Scope  string  `json:"scope,omitempty"`   // gpa

	Programs []string `json:"programs,omitempty"` // program: catalogue codes

	// profile: Attr names the fact and Value what it must be: has_* is a
	// boolean, min_*/max_* a numeric limit, *_by a date.
	Attr  string `json:"attr,omitempty"`
	Value string `json:"value,omitempty"`

	Kind TextKind `json:"kind,omitempty"` // text
	Text string   `json:"text,omitempty"` // text, program, profile (the phrase as written)

	// within this subtree a course in progress counts as taken
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

// ExclusionItem is a course set (excluded if all of Courses were taken), a
// filter (Designators, Levels, Exclude) or text. A match with a Note gives
// Review; a Concurrent one matches only while its courses are in progress.
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

// Exclusions is the stored form of an exclusions field. Raw is empty when the
// items came from a timing restriction in the requisites.
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
