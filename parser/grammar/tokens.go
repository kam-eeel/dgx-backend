package grammar

import (
	"degree-explorer/types"
)

type TokKind int

const (
	tEOF         TokKind = iota // returned by the parser past the last token
	TCourse                     // CSC148H1, STA247H1(70%), DRM220Y1 with a minimum of 70%
	tGradePrefix                // "75% or higher in", "A final grade of 75% or higher in each of"
	tGradeSuffix                // "(...) with a minimum grade of 68% in each", "(minimum 70%)" after a group
	tCredits                    // credit phrases, any-course, year standing, breadth
	tGPA                        // "Minimum GPA of 3.5 in Astronomy program courses"
	tAvgSuffix                  // "(average of both must be 80%)" after a group
	tPool                       // "one of", "1.0 credit from:", "of which at least 0.5 credits in"
	TCond                       // "For Specialist:"
	TSemi                       // ;
	TComma                      // ,
	TCommaAnd                   // ", and"  ", plus"  ", including"
	TCommaOr                    // ", or"
	TAnd                        // and  &  plus  including
	TOr                         // or
	TSlash                      // /
	TLParen                     // (
	TRParen                     // )
	TText                       // fallback: a run of words

	// Completion text only (LexCompletion):
	TWild    // a pool member named by designator and level: "PSL400-series courses", "EEB (excluding ...)"
	tEither  // "Either" before alternatives: "Either A or B"
	tSkip    // words that only restate the grammar ("All of:"); dropped by LexCompletion
	tExclude // ", excluding JPH311H1, JPH441H1" after a list
)

type Token struct {
	Kind       TokKind
	Start, End int            // byte span in the source string
	Node       *types.ReqNode // prebuilt leaf for course / credits / gpa; tPool (completion): levels that narrow every member
	grade      int            // tGradePrefix, tGradeSuffix, tAvgSuffix
	each       bool           // tGradePrefix: "in each of"; tGradeSuffix: "in each (course)"
	amount     float64        // tPool: credit amount ("1.0 credit from:"), else 0
	upper      float64        // tPool (completion): the upper end of a range ("2.0 to 2.5 credits from"), else 0
	sum        bool           // tPool (completion): "2.0 credits: A; B/ C; D", a list whose items are all needed when they add up to the amount
	count      int            // tPool: how many items ("one of" = 1, "two of" = 2); tAvgSuffix: courses named ("both" = 2, else 0)
	codes      []string       // tExclude: the excluded codes
}
