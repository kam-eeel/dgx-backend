package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// EnrolmentKind says how students get into a program.
type EnrolmentKind string

const (
	EnrolmentOpen      EnrolmentKind = "open"      // meeting Req means the student may enrol
	EnrolmentLimited   EnrolmentKind = "limited"   // meeting Req means the student may apply
	EnrolmentFocus     EnrolmentKind = "focus"     // a focus inside a parent program; Req names the parent
	EnrolmentSuspended EnrolmentKind = "suspended" // not admitting; always NotMet
)

// Enrolment is the stored form of a program's enrolment requirements.
type Enrolment struct {
	Kind       EnrolmentKind `json:"kind"`
	Req        *ReqNode      `json:"req"`                  // nil only when suspended
	Exclusions []*ReqNode    `json:"exclusions,omitempty"` // programs the student must not be enrolled in
	// only first-round requests count: when to apply, not who may, so not in Req
	FirstRoundOnly bool     `json:"first_round_only,omitempty"`
	Notes          []string `json:"notes,omitempty"`
	Raw            string   `json:"raw"`
}

func (e Enrolment) Value() (driver.Value, error) {
	b, err := json.Marshal(e)
	return string(b), err
}

func (e *Enrolment) Scan(src any) error {
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, e)
	case string:
		return json.Unmarshal([]byte(v), e)
	}
	return fmt.Errorf("Enrolment: unsupported type %T", src)
}

// Completion is the stored form of a program's completion requirements.
type Completion struct {
	Requirements []*Requirement `json:"requirements"`         // numbered 1..N in DE order
	Lists        []*CourseList  `json:"lists,omitempty"`      // named course lists
	Exclusions   []*ReqNode     `json:"exclusions,omitempty"` // programs that can't be combined with this one
	Notes        []string       `json:"notes,omitempty"`      // program-level advisory text
	Raw          string         `json:"raw"`
}

// RequirementKind says what a requirement row does with the student's
// courses.
type RequirementKind string

const (
	RequirementAllocation RequirementKind = "allocation" // claims courses
	RequirementConstraint RequirementKind = "constraint" // a floor or must-include on the courses counted under Parent
	RequirementLimit      RequirementKind = "limit"      // at most MaxAmount matching credits count toward Parent
	RequirementChoice     RequirementKind = "choice"     // picks among option requirements; top level only
)

// ScopeKind says which courses a constraint or limit counts over.
type ScopeKind string

const (
	// ScopeTranscript: every course on the transcript, not only the parent's.
	ScopeTranscript ScopeKind = "transcript"
)

// Requirement is one numbered row: DE's "Requirement N".
type Requirement struct {
	Number   int             `json:"number"` // 1..N
	Kind     RequirementKind `json:"kind"`
	Label    string          `json:"label,omitempty"`    // the calendar's own label: "1a"
	Section  string          `json:"section,omitempty"`  // the heading it sits under; display only
	Parent   int             `json:"parent,omitempty"`   // DE's "in ReqK" (0 = top level)
	Overlaps []int           `json:"overlaps,omitempty"` // allocations under the same parent that may share a course
	Req      *ReqNode        `json:"req"`                // nil only for an option group
	Audience *ReqNode        `json:"audience,omitempty"` // profile node the student must match (nil = everyone)
	Scope    ScopeKind       `json:"scope,omitempty"`    // courses a constraint or limit counts over ("" = the parent's)
	Notes    []string        `json:"notes,omitempty"`    // advisory text after this requirement
	Raw      string          `json:"raw"`                // the calendar sentences it came from
}

// CourseList is a named list. A course is on it if it matches any item.
type CourseList struct {
	Name  string     `json:"name"`            // how the text refers to it: "Group E"; unique in the program
	Title string     `json:"title,omitempty"` // the full heading, when it differs
	Items []*ReqNode `json:"items"`           // credits filters, or and(filter, text) for "maybe" members
}

// Value stores c as JSON, with nil slices as [] rather than null.
func (c Completion) Value() (driver.Value, error) {
	if c.Requirements == nil {
		c.Requirements = []*Requirement{}
	}
	if c.Lists != nil {
		lists := make([]*CourseList, len(c.Lists))
		for i, l := range c.Lists {
			if l != nil && l.Items == nil {
				cp := *l
				cp.Items = []*ReqNode{}
				l = &cp
			}
			lists[i] = l
		}
		c.Lists = lists
	}
	b, err := json.Marshal(c)
	return string(b), err
}

func (c *Completion) Scan(src any) error {
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, c)
	case string:
		return json.Unmarshal([]byte(v), c)
	}
	return fmt.Errorf("Completion: unsupported type %T", src)
}
