package reqs

import (
	"regexp"
	"strings"

	"degree-explorer/types"
)

// Proper names and subject lists contain commas, "and" and "or" that the
// operator matchers would otherwise read as requirement operators: "approval
// of the Centre for Drama, Theatre and Performance Studies (CDTPS)", "from
// Group C: Society, Culture, and Language", "1.0 credit in political theory,
// history of science, or intellectual history". joinTitleRuns rejoins such
// runs of text tokens into a single text token before parsing, so they stay
// one text node and never split structured siblings apart.
//
// Only text tokens and separators are ever joined, so a run never swallows a
// course, credit amount or other structured token.

var (
	// titleRejectRe marks an item that reads as a requirement of its own, not
	// as part of a name: "permission of the Department", "Spanish placement
	// test", "Independent Study Application Form".
	titleRejectRe = regexp.MustCompile(`(?i)\b(?:permission|approv|consent|admission|admitted|enrol|registra|complet|students?\b|instructor|required|requirements?\b|placement|tests?\b|equivalent|interview|audition|applica|minimum|grade|credits?\b|level|years?\b|standing|average|gpa\b|cgpa\b|proficien|knowledge|fluen|recommend|experience|must\b|will\b|should\b|may\b|only\b|not\b|who\b|which\b|that\b)`)

	// designatorWordRe is a bare three-letter designator ("SOC"), which is a
	// code, not a name. Acronyms in brackets ("(HPS)") are fine.
	designatorWordRe = regexp.MustCompile(`^[A-Z]{3}$`)
	acronymRe        = regexp.MustCompile(`^[A-Z]{2,}$`) // "(HPS)", "(CDTPS)"

	// subjectRejectRe marks a lower-case alternative that isn't a subject:
	// "1.0 credit in history or demonstrated interest".
	subjectRejectRe = regexp.MustCompile(`(?i)\b(?:interest|relevant|related|similar|demonstrat|appropriate|suitable|department|faculty|other|any)`)

	// subjectLeadRe starts a subject qualifier right after a credit phrase:
	// "1.0 credit in ...", "1.0 credit on ...".
	subjectLeadRe = regexp.MustCompile(`^(?i:in|on)\s`)
)

// nameConnectors may appear in lower case inside a capitalised name:
// "Critical Studies in Equity", "Faculty of Arts & Science". "and" and "or"
// only show up inside items that were joined already.
var nameConnectors = wordSet(`of the for in on at to & and or de des du la le et`)

// nameHeadNouns may end the last name in a list in lower case: "Science,
// Technology, and Society minor", "History, or Philosophy programs of study",
// "European or Asian history".
var nameHeadNouns = wordSet(`history histories minor minors major majors specialist specialists
	program programs programme programmes studies course courses group groups stream streams`)

// joinTitleRuns returns toks with name runs joined into single text tokens.
func joinTitleRuns(src string, toks []token) []token {
	toks = joinParenNames(src, toks)
	for i := 0; i < len(toks); i++ {
		if toks[i].kind != tText || isProgramToken(src, toks[i]) {
			continue
		}
		subject := i > 0 && toks[i-1].kind == tCredits && subjectLeadRe.MatchString(src[toks[i].start:])
		// Retry at i after each join: "History and Philosophy of Science
		// (HPS), Science, Technology and Society (STS), History, or
		// Philosophy" is a pair followed by two series.
		for {
			k := nameRunEnd(src, toks, i, subject)
			if k == i {
				break
			}
			toks = joinTokens(toks, i, k)
		}
	}
	return toks
}

// joinParenNames joins a bracketed list of capitalised names to the text right
// before it: "Intermediate-level Hebrew (Modern or Biblical)", "from Group C
// (Society, Culture and Language)". The bracket is one unit with that text
// anyway, so only text nodes change.
func joinParenNames(src string, toks []token) []token {
	for i := 0; i+2 < len(toks); i++ {
		a := toks[i]
		if a.kind != tText || toks[i+1].kind != tLParen || desigItemRe.MatchString(src[a.start:a.end]) {
			continue
		}
		k, items := i+2, 0
	scan:
		for ; k < len(toks); k++ {
			switch t := toks[k]; t.kind {
			case tText:
				// "course(s)": a plural marker right after the word
				plural := src[t.start:t.end] == "s" && toks[i+1].start == a.end
				if s := src[t.start:t.end]; !parenItem(s) && !plural {
					break scan
				}
				items++
			case tComma, tCommaAnd, tCommaOr, tAnd, tOr, tSlash, tSemi:
			default:
				break scan
			}
		}
		if items > 0 && k < len(toks) && toks[k].kind == tRParen && toks[k-1].kind == tText {
			toks = joinTokens(toks, i, k)
		}
	}
	return toks
}

