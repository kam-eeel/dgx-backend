package completion

import (
	"regexp"
	"strings"

	"degree-explorer/parser/grammar"
)

// Advice while reading rows: headings, labels and blocks that give advice, and
// the lines after them that only list courses.

// AdviceHeadingRe matches a heading that names advice: "Suggested Related
// Courses".
var AdviceHeadingRe = regexp.MustCompile(`(?i)\b(?:recommended|suggested|suggestions?)\b`)

// adviceHeading reports whether heading h is advice: it says so and the
// classifier rates it advisory. A notes heading is not: notes hold rules too.
func (a *Assembler) adviceHeading(h *Block) bool {
	return h.Kind == BlockHeading && !h.IsNotes() && AdviceHeadingRe.MatchString(h.Text) && a.role(h.Text) == RoleAdvisory
}

// UnderAdviceHeading reports whether a heading over b, at any depth, is advice.
func (a *Assembler) UnderAdviceHeading(b *Block) bool {
	for h := b.Heading; h != nil; h = h.Heading {
		if a.adviceHeading(h) {
			return true
		}
	}
	return false
}

// AdviceLabel reports a labelled line whose label is advice: "Other courses
// which might of interest: ESS391H1, ...".
func (a *Assembler) AdviceLabel(b *Block) bool {
	return b.Kind == blockLabelled && !BlkNoteLabelRe.MatchString(b.Label) && adviceLabelRe.MatchString(b.Label) && a.role(b.Label) == RoleAdvisory
}

// adviceLabelRe matches a label that names advice: "might be of interest".
var adviceLabelRe = regexp.MustCompile(`(?i)\b(?:recommended|suggested|might\s+(?:be\s+)?of\s+interest)\b`)

// adviceBlock reads an advice block and the blocks under it: its sentences are
// one note, prefixed with the heading they are under. An approval route, an
// exclusion or a cap among them is still read as one.
func (a *Assembler) adviceBlock(b *Block, ctx compCtx) {
	var keep []string
	for _, s := range SplitSentences(BlockLine(b)) {
		switch role := a.role(s); {
		case role == roleExclusion:
			a.exclusion(s)
		case role == RoleRequirement && a.IsApproval(s):
			a.approval(ctx.owner, s)
		default:
			// a cap in advice is still a cap
			parts := grammar.SplitTop(s, ';')
			var advice []string
			for i, p := range parts {
				p = strings.TrimSpace(p)
				if r := capRow(p); i > 0 && r != nil && capLeadRe.MatchString(p) {
					a.rowUnder(ctx.owner, r)
					continue
				}
				advice = append(advice, p)
			}
			keep = append(keep, strings.TrimSuffix(grammar.CleanNote(strings.Join(advice, "; ")), "."))
		}
	}
	if keep != nil {
		note := strings.Join(keep, ". ")
		if h := b.Heading; h != nil && h.Section != "" && b.Label != h.Section {
			note = h.Section + ": " + note
		}
		a.notesTo(ctx.owner, []string{note})
	}
	for _, c := range b.Children {
		a.adviceBlock(c, ctx)
	}
}

// adviceLeadEndRe matches the end of a lead into what follows: "are as
// follows."
var adviceLeadEndRe = regexp.MustCompile(`(?i)(?::|\bas\s+follows\s*[.:]?|\(as\s+listed\s+below\)\.?)\s*$`)

// notRequirementLeadRe matches a lead saying that what follows is no
// requirement, which points forward whatever its end: "streams are not an
// official program requirement".
var notRequirementLeadRe = regexp.MustCompile(`(?i)\bare\s+not\s+an?\s+official\s+(?:program\s+)?requirement\b`)

// adviceForwardRe matches advice pointing at what follows: "we suggest you
// consider the following courses".
var adviceForwardRe = regexp.MustCompile(`(?i)\bthe\s+following\b|\bas\s+follows\b|\bbelow\b|:\s*$`)

// AdviceZone returns how many blocks after bs[0] are the lines of advice bs[0]
// leads into, or 0: bs[0] ends in ":" or "as follows", with advice pointing
// forward and no rule, and the lines are labelled lines, bullets and codes.
func (a *Assembler) AdviceZone(bs []*Block) int {
	b := bs[0]
	notRequired := notRequirementLeadRe.MatchString(b.Text)
	if b.Kind != blockProse && !(b.Kind == blockLabelled && BlkNoteLabelRe.MatchString(b.Label)) || len(b.Children) > 0 ||
		!adviceLeadEndRe.MatchString(strings.TrimSpace(b.Text)) && !notRequired {
		return 0
	}
	forward := notRequired
	for _, s := range SplitSentences(b.Text) {
		switch role := a.role(s); {
		case role == RoleAdvisory:
			forward = forward || adviceForwardRe.MatchString(s)
		case role == RoleRequirement && !a.IsApproval(s) && !a.IsAlternative(s):
		default:
			return 0
		}
	}
	if !forward {
		return 0
	}
	n := 0
	for _, c := range bs[1:] {
		if c.Kind == BlockHeading && !c.IsNotes() && !blkAmountRe.MatchString(c.Text) && len(c.Children) == 0 {
			// a heading naming who the advice is for
			n++
			continue
		}
		// a line whose emphasis kept its label from being one is a member too
		_, after, colon := strings.Cut(c.Text, ": ")
		member := c.Kind == blockLabelled && !BlkNoteLabelRe.MatchString(c.Label) || c.Kind == blockBullet || isCodeListLine(c) ||
			c.Kind == blockProse && len(c.Children) == 0 && colon && mostlyCodes(after)
		if !member || blkNumbered(c) || !grammar.CodeRe.MatchString(c.Text) && !amountRe.MatchString(c.Text) {
			break
		}
		n++
	}
	for n > 0 && bs[n].Kind == BlockHeading {
		n-- // a heading the zone ends on belongs to what follows
	}
	return n
}

// adviceNote joins advice with the lines under it that only list or
// describe courses, and returns the note and the lines that are rules.
func (a *Assembler) adviceNote(lead string, kids []*Block) (string, []*Block) {
	// the lead ends in ":" to lead into its list in one sentence
	note := strings.TrimSuffix(grammar.CleanNote(lead), ":") + ":"
	var rest []*Block
	var walk func(bs []*Block)
	walk = func(bs []*Block) {
		for _, c := range bs {
			if !a.listsOnly(c.Text) {
				rest = append(rest, c)
				continue
			}
			if c.Kind == BlockHeading {
				note += " " + strings.TrimSuffix(grammar.CleanNote(c.Text), ":") + ":"
				continue
			}
			note += " " + grammar.CleanNote(BlockLine(c))
			walk(c.Children)
		}
	}
	walk(kids)
	return note, rest
}

// listsOnly reports whether every sentence of s is advice, a list or a plain
// requirement: no rule, approval or exclusion.
func (a *Assembler) listsOnly(s string) bool {
	for _, sent := range SplitSentences(s) {
		switch a.role(sent) {
		case RoleAdvisory, RoleListDef:
		case RoleRequirement:
			if a.IsApproval(sent) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (a *Assembler) allAdvice(s string) bool {
	for _, sent := range SplitSentences(s) {
		if a.role(sent) != RoleAdvisory {
			return false
		}
	}
	return true
}
