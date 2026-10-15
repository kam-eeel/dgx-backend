package reqs

import (
	"regexp"
	"slices"
	"strings"
)

// operators, tried after the phrase matchers. Longer forms come first so
// ", and" wins over ",".
var operators = []struct {
	re   *regexp.Regexp
	kind tokKind
}{
	{regexp.MustCompile(`^,\s*(?i:and|plus|including)\b`), tCommaAnd},
	{regexp.MustCompile(`^,\s*(?i:or)\b`), tCommaOr},
	{regexp.MustCompile(`^,`), tComma},
	{regexp.MustCompile(`^;`), tSemi},
	{regexp.MustCompile(`^/`), tSlash},
	{regexp.MustCompile(`^(?:(?i:and|plus|including)\b|&|\+)`), tAnd},
	{regexp.MustCompile(`^(?i:or)\b`), tOr},
	{regexp.MustCompile(`^\(`), tLParen},
	{regexp.MustCompile(`^\)`), tRParen},
}

// lex splits normalized text into tokens. Anything no matcher recognizes
// becomes a tText token that runs until the next word where some other
// matcher would match.
func lex(src string) []token { return lexWith(src, phraseMatchers) }

// lexCompletion lexes program completion text: the completion phrases are
// tried before the shared ones, and tSkip tokens are dropped.
func lexCompletion(src string) []token {
	return slices.DeleteFunc(lexWith(src, completionMatchers), func(t token) bool { return t.kind == tSkip })
}

func lexWith(src string, matchers []phraseMatcher) []token {
	var toks []token
	pos := skipSpace(src, 0)
	for pos < len(src) {
		t, ok := matchAt(src, pos, matchers)
		if !ok {
			t = textAt(src, pos, matchers)
		}
		toks = append(toks, t)
		pos = skipSpace(src, t.end)
	}
	return toks
}

// matchAt tries every phrase matcher, then every operator, at pos.
func matchAt(src string, pos int, matchers []phraseMatcher) (token, bool) {
	rest := src[pos:]
	for _, m := range matchers {
		if t, ok := m(rest); ok {
			t.start, t.end = pos, pos+t.end
			return t, true
		}
	}
	for _, op := range operators {
		if loc := op.re.FindStringIndex(rest); loc != nil {
			return token{kind: op.kind, start: pos, end: pos + loc[1]}, true
		}
	}
	return token{}, false
}

// textAt consumes words starting at pos until the next word is something
// another matcher recognizes. A word ends at whitespace or at one of , ; / ( ).
func textAt(src string, pos int, matchers []phraseMatcher) token {
	end := pos
	for {
		end = wordEnd(src, end)
		next := skipSpace(src, end)
		if next >= len(src) {
			break
		}
		if _, ok := matchAt(src, next, matchers); ok {
			break
		}
		end = next
	}
	return token{kind: tText, start: pos, end: end}
}

func wordEnd(src string, pos int) int {
	i := pos
	for i < len(src) && src[i] != ' ' && !strings.ContainsRune(",;/()", rune(src[i])) {
		i++
	}
	if i == pos { // a lone separator the operators didn't claim; take it as text
		i++
	}
	return i
}

func skipSpace(src string, pos int) int {
	for pos < len(src) && src[pos] == ' ' {
		pos++
	}
	return pos
}
