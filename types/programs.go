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
	Exclusions []*ReqNode    `json:"exclusions,omitempty"` // program/text nodes the student must not be enrolled in
	// FirstRoundOnly: requests are only considered in the first program
	// request period. When to apply, not who may, so it isn't part of Req.
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
