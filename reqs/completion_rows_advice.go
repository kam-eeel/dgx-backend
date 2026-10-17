package reqs

import (
	"regexp"
	"strings"
)

// Advice while reading rows: headings, labels and blocks that give advice, and
// the lines after them that only list courses.

// complAdviceHeadingRe matches a heading that names advice: "Suggested Related
// Courses".
var complAdviceHeadingRe = regexp.MustCompile(`(?i)\b(?:recommended|suggested|suggestions?)\b`)

// adviceHeading reports whether heading h is advice: it says so and the
// classifier rates it advisory. A notes heading is not: notes hold rules too.
func (a *compAssembler) adviceHeading(h *compBlock) bool {
	return h.kind == blockHeading && !h.isNotes() && complAdviceHeadingRe.MatchString(h.text) && a.role(h.text) == roleAdvisory
}

// underAdviceHeading reports whether a heading over b, at any depth, is advice.
func (a *compAssembler) underAdviceHeading(b *compBlock) bool {
	for h := b.heading; h != nil; h = h.heading {
		if a.adviceHeading(h) {
			return true
		}
	}
	return false
}

// adviceLabel reports a labelled line whose label is advice: "Other courses
// which might of interest: ESS391H1, ...".
func (a *compAssembler) adviceLabel(b *compBlock) bool {
	return b.kind == blockLabelled && !blkNoteLabelRe.MatchString(b.label) && complAdviceLabelRe.MatchString(b.label) && a.role(b.label) == roleAdvisory
}

// complAdviceLabelRe matches a label that names advice: "might be of interest".
var complAdviceLabelRe = regexp.MustCompile(`(?i)\b(?:recommended|suggested|might\s+(?:be\s+)?of\s+interest)\b`)

// adviceBlock reads an advice block and the blocks under it: its sentences are
// one note, prefixed with the heading they are under. An approval route, an
// exclusion or a cap among them is still read as one.
func (a *compAssembler) adviceBlock(b *compBlock, ctx compCtx) {
	var keep []string
	for _, s := range splitComplSentences(blockLine(b)) {
		switch role := a.role(s); {
		case role == roleExclusion:
			a.exclusion(s)
		case role == roleRequirement && a.isApproval(s):
			a.approval(ctx.owner, s)
		default:
			// a cap in advice is still a cap
			parts := splitTop(s, ';')
			var advice []string
			for i, p := range parts {
				p = strings.TrimSpace(p)
				if r := capRow(p); i > 0 && r != nil && complCapLeadRe.MatchString(p) {
					a.rowUnder(ctx.owner, r)
					continue
				}
				advice = append(advice, p)
			}
			keep = append(keep, strings.TrimSuffix(cleanNote(strings.Join(advice, "; ")), "."))
		}
	}
	if keep != nil {
		note := strings.Join(keep, ". ")
		if h := b.heading; h != nil && h.section != "" && b.label != h.section {
			note = h.section + ": " + note
		}
		a.notesTo(ctx.owner, []string{note})
	}
	for _, c := range b.children {
		a.adviceBlock(c, ctx)
	}
}

// complAdviceLeadEndRe matches the end of a lead into what follows: "are as
// follows."
var complAdviceLeadEndRe = regexp.MustCompile(`(?i)(?::|\bas\s+follows\s*[.:]?|\(as\s+listed\s+below\)\.?)\s*$`)

// complNotRequirementLeadRe matches a lead saying that what follows is no
// requirement, which points forward whatever its end: "streams are not an
// official program requirement".
var complNotRequirementLeadRe = regexp.MustCompile(`(?i)\bare\s+not\s+an?\s+official\s+(?:program\s+)?requirement\b`)

// complAdviceForwardRe matches advice pointing at what follows: "we suggest you
// consider the following courses".
var complAdviceForwardRe = regexp.MustCompile(`(?i)\bthe\s+following\b|\bas\s+follows\b|\bbelow\b|:\s*$`)

// adviceZone returns how many blocks after bs[0] are the lines of advice bs[0]
// leads into, or 0: bs[0] ends in ":" or "as follows", with advice pointing
// forward and no rule, and the lines are labelled lines, bullets and codes.
func (a *compAssembler) adviceZone(bs []*compBlock) int {
	b := bs[0]
	notRequired := complNotRequirementLeadRe.MatchString(b.text)
	if b.kind != blockProse && !(b.kind == blockLabelled && blkNoteLabelRe.MatchString(b.label)) || len(b.children) > 0 ||
		!complAdviceLeadEndRe.MatchString(strings.TrimSpace(b.text)) && !notRequired {
		return 0
	}
	forward := notRequired
	for _, s := range splitComplSentences(b.text) {
		switch role := a.role(s); {
		case role == roleAdvisory:
			forward = forward || complAdviceForwardRe.MatchString(s)
		case role == roleRequirement && !a.isApproval(s) && !a.isAlternative(s):
		default:
			return 0
		}
	}
	if !forward {
		return 0
	}
	n := 0
	for _, c := range bs[1:] {
		if c.kind == blockHeading && !c.isNotes() && !blkAmountRe.MatchString(c.text) && len(c.children) == 0 {
			// a heading naming who the advice is for
			n++
			continue
		}
		// a line whose emphasis kept its label from being one is a member too
		_, after, colon := strings.Cut(c.text, ": ")
		member := c.kind == blockLabelled && !blkNoteLabelRe.MatchString(c.label) || c.kind == blockBullet || isCodeListLine(c) ||
			c.kind == blockProse && len(c.children) == 0 && colon && mostlyCodes(after)
		if !member || blkNumbered(c) || !codeRe.MatchString(c.text) && !complAmountRe.MatchString(c.text) {
			break
		}
		n++
	}
	for n > 0 && bs[n].kind == blockHeading {
		n-- // a heading the zone ends on belongs to what follows
	}
	return n
}

// adviceNote joins advice with the lines under it that only list or
// describe courses, and returns the note and the lines that are rules.
func (a *compAssembler) adviceNote(lead string, kids []*compBlock) (string, []*compBlock) {
	// the lead ends in ":" to lead into its list in one sentence
	note := strings.TrimSuffix(cleanNote(lead), ":") + ":"
	var rest []*compBlock
	var walk func(bs []*compBlock)
	walk = func(bs []*compBlock) {
		for _, c := range bs {
			if !a.listsOnly(c.text) {
				rest = append(rest, c)
				continue
			}
			if c.kind == blockHeading {
				note += " " + strings.TrimSuffix(cleanNote(c.text), ":") + ":"
				continue
			}
			note += " " + cleanNote(blockLine(c))
			walk(c.children)
		}
	}
	walk(kids)
	return note, rest
}

// listsOnly reports whether every sentence of s is advice, a list or a plain
// requirement: no rule, approval or exclusion.
func (a *compAssembler) listsOnly(s string) bool {
	for _, sent := range splitComplSentences(s) {
		switch a.role(sent) {
		case roleAdvisory, roleListDef:
		case roleRequirement:
			if a.isApproval(sent) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (a *compAssembler) allAdvice(s string) bool {
	for _, sent := range splitComplSentences(s) {
		if a.role(sent) != roleAdvisory {
			return false
		}
	}
	return true
}
