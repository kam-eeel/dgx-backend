// Package parser parses calendar requisite and requirement text into
// requirement trees. The parsers live in its subpackages; Link joins their
// results to the catalogue.
package parser

import (
	"degree-explorer/parser/courses"
	"degree-explorer/parser/programs/completion"
	"degree-explorer/parser/programs/enrolment"
	"degree-explorer/types"
)

// ParseRequisite parses a course's prerequisites or corequisites field.
func ParseRequisite(raw string) *types.Requisite { return courses.ParseRequisite(raw) }

// ParseExclusions parses a course's exclusions field.
func ParseExclusions(raw string) *types.Exclusions { return courses.ParseExclusions(raw) }

// ParseEnrolment parses a program's enrolment requirements.
func ParseEnrolment(raw string) *types.Enrolment { return enrolment.Parse(raw) }

// ParseCompletion parses a program's completion requirements.
func ParseCompletion(raw string) *types.Completion { return completion.Parse(raw) }
