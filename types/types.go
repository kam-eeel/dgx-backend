package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type Campus string
type ParseResult bool

const (
	ONLINE          Campus      = "online"
	UTSG            Campus      = "utsg"
	UTM             Campus      = "utm"
	UTSC            Campus      = "utsc"
	ParseSuccessful ParseResult = true
	ParseFailure    ParseResult = false
)

// UTSGBreadthCategories are the breadth categories as the calendar names
// them; category n is UTSGBreadthCategories[n-1].
var UTSGBreadthCategories = []string{
	"Creative and Cultural Representations (1)",
	"Thought, Belief and Behaviour (2)",
	"Society and its Institutions (3)",
	"Living Things and Their Environment (4)",
	"The Physical and Mathematical Universes (5)",
}

type StringList []string

func (l StringList) Value() (driver.Value, error) {
	if l == nil {
		return nil, nil // SQL NULL
	}
	b, err := json.Marshal(l)
	return string(b), err
}

func (l *StringList) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*l = nil
		return nil
	case []byte:
		return json.Unmarshal(v, l)
	case string:
		return json.Unmarshal([]byte(v), l)
	}
	return fmt.Errorf("StringList: unsupported type %T", src)
}

// IntList is a list of ints stored as a JSON array.
type IntList []int

func (l IntList) Value() (driver.Value, error) {
	if l == nil {
		return nil, nil // SQL NULL
	}
	b, err := json.Marshal(l)
	return string(b), err
}

func (l *IntList) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*l = nil
		return nil
	case []byte:
		return json.Unmarshal(v, l)
	case string:
		return json.Unmarshal([]byte(v), l)
	}
	return fmt.Errorf("IntList: unsupported type %T", src)
}

type Course struct {
	Code          string      `json:"code"`       // ex. CSC148H1
	Designator    string      `json:"designator"` // ex. CSC
	Number        string      `json:"number"`     // ex. 148
	Level         int64       `json:"level"`      // 100-400
	Weight        float64     `json:"weight"`     // 0.5 or 1.0
	Campus        string      `json:"campus"`     // online (0), utsg (1), utsc (3), utm (5)
	Title         string      `json:"title"`
	Description   string      `json:"description"`
	Prerequisites *Requisite  `json:"prerequisites"`
	Corequisites  *Requisite  `json:"corequisites"`
	Exclusions    *Exclusions `json:"exclusions"`
	Breadth       IntList     `json:"breadth"` // breadth categories 1-5 (UTSGBreadthCategories)
}

type Program struct {
	Code       string      `json:"code"`
	Type       string      `json:"type"`
	Number     string      `json:"number"`
	Title      string      `json:"title"`
	Enrolment  *Enrolment  `json:"enrolment"`
	Completion *Completion `json:"completion"`
	Areas      StringList  `json:"areas"`
}
