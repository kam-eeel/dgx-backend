package scraper

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// blockText renders a requirements section as text with a line per paragraph,
// break or list item ("- ", "1. "), **bold**, _italics_ and no struck-out text.
// Unlike .Text() it keeps the boundaries the requirement parsers depend on.
func blockText(sel *goquery.Selection) string {
	r := &renderer{}
	for _, n := range sel.Nodes {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			r.walk(c)
		}
	}
	r.flush()
	return strings.Join(r.lines, "\n")
}

// run is a piece of inline text with the emphasis markers around it, outermost
// first.
type run struct {
	text  string
	marks []string
}

type list struct {
	ordered bool
	style   string // ol numbering: "1", "a", "A", "i" or "I"
	n       int    // number of the next item
}

type renderer struct {
	lines  []string
	runs   []run    // inline content of the line being built
	marks  []string // open emphasis markers
	lists  []*list
	prefix string // indent and marker for the line being built
	cont   string // prefix for the lines after it
}

var zeroWidth = strings.NewReplacer("\u200b", "", "\u200c", "", "\u200d", "", "\ufeff", "")

func (r *renderer) walk(n *html.Node) {
	if n.Type == html.TextNode {
		r.runs = append(r.runs, run{n.Data, r.marks})
		return
	}
	if n.Type != html.ElementNode {
		return
	}

	switch n.Data {
	case "strike", "s", "del":
		// Struck-out text is a removed course or rule, not part of the requirement.
		return
	case "br":
		r.flush()
		return
	case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6":
		r.flush()
		r.children(n)
		r.flush()
		return
	case "ul", "ol":
		r.flush()
		l := &list{ordered: n.Data == "ol", style: listStyle(n), n: 1}
		if s, err := strconv.Atoi(attr(n, "start")); err == nil {
			l.n = s
		}
		r.lists = append(r.lists, l)
		r.children(n)
		r.lists = r.lists[:len(r.lists)-1]
		r.flush()
		return
	case "li":
		r.flush()
		prefix, cont := r.prefix, r.cont
		indent, marker := "", "- "
		if d := len(r.lists); d > 0 {
			indent = strings.Repeat("  ", d-1)
			if l := r.lists[d-1]; l.ordered {
				marker = itemNumber(l.n, l.style) + ". "
				l.n++
			}
		}
		// CKEditor nests lists by wrapping them in a bullet-less item.
		if listStyle(n) == "none" {
			marker = ""
		}
		r.prefix = indent + marker
		r.cont = indent + strings.Repeat(" ", len(marker))
		r.children(n)
		r.flush()
		// Restoring the prefix too keeps the parent's marker if the parent
		// item had no text of its own before this one.
		r.prefix, r.cont = prefix, cont
		return
	}

	mark := ""
	switch n.Data {
	case "strong", "b":
		mark = "**"
	case "em", "i":
		mark = "_"
	}
	if mark == "" || slices.Contains(r.marks, mark) {
		r.children(n)
		return
	}
	// Clone, as runs already collected keep a reference to the current slice.
	saved := r.marks
	r.marks = append(slices.Clone(r.marks), mark)
	r.children(n)
	r.marks = saved
}

func (r *renderer) children(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		r.walk(c)
	}
}

// flush renders the inline runs as one line, collapsing whitespace. Markers
// open right before a word and close before the next space; one that spans a
// line break is closed at the line's end and reopened on the next line.
func (r *renderer) flush() {
	var b strings.Builder
	var open []string
	space := false
	for _, ru := range r.runs {
		t := zeroWidth.Replace(ru.text)
		if t == "" {
			continue
		}
		if first, _ := utf8.DecodeRuneInString(t); unicode.IsSpace(first) {
			space = true
		}
		for i, w := range strings.Fields(t) {
			if i > 0 {
				space = true
			}
			keep := 0
			for keep < len(open) && keep < len(ru.marks) && open[keep] == ru.marks[keep] {
				keep++
			}
			for j := len(open) - 1; j >= keep; j-- {
				b.WriteString(open[j])
			}
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			for _, m := range ru.marks[keep:] {
				b.WriteString(m)
			}
			open = ru.marks
			b.WriteString(w)
		}
		if last, _ := utf8.DecodeLastRuneInString(t); unicode.IsSpace(last) {
			space = true
		}
	}
	for j := len(open) - 1; j >= 0; j-- {
		b.WriteString(open[j])
	}
	r.runs = nil
	if b.Len() == 0 {
		return
	}
	r.lines = append(r.lines, r.prefix+b.String())
	r.prefix = r.cont
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

var listStyleRe = regexp.MustCompile(`list-style-type:\s*([a-z-]+)`)

// listStyle returns an element's numbering style from its list-style-type
// (which the calendar uses) or its type attribute.
func listStyle(n *html.Node) string {
	if m := listStyleRe.FindStringSubmatch(attr(n, "style")); m != nil {
		switch m[1] {
		case "lower-alpha", "lower-latin":
			return "a"
		case "upper-alpha", "upper-latin":
			return "A"
		case "lower-roman":
			return "i"
		case "upper-roman":
			return "I"
		case "none":
			return "none"
		}
		return "1"
	}
	switch t := attr(n, "type"); t {
	case "a", "A", "i", "I":
		return t
	}
	return "1"
}

func itemNumber(n int, style string) string {
	switch style {
	case "a", "A":
		s := ""
		for ; n > 0; n = (n - 1) / 26 {
			s = string(rune('a'+(n-1)%26)) + s
		}
		if style == "A" {
			s = strings.ToUpper(s)
		}
		return s
	case "i", "I":
		s := roman(n)
		if style == "i" {
			s = strings.ToLower(s)
		}
		return s
	}
	return fmt.Sprint(n)
}

func roman(n int) string {
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	s := ""
	for i, v := range vals {
		for ; n >= v; n -= v {
			s += syms[i]
		}
	}
	return s
}
