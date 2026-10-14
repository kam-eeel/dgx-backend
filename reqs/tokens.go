package reqs

import "degree-explorer/types"

type tokKind int

const (
	tEOF         tokKind = iota // returned by the parser past the last token
	tCourse                     // CSC148H1, STA247H1(70%), DRM220Y1 with a minimum of 70%
	tGradePrefix                // "75% or higher in", "A final grade of 75% or higher in each of"
	tCredits                    // credit phrases, any-course, year standing, breadth
	tGPA                        // "Minimum GPA of 3.5 in Astronomy program courses"
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
	grade      int            // tGradePrefix
	each       bool           // tGradePrefix: "in each of"
	amount     float64        // tPool: 0 for "one of"
}
