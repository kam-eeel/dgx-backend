package completion

import (
	"testing"
)

// TestCompletionRuleTitles checks course titles and labels in lists, and pools
// written inside an item.
func TestCompletionRuleTitles(t *testing.T) {
	runComplRuleCases(t, []ruleCase{
		{"bare course titles after the codes are dropped (ASMAJ0151)",
			"3. 0.5 credit from one of the following traditions: RLG201H1 Indigenous Spiritualities and Religions/ RLG202H1 Judaism/ RLG241H1 The Earliest Christians",
			[]string{
				`1 allocation {"type":"credits","amount":0.5,"courses":["RLG201H1","RLG202H1","RLG241H1"]}`,
			},
			nil},
		{"course titles in brackets are dropped (ASMAJ2745)",
			"2. 0.5 credit of field courses: ESS410H1 (Field Techniques in Hydrogeochemistry)/ ESS450H1 (Geophysical Field Techniques)/ GGR390H1 (Field Methods)",
			[]string{
				`1 allocation {"type":"credits","amount":0.5,"courses":["ESS410H1","ESS450H1","GGR390H1"]}`,
			},
			nil},
		{"category labels in a list are dropped, a restated corequisite is a note (ASMAJ1390, shortened)",
			"6. 0.5 credit at the 400-level from: field course EEB403H1, EEB405H1; seminar/lecture course EEB428H1, ENV432H1; EEB497H1, independent research project course EEB498Y1 (concurrent with research issues course EEB488H1), EEB499Y1",
			[]string{
				`1 allocation {"type":"credits","amount":0.5,"courses":["EEB403H1","EEB405H1","EEB428H1","ENV432H1","EEB497H1","EEB498Y1","EEB499Y1"]}`,
				`  note: EEB498Y1 concurrent with research issues course EEB488H1`,
			},
			nil},
		{"a topic of two subjects describes the pool (ASSPE2364)",
			"5. 0.5 credit at the 300+ level in **plant or microbial biology** from: CSB340H1, CSB350H1, EEB328H1",
			[]string{
				`1 allocation {"type":"credits","amount":0.5,"courses":["CSB340H1","CSB350H1","EEB328H1"]}`,
			},
			nil},
	})
}
