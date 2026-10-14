package scraper

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// The snippets are copied from the calendar pages in data/programs, trimmed to
// a few items.
func TestBlockText(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "paragraph with course links and zero-width spaces",
			html: `<p>2.  <a href="/course/CSC207H1">CSC207H1</a>,  <a href="/course/CSC236H1">CSC236H1</a>/` + "\u200b" + `  <a href="/course/CSC240H1">CSC240H1</a>,  <a href="/course/CSC258H1">CSC258H1</a></p>`,
			want: "2. CSC207H1, CSC236H1/ CSC240H1, CSC258H1",
		},
		{
			name: "bullet list with nbsp",
			html: `<ul style="list-style-type: circle;"> <li> <a href="/course/CSC108H1">CSC108H1</a>,  <a href="/course/CSC148H1">CSC148H1</a>,  <a href="/course/CSC165H1">CSC165H1</a> or&nbsp;</li> <li> <a href="/course/CSC148H1">CSC148H1</a>,  <a href="/course/CSC165H1">CSC165H1</a> or</li> </ul>`,
			want: "- CSC108H1, CSC148H1, CSC165H1 or\n- CSC148H1, CSC165H1 or",
		},
		{
			name: "lower-alpha list",
			html: `<p>Notes:</p> <ol style="list-style-type: lower-alpha;"> <li>Students with a strong background in an object-oriented language such as Python, Java or C++ may omit  <a href="/course/CSC108H1">CSC108H1</a> and proceed directly with  <a href="/course/CSC148H1">CSC148H1</a>.</li> <li>Consult the Undergraduate Office for advice about choosing among  <a href="/course/CSC108H1">CSC108H1</a> and  <a href="/course/CSC148H1">CSC148H1</a>.</li> </ol>`,
			want: "Notes:\na. Students with a strong background in an object-oriented language such as Python, Java or C++ may omit CSC108H1 and proceed directly with CSC148H1.\nb. Consult the Undergraduate Office for advice about choosing among CSC108H1 and CSC148H1.",
		},
		{
			name: "nested bullet list",
			html: `<ul> <li>0.5 credit from  <a href="/course/MUN150H1">MUN150H1</a>/` + "\u200b" + `  <a href="/course/MUN160H1">MUN160H1</a>, with a minimum mark of 65%, and</li> <li>Either <ul> <li>An additional 0.5 credit from  <a href="/course/MUN150H1">MUN150H1</a>/` + "\u200b" + `  <a href="/course/MUN160H1">MUN160H1</a> with a minimum mark of 65%, <u><b>or</b></u></li> <li>0.5 credit from  <a href="/course/MUN100H1">MUN100H1</a>/` + "\u200b" + `  <a href="/course/POL101H1">POL101H1</a></li> </ul> </li> </ul>`,
			want: "- 0.5 credit from MUN150H1/ MUN160H1, with a minimum mark of 65%, and\n- Either\n  - An additional 0.5 credit from MUN150H1/ MUN160H1 with a minimum mark of 65%, **or**\n  - 0.5 credit from MUN100H1/ POL101H1",
		},
		{
			name: "nested numbered list in a bullet-less item",
			html: `<ol> <li>1.0 credits including:</li> </ol> <ol> <li style="list-style-type: none;"> <ol style="list-style-type: lower-alpha;"> <li>Up to 0.5 credits from <a href="https://artsci.calendar.utoronto.ca/course/CSC301H1">CSC301H1</a> and  <a href="/course/CSC364H1">CSC364H1</a> (if not taken in list 3)</li> <li>Up to 1.0 credits from <a href="https://artsci.calendar.utoronto.ca/course/CSC404H1">CSC404H1</a></li> </ol> </li> </ol>`,
			want: "1. 1.0 credits including:\n  a. Up to 0.5 credits from CSC301H1 and CSC364H1 (if not taken in list 3)\n  b. Up to 1.0 credits from CSC404H1",
		},
		{
			name: "continuation lines in a list item",
			html: `<ol> <li>2.0 credits to be selected from lists 1 and 2: <br>(1)  <a href="/course/ACT350H1">ACT350H1</a>,  <a href="/course/ACT371H1">ACT371H1</a>, and <br>(2)  <a href="/course/ACT372H1">ACT372H1</a>,  <a href="/course/ACT470H1">ACT470H1</a>.</li> <li>Professional Experience Course  <a href="/course/ACT390H1">ACT390H1</a></li> </ol>`,
			want: "1. 2.0 credits to be selected from lists 1 and 2:\n   (1) ACT350H1, ACT371H1, and\n   (2) ACT372H1, ACT470H1.\n2. Professional Experience Course ACT390H1",
		},
		{
			name: "heading wrapped around a line break",
			html: `<p><em>Variable Minimum Grade or Variable Minimum Grade Average<br></em>A minimum grade and minimum grade average in required courses are needed for entry.</p>`,
			want: "_Variable Minimum Grade or Variable Minimum Grade Average_\nA minimum grade and minimum grade average in required courses are needed for entry.",
		},
		{
			name: "struck-out course",
			html: `<ol> <li>1.0 credit from:  <a href="/course/STS248H1">STS248H1</a>, <strike> <a href="/course/VIC274H1">VIC274H1</a></strike>,  <a href="/course/STS301H1">STS301H1</a></li> </ol>`,
			want: "1. 1.0 credit from: STS248H1, , STS301H1",
		},
		{
			name: "bold heading with trailing space",
			html: `<p><strong>Group B: </strong> <a href="/course/GGR221H1">GGR221H1</a>/` + "\u200b" + `  <a href="/course/IRE332Y0">IRE332Y0</a></p>`,
			want: "**Group B:** GGR221H1/ IRE332Y0",
		},
		{
			name: "italic word inside a list item",
			html: `<ul> <li><strong>Group A: </strong> <a href="/course/CSC209H1">CSC209H1</a>; any 300-/ 400-level CSC course <em>except </em>those listed in Group B</li> </ul>`,
			want: "- **Group A:** CSC209H1; any 300-/ 400-level CSC course _except_ those listed in Group B",
		},
		{
			name: "nested emphasis",
			html: `<ul> <li> <a href="https://utsc.calendar.utoronto.ca/course/PSYA01H3">PSYA01H3</a> <strong><em>and</em> </strong> <a href="https://utsc.calendar.utoronto.ca/course/PSYA02H3">PSYA02H3</a> (both) from UTSC.</li> </ul>`,
			want: "- PSYA01H3 **_and_** PSYA02H3 (both) from UTSC.",
		},
		{
			name: "hidden editor bookmarks",
			html: `<p><br><span style="display:none;" id="cke_bm_267C"><strong>&nbsp;</strong></span><span style="display:none;" id="cke_bm_270C"><strong>&nbsp;</strong></span><strong>Academic Path to Completion</strong></p><p>&nbsp;</p>`,
			want: "**Academic Path to Completion**",
		},
		{
			// The calendar never sets start or type; this is the HTML standard.
			name: "start attribute and roman numbering",
			html: `<ol start="3" type="I"><li>x</li><li>y</li></ol><ol start="26" style="list-style-type: lower-alpha;"><li>z</li><li>aa</li></ol>`,
			want: "III. x\nIV. y\nz. z\naa. aa",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<div class="field-content">` + tt.html + `</div>`))
			if err != nil {
				t.Fatal(err)
			}
			if got := blockText(doc.Find(".field-content")); got != tt.want {
				t.Errorf("blockText() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
