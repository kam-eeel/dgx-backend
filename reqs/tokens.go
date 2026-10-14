package reqs

import "degree-explorer/types"

type tokKind int

const (
	tEOF         tokKind = iota // returned by the parser past the last token
	tCourse                     // CSC148H1, STA247H1(70%), DRM220Y1 with a minimum of 70%
	tGradePrefix                // "75% or higher in", "A final grade of 75% or higher in each of"
	tGradeSuffix                // "(...) with a minimum grade of 68% in each", "(minimum 70%)" after a group
	tCredits                    // credit phrases, any-course, year standing, breadth
	tGPA                        // "Minimum GPA of 3.5 in Astronomy program courses"
	tAvgSuffix                  // "(average of both must be 80%)" after a group
	tPool                       // "one of", "1.0 credit from:", "of which at least 0.5 credits in"
	tCond                       // "For Specialist:"
	tSemi                       // ;
	tComma                      // ,
	tCommaAnd                   // ", and"  ", plus"  ", including"
	tCommaOr                    // ", or"
	tAnd                        // and  &  plus  including
	tOr                         // or
	tSlash                      // /
	tLParen                     // (
	tRParen                     // )
	tText                       // fallback: a run of words
)

type token struct {
	kind       tokKind
	start, end int            // byte span in the source string
	node       *types.ReqNode // prebuilt leaf for course / credits / gpa
	grade      int            // tGradePrefix, tGradeSuffix, tAvgSuffix
	each       bool           // tGradePrefix: "in each of"; tGradeSuffix: "in each (course)"
	amount     float64        // tPool: credit amount ("1.0 credit from:"), else 0
	count      int            // tPool: how many items ("one of" = 1, "two of" = 2); tAvgSuffix: courses named ("both" = 2, else 0)
}