// nameRunEnd returns the index of the last token of a name run that starts
// with the text token at i, or i if there is none. A run is
//
//	T0 ("and" | "or" | "/") T1             a pair: "Near and Middle Eastern Civilization"
//	T0 "," T1 { "," Tn } CONJ Tlast        a series: "Society, Culture, and Language"
//
// where T0 ends in a capitalised word, every other item is a name (see
// nameItem) and CONJ is "and", "or", ", and" or ", or". ", and" / ", or" only
// close a series: after a pair or a complete series they start a new
// requirement ("... and Language, or Near and ...").
// In subject mode (T0 is "in ..." after a credit phrase) the items may be
// lower case: "1.0 credit in political theory, history of science, or
// intellectual history".
func nameRunEnd(src string, toks []token, i int, subject bool) int {
	if !nameEnd(src[toks[i].start:toks[i].end], subject) {
		return i
	}
	commas := 0
	for j := i + 1; ; {
		next, conj, comma := separatorAt(toks, j)
		if next == j || next >= len(toks) || toks[next].kind != tText {
			return i
		}
		item := src[toks[next].start:toks[next].end]
		switch {
		case !conj && !comma:
			return i
		case comma:
			if !nameItem(item, false) && !(subject && subjectItem(item)) {
				return i
			}
			commas++
			j = next + 1
			continue
		case commas == 0 && (toks[j].kind == tCommaAnd || toks[j].kind == tCommaOr),
			commas > 0 && toks[j].kind == tSlash:
			return i
		case !nameItem(item, true) && !(subject && subjectItem(item)):
			return i
		}
		return next
	}
}

// separatorAt reads the separator at j and returns the index after it, and
// whether it is a conjunction or a plain comma.
func separatorAt(toks []token, j int) (next int, conj, comma bool) {
	if j >= len(toks) {
		return j, false, false
	}
	switch toks[j].kind {
	case tComma:
		return j + 1, false, true
	case tAnd, tOr, tSlash, tCommaAnd, tCommaOr:
		return j + 1, true, false
	}
	return j, false, false
}

// nameEnd reports whether the text before a run ends in a name word
// ("approval of the Centre for Drama", "from Group C: Society"), or, in
// subject mode, in any plain word ("in political theory").
func nameEnd(s string, subject bool) bool {
	words := strings.Fields(s)
	if len(words) == 0 {
		return false
	}
	last := words[len(words)-1]
	return titleWord(last) || subject && plainWord(last) && !titleRejectRe.MatchString(s)
}

// nameItem reports whether s is a capitalised name: every word is a title
// word or a connector, and nothing reads as a requirement of its own. The last
// item of a list may end in lower-case head nouns ("Society minor").
func nameItem(s string, last bool) bool {
	words := strings.Fields(s)
	if len(words) == 0 || !titleWord(words[0]) || titleRejectRe.MatchString(s) {
		return false
	}
	tail := false // inside the lower-case head nouns
	for _, w := range words[1:] {
		switch {
		case last && nameHeadNouns[w]:
			tail = true
		case tail:
			if w != "of" && w != "study" {
				return false
			}
		case titleWord(w) || nameConnectors[w]:
		default:
			return false
		}
	}
	return true
}

// parenItem reports whether s can be part of a bracketed name list: it starts
// with a capitalised word or is an acronym, and nothing in it reads as a
// requirement of its own. "Summer Session projects", "HPS".
func parenItem(s string) bool {
	words := strings.Fields(s)
	return len(words) > 0 && (titleWord(words[0]) || acronymRe.MatchString(s)) && !titleRejectRe.MatchString(s)
}

// subjectItem reports whether s is a short lower-case subject: "history of
// science", "pre-modern history", "sexuality".
func subjectItem(s string) bool {
	words := strings.Fields(s)
	if len(words) == 0 || len(words) > 4 || titleRejectRe.MatchString(s) ||
		subjectRejectRe.MatchString(s) || classify(s) != types.KindOther {
		return false
	}
	switch strings.ToLower(words[0]) {
	case "a", "an", "the", "any", "another", "other":
		return false
	}
	for _, w := range words {
		if !plainWord(w) {
			return false
		}
	}
	return true
}

// titleWord reports whether w is a capitalised word with no digits: "Drama",
// "U.S.", "C++", "(CDTPS)". A bare designator ("SOC") is not one.
func titleWord(w string) bool {
	core := strings.Trim(w, `().,:;'"`)
	if core == "" || core[0] < 'A' || core[0] > 'Z' || strings.ContainsAny(core, "0123456789") {
		return false
	}
	return strings.HasPrefix(w, "(") || !designatorWordRe.MatchString(core)
}

// plainWord reports whether w is made of letters only (plus ' and -).
func plainWord(w string) bool {
	core := strings.Trim(w, `.,:;`)
	if core == "" {
		return false
	}
	for _, c := range core {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '\'' || c == '-' || c == '’') {
			return false
		}
	}
	return true
}

// isProgramToken reports whether t is a program reference kept whole by
// matchProgram; those are never extended.
func isProgramToken(src string, t token) bool {
	m, ok := matchProgram(src[t.start:])
	return ok && m.end == t.end-t.start
}

// joinTokens replaces toks[i..k] with one text token spanning them.
func joinTokens(toks []token, i, k int) []token {
	joined := token{kind: tText, start: toks[i].start, end: toks[k].end}
	return append(append(toks[:i:i], joined), toks[k+1:]...)
}
