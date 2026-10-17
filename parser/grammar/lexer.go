package grammar

import (
	"regexp"
	"slices"
	"strings"
)

// operators, tried after the phrase matchers. Longer forms come first so
// ", and" wins over ",".
var operators = []struct {
	re   *regexp.Regexp
	kind TokKind
}{
	{regexp.MustCompile(`^,\s*(?i:and|plus|including)\b`), TCommaAnd},
	{regexp.MustCompile(`^,\s*(?i:or)\b`), TCommaOr},
	{regexp.MustCompile(`^,`), TComma},
	{regexp.MustCompile(`^;`), TSemi},
	{regexp.MustCompile(`^/`), TSlash},
	{regexp.MustCompile(`^(?:(?i:and|plus|including)\b|&|\+)`), TAnd},
	{regexp.MustCompile(`^(?i:or)\b`), TOr},
	{regexp.MustCompile(`^\(`), TLParen},
	{regexp.MustCompile(`^\)`), TRParen},
}

// lex splits normalized text into tokens. Anything no matcher recognizes
// becomes a TText token that runs until the next word where some other
// matcher would match.
func Lex(src string) []Token { return lexWith(src, phraseMatchers) }

// LexCompletion lexes program completion text: the completion phrases are
// tried before the shared ones, and tSkip tokens are dropped.
func LexCompletion(src string) []Token {
	return slices.DeleteFunc(lexWith(src, completionMatchers), func(t Token) bool { return t.Kind == tSkip })
}

func lexWith(src string, matchers []phraseMatcher) []Token {
	var toks []Token
	pos := skipSpace(src, 0)
	for pos < len(src) {
		t, ok := matchAt(src, pos, matchers)
		if !ok {
			t = textAt(src, pos, matchers)
		}
		toks = append(toks, t)
		pos = skipSpace(src, t.End)
	}
	return toks
}

// matchAt tries every phrase matcher, then every operator, at pos.
func matchAt(src string, pos int, matchers []phraseMatcher) (Token, bool) {
	rest := src[pos:]
	for _, m := range matchers {
		if t, ok := m(rest); ok {
			t.Start, t.End = pos, pos+t.End
			return t, true
		}
	}
	for _, op := range operators {
		if loc := op.re.FindStringIndex(rest); loc != nil {
			return Token{Kind: op.kind, Start: pos, End: pos + loc[1]}, true
		}
	}
	return Token{}, false
}

// textAt consumes words starting at pos until the next word is something
// another matcher recognizes. A word ends at whitespace or at one of , ; / ( ).
func textAt(src string, pos int, matchers []phraseMatcher) Token {
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
	return Token{Kind: TText, Start: pos, End: end}
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
