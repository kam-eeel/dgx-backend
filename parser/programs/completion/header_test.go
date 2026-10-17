package completion

import (
	"fmt"
	"testing"

	"degree-explorer/types"
)

// hdrView is compHeader in JSON, for comparing with inline expectations.
type hdrView struct {
	Total float64              `json:"total"`
	Upper float64              `json:"upper,omitempty"`
	Pool  *types.ReqNode       `json:"pool,omitempty"`
	Rows  []*types.Requirement `json:"rows,omitempty"`
	Tail  []string             `json:"tail,omitempty"`
}

// jFloor and jCap build an unnumbered constraint and limit row.
func jFloor(req, raw string) string {
	return fmt.Sprintf(`{"number":0,"kind":"constraint","req":%s,"raw":%q}`, req, raw)
}

func jCap(req, raw string) string {
	return fmt.Sprintf(`{"number":0,"kind":"limit","req":%s,"raw":%q}`, req, raw)
}

// Header lines from the calendar, by production. Each want is the header as
// JSON: the total, the range's upper end, Req 1's pool, the floors and caps
// stated with the total, and the sentences left to the classifier.
func TestParseHeader(t *testing.T) {
	tests := []struct{ name, raw, want string }{
		// 1. The total alone, in every wrapping.
		{"paren", "(7.0 credits)", `{"total":7}`},
		{"bold paren", "**(3.0 credits)**", `{"total":3}`},
		{"bold paren colon", "**(7.0 credits):**", `{"total":7}`},
		{"whole number", "(10 credits)", `{"total":10}`},
		{"full courses", "(4 full courses or equivalent)", `{"total":4}`},
		{"bare", "3.0 Credits", `{"total":3}`},
		{"bare total word", "2.5 credits (total)", `{"total":2.5}`},
		{"focus sentence", "This Focus requires the completion of 2.5 credits.", `{"total":2.5}`},
		{"focus sentence, no article", "This Focus requires completion of 2.5 credits.", `{"total":2.5}`},
		{"program course requirements", "_**Program Course Requirements: 13.0 credits**_", `{"total":13}`},
		{"completion requires", "Completion of the program requires 7.0 credits, meeting the following requirements:", `{"total":7}`},
		{"specified as follows", "**7.0 credits specified as follows:**", `{"total":7}`},
		{"total colon", "Total: 2.0 credits", `{"total":2}`},
		{"paren total colon", "(Total: 2.5 credits)", `{"total":2.5}`},
		{"students complete", "Students complete 7.0 credits, including:", `{"total":7}`},
		{"chosen from those listed below", "4.0 credits chosen from those listed below, including at least 1.0 credit at the 300+ level",
			`{"total":4,"rows":[` + jFloor(`{"type":"credits","amount":1,"levels":[300,400]}`, "including at least 1.0 credit at the 300+ level") + `]}`},

		// 2. Floors stated with the total.
		{"one 0.5 at the 400-level", "(8.0 credits, including at least one 0.5 credit at the 400-level)",
			`{"total":8,"rows":[` + jFloor(`{"type":"credits","amount":0.5,"levels":[400]}`, "including at least one 0.5 credit at the 400-level") + `]}`},
		{"300+ floor", "(7.0 credits, including at least 2.0 credits at the 300+ level)",
			`{"total":7,"rows":[` + jFloor(`{"type":"credits","amount":2,"levels":[300,400]}`, "including at least 2.0 credits at the 300+ level") + `]}`},
		{"with at least", "Total: 2.0 credits, with at least 1.0 credit at the 200+ level",
			`{"total":2,"rows":[` + jFloor(`{"type":"credits","amount":1,"levels":[200,300,400]}`, "with at least 1.0 credit at the 200+ level") + `]}`},
		{"level before credit", "(10.0 credits, including at least 1.0 400-level credit)",
			`{"total":10,"rows":[` + jFloor(`{"type":"credits","amount":1,"levels":[400]}`, "including at least 1.0 400-level credit") + `]}`},
		{"300- or 400-level", "(4.0 credits, including at least 1.0 credit at the 300- or 400-level)",
			`{"total":4,"rows":[` + jFloor(`{"type":"credits","amount":1,"levels":[300,400]}`, "including at least 1.0 credit at the 300- or 400-level") + `]}`},
		{"300/400-level", "(4.0 credits, including 1.0 credit at the 300/400-level.)",
			`{"total":4,"rows":[` + jFloor(`{"type":"credits","amount":1,"levels":[300,400]}`, "including 1.0 credit at the 300/400-level") + `]}`},
		{"from 400-level courses", "(14.0 credits, including 1.5 credits from 400-level courses)",
			`{"total":14,"rows":[` + jFloor(`{"type":"credits","amount":1.5,"levels":[400]}`, "including 1.5 credits from 400-level courses") + `]}`},
		{"level or above", "4.0 ENG credits as follows, including at least 1.5 credits at the 300 level or above",
			`{"total":4,"pool":{"type":"credits","designators":["ENG"]},"rows":[` + jFloor(`{"type":"credits","amount":1.5,"designators":["ENG"],"levels":[300,400]}`, "including at least 1.5 credits at the 300 level or above") + `]}`},
		{"full-course equivalent", "(4 full courses or their equivalent; must include at least one full-course equivalent at the 300+-level)",
			`{"total":4,"rows":[` + jFloor(`{"type":"credits","amount":1,"levels":[300,400]}`, "must include at least one full-course equivalent at the 300+-level") + `]}`},
		{"alternative amounts take the lower", "(4.0 credits, including 1.0 or 1.5 credits at the 300+ level)",
			`{"total":4,"rows":[` + jFloor(`{"type":"credits","amount":1,"levels":[300,400]}`, "including 1.0 or 1.5 credits at the 300+ level") + `]}`},

		// 3. Nested floors: "of which", "N of which", "with", "including".
		{"of which a 0.5", "(6.5 credits, including 2.0 credits at the 300+ level, of which a 0.5 credit must be at the 400-level)",
			`{"total":6.5,"rows":[` +
				jFloor(`{"type":"credits","amount":2,"levels":[300,400]}`, "including 2.0 credits at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"levels":[400]}`, "of which a 0.5 credit must be at the 400-level") + `]}`},
		{"N of which", "(12.0 credits including at least 4.0 credits at the 300+ level, 1.0 of which must be at the 400-level)",
			`{"total":12,"rows":[` +
				jFloor(`{"type":"credits","amount":4,"levels":[300,400]}`, "including at least 4.0 credits at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":1,"levels":[400]}`, "1.0 of which must be at the 400-level") + `]}`},
		{"N credit of which", "(10.0 credits, including 4.0 credits at the 300+ level, 1.0 credit of which must be at the 400-level.)",
			`{"total":10,"rows":[` +
				jFloor(`{"type":"credits","amount":4,"levels":[300,400]}`, "including 4.0 credits at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":1,"levels":[400]}`, "1.0 credit of which must be at the 400-level") + `]}`},
		{"with at least, series", "(8.0 credits including at least 2.0 credits at 300+ series with at least 0.5 credit at the 400-series)",
			`{"total":8,"rows":[` +
				jFloor(`{"type":"credits","amount":2,"levels":[300,400]}`, "including at least 2.0 credits at 300+ series") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"levels":[400]}`, "with at least 0.5 credit at the 400-series") + `]}`},
		{"of which at least, then including", "(7.0 credits, of which at least 2.5 must be at the 300+ level, including at least 0.5 credit at the 400-level).",
			`{"total":7,"rows":[` +
				jFloor(`{"type":"credits","amount":2.5,"levels":[300,400]}`, "of which at least 2.5 must be at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"levels":[400]}`, "including at least 0.5 credit at the 400-level") + `]}`},
		{"300+ level and a 0.5 at the 400-level", "(8.0 credits, including at least 2.0 credits at the 300+ level, and a 0.5 credit at the 400-level)",
			`{"total":8,"rows":[` +
				jFloor(`{"type":"credits","amount":2,"levels":[300,400]}`, "including at least 2.0 credits at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"levels":[400]}`, "a 0.5 credit at the 400-level") + `]}`},
		{"two separate levels, advice after a semicolon",
			"(8.0 credits, including at least 1.5 credits at the 300-level and 1.0 credit at the 400-level; other equivalent and approved courses offered by other Faculties, University of Toronto Mississauga or University of Toronto Scarborough may be eligible for inclusion.)",
			`{"total":8,"rows":[` +
				jFloor(`{"type":"credits","amount":1.5,"levels":[300]}`, "including at least 1.5 credits at the 300-level") + `,` +
				jFloor(`{"type":"credits","amount":1,"levels":[400]}`, "1.0 credit at the 400-level") + `],` +
				`"tail":["other equivalent and approved courses offered by other Faculties, University of Toronto Mississauga or University of Toronto Scarborough may be eligible for inclusion."]}`},
		// "including" puts the 400-level credit inside the 300-level floor, so
		// that floor counts 400-level courses too (ASMAJ2660).
		{"an inner floor widens the levels of the floor it is in", "(8.0 credits with at least 2.0 credits at the 300 level, including 0.5 credit at the 400 level )",
			`{"total":8,"rows":[` +
				jFloor(`{"type":"credits","amount":2,"levels":[300,400]}`, "with at least 2.0 credits at the 300 level") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"levels":[400]}`, "including 0.5 credit at the 400 level") + `]}`},

		// 4. Ranges: the lower end is the total, the upper end is kept.
		{"range written backwards", "(15.5-14.0 credits)", `{"total":14,"upper":15.5}`},
		{"range, nothing else (the plan's example)", "(14.0-15.5 credits)", `{"total":14,"upper":15.5}`},
		{"range with a floor", "(14.0-15.5 credits, including at least 2.0 credits at the 400-level)",
			`{"total":14,"upper":15.5,"rows":[` + jFloor(`{"type":"credits","amount":2,"levels":[400]}`, "including at least 2.0 credits at the 400-level") + `]}`},
		{"bold range", "**12.0-12.5 credits**", `{"total":12,"upper":12.5}`},
		{"range with to", "(11.0 to 12.0 credits)", `{"total":11,"upper":12}`},
		{"range with to and with", "(13.5 to 14.0 credits with at least 1.0 credit at the 400-level)",
			`{"total":13.5,"upper":14,"rows":[` + jFloor(`{"type":"credits","amount":1,"levels":[400]}`, "with at least 1.0 credit at the 400-level") + `]}`},
		{"or range, plus a focus", "(10.5 or 11.0 credits plus a disciplinary focus requiring 2.0-3.5 credits)",
			`{"total":10.5,"upper":11,"tail":["plus a disciplinary focus requiring 2.0-3.5 credits"]}`},

		// 5. Floors naming designators (and Req 1's pool narrowing them).
		{"CHM credit", "(8.0 credits, including at least one 0.5 CHM credit at the 400-level)",
			`{"total":8,"rows":[` + jFloor(`{"type":"credits","amount":0.5,"designators":["CHM"],"levels":[400]}`, "including at least one 0.5 CHM credit at the 400-level") + `]}`},
		{"CHM course", "(4.0 credits, including 1.0 credit CHM course at the 300+ level)",
			`{"total":4,"rows":[` + jFloor(`{"type":"credits","amount":1,"designators":["CHM"],"levels":[300,400]}`, "including 1.0 credit CHM course at the 300+ level") + `]}`},
		{"STA series course", "(7.0 or 7.5 credits, including a 0.5 credit STA 400-series course)",
			`{"total":7,"upper":7.5,"rows":[` + jFloor(`{"type":"credits","amount":0.5,"designators":["STA"],"levels":[400]}`, "including a 0.5 credit STA 400-series course") + `]}`},
		{"SDS, nested", "(6.0 credits including 2.5 SDS credits at the 300+ level, 1.0 credit of which must be at the 400-level)",
			`{"total":6,"rows":[` +
				jFloor(`{"type":"credits","amount":2.5,"designators":["SDS"],"levels":[300,400]}`, "including 2.5 SDS credits at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":1,"designators":["SDS"],"levels":[400]}`, "1.0 credit of which must be at the 400-level") + `]}`},
		{"CRI, parenthesized floors", "Total credits: 11.0 of which 7.0 credits must be CRI courses (including 3.0 credits at the 300+ level and 1.0 credit at the 400-level).",
			`{"total":11,"rows":[` +
				jFloor(`{"type":"credits","amount":7,"designators":["CRI"]}`, "of which 7.0 credits must be CRI courses") + `,` +
				jFloor(`{"type":"credits","amount":3,"designators":["CRI"],"levels":[300,400]}`, "including 3.0 credits at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":1,"designators":["CRI"],"levels":[400]}`, "1.0 credit at the 400-level") + `]}`},
		{"CRI, of these", "Students must complete a total of 8.0 credits including a minimum of 2.0 credits from 300+level CRI courses, of these 0.5 CRI credit must be at the 400-level.",
			`{"total":8,"rows":[` +
				jFloor(`{"type":"credits","amount":2,"designators":["CRI"],"levels":[300,400]}`, "including a minimum of 2.0 credits from 300+level CRI courses") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"designators":["CRI"],"levels":[400]}`, "of these 0.5 CRI credit must be at the 400-level") + `]}`},
		{"one of which must be a course", "(10.0 credits, including 5.0 credits at the 300+ level, including at least 2.0 WGS credits at the 400 level, one of which must be WGS460Y1)",
			`{"total":10,"rows":[` +
				jFloor(`{"type":"credits","amount":5,"levels":[300,400]}`, "including 5.0 credits at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":2,"designators":["WGS"],"levels":[400]}`, "including at least 2.0 WGS credits at the 400 level") + `,` +
				jFloor(`{"type":"credits","amount":1,"courses":["WGS460Y1"]}`, "one of which must be WGS460Y1") + `]}`},
		{"IRE course, as noted below", "(10.0 credits, including 4.0 credits at the 300+ level, of which 1.0 credit must be a 400-level IRE course, as noted below.)",
			`{"total":10,"rows":[` +
				jFloor(`{"type":"credits","amount":4,"levels":[300,400]}`, "including 4.0 credits at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":1,"designators":["IRE"],"levels":[400]}`, "of which 1.0 credit must be a 400-level IRE course") + `]}`},
		{"second sentence names designators", "4.0 credits, including at least 1.0 credit at the 300+ level. All 4.0 credits must be NMC and/or NML courses.",
			`{"total":4,"rows":[` +
				jFloor(`{"type":"credits","amount":1,"levels":[300,400]}`, "including at least 1.0 credit at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":4,"designators":["NMC","NML"]}`, "All 4.0 credits must be NMC and/or NML courses") + `]}`},
		{"subject the grammar can't check", "(6.0 credits, including 2.0 credits in Hungarian language study)",
			`{"total":6,"rows":[` + jFloor(jAnd(`{"type":"credits","amount":2}`, jT("other", "including 2.0 credits in Hungarian language study")), "including 2.0 credits in Hungarian language study") + `]}`},

		// 6. Caps stated with the total.
		{"no more than, and at least", "(6.0 credits, no more than 1.0 credit at the 100-level, and at least 2.0 credits at 300+ level)",
			`{"total":6,"rows":[` +
				jCap(`{"type":"credits","levels":[100],"max_amount":1}`, "no more than 1.0 credit at the 100-level") + `,` +
				jFloor(`{"type":"credits","amount":2,"levels":[300,400]}`, "at least 2.0 credits at 300+ level") + `]}`},
		{"cap, then a nested floor", "(7.5 credits, no more than 1.0 credit at the 100-level, at least 2.0 credits at the 300+ level including a 0.5 credit at the 400-level)",
			`{"total":7.5,"rows":[` +
				jCap(`{"type":"credits","levels":[100],"max_amount":1}`, "no more than 1.0 credit at the 100-level") + `,` +
				jFloor(`{"type":"credits","amount":2,"levels":[300,400]}`, "at least 2.0 credits at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"levels":[400]}`, "including a 0.5 credit at the 400-level") + `]}`},
		{"of which only", "(4.0 credits, of which only 1.0 credit at the 100-level may be used to fulfill program requirements)",
			`{"total":4,"rows":[` + jCap(`{"type":"credits","levels":[100],"max_amount":1}`, "of which only 1.0 credit at the 100-level may be used to fulfill program requirements") + `]}`},
		{"with a maximum of, then a sentence for the classifier",
			"4.0 credits including at least 1.0 credit at the 300+ level, with a maximum of 1.0 credit at the 100-level. Up to 1.0 credit may be chosen from approved courses offered by other departments (see list of Cognate Courses).",
			`{"total":4,"rows":[` +
				jFloor(`{"type":"credits","amount":1,"levels":[300,400]}`, "including at least 1.0 credit at the 300+ level") + `,` +
				jCap(`{"type":"credits","levels":[100],"max_amount":1}`, "with a maximum of 1.0 credit at the 100-level") + `],` +
				`"tail":["Up to 1.0 credit may be chosen from approved courses offered by other departments (see list of Cognate Courses)."]}`},
		{"cap and floor both of the total",
			"A total of 7.0 credits of which no more than 1.0 credit can be at the 100-level and of which at least 2.0 credits must be at the 300+ level, including 0.5 credit at the 400-level. Some courses may contribute to both the Geographic Distribution and Temporal requirements (requirements 1 and 2).",
			`{"total":7,"rows":[` +
				jCap(`{"type":"credits","levels":[100],"max_amount":1}`, "of which no more than 1.0 credit can be at the 100-level") + `,` +
				jFloor(`{"type":"credits","amount":2,"levels":[300,400]}`, "of which at least 2.0 credits must be at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"levels":[400]}`, "including 0.5 credit at the 400-level") + `],` +
				`"tail":["Some courses may contribute to both the Geographic Distribution and Temporal requirements (requirements 1 and 2)."]}`},

		{"including after a cap narrows the total, not the cap", "(4.0 credits, no more than 1.0 credit in SPA courses, including 1.0 credit at the 300+ level)",
			`{"total":4,"rows":[` +
				jCap(`{"type":"credits","designators":["SPA"],"max_amount":1}`, "no more than 1.0 credit in SPA courses") + `,` +
				jFloor(`{"type":"credits","amount":1,"levels":[300,400]}`, "including 1.0 credit at the 300+ level") + `]}`},
		{"a cap the grammar can't check goes to the classifier", "(7.0 credits, no more than 1.0 credit in PRT courses taught in Portuguese)",
			`{"total":7,"tail":["no more than 1.0 credit in PRT courses taught in Portuguese"]}`},

		// 7. The total at the bottom.
		{"bottom total", "**Total credits: 7.0**", `{"total":7}`},
		{"bottom total 12", "**Total credits: 12.0**", `{"total":12}`},
		{"total equals", "Total = 12.0 credits", `{"total":12}`},

		// 8. Inside "Required Courses (…)".
		{"required courses", "Required Courses (8.0 credits)", `{"total":8}`},
		{"required courses, a 0.5", "**Required Courses (8.0 credits, including a 0.5 credit at the 400-level)**",
			`{"total":8,"rows":[` + jFloor(`{"type":"credits","amount":0.5,"levels":[400]}`, "including a 0.5 credit at the 400-level") + `]}`},
		{"required courses, 400 level", "**Required Courses (12.0 credits, including at least 1.0 credit at the 400 level)**",
			`{"total":12,"rows":[` + jFloor(`{"type":"credits","amount":1,"levels":[400]}`, "including at least 1.0 credit at the 400 level") + `]}`},

		// 9. Designator totals: Req 1's own pool.
		{"designator total", "(4.0 GER credits)", `{"total":4,"pool":{"type":"credits","designators":["GER"]}}`},
		{"subject total", "(7.0 credits in Sociology)", `{"total":7,"pool":{"type":"credits","designators":["SOC"]}}`},
		{"program name is not a pool", "(4.0 credits in Visual Studies, including at least 1.0 credit at the 300-level.)",
			`{"total":4,"rows":[` + jFloor(`{"type":"credits","amount":1,"levels":[300]}`, "including at least 1.0 credit at the 300-level") + `]}`},
		{"designator total, floor narrows it", "4.0 ENG credits, including 1.0 credit from 300+level ENG courses.",
			`{"total":4,"pool":{"type":"credits","designators":["ENG"]},"rows":[` +
				jFloor(`{"type":"credits","amount":1,"designators":["ENG"],"levels":[300,400]}`, "including 1.0 credit from 300+level ENG courses") + `]}`},
		{"designator total with a topic", "7.0 ENG credits including 2.0 credits from 300+ level Creative Writing courses, at least 0.5 credit of which must be at the 400-level.",
			`{"total":7,"pool":{"type":"credits","designators":["ENG"]},"rows":[` +
				jFloor(jAnd(`{"type":"credits","amount":2,"designators":["ENG"],"levels":[300,400]}`, jT("other", "including 2.0 credits from 300+ level Creative Writing courses")), "including 2.0 credits from 300+ level Creative Writing courses") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"designators":["ENG"],"levels":[400]}`, "at least 0.5 credit of which must be at the 400-level") + `]}`},
		{"designator and level pool with a language condition", "2.0 PRT credits at the 200+ level taught in Portuguese, including at least 0.5 credit at the 300+ level.",
			`{"total":2,"pool":{"type":"credits","designators":["PRT"],"levels":[200,300,400]},"rows":[` +
				jFloor(jAnd(`{"type":"credits","amount":2,"designators":["PRT"],"levels":[200,300,400]}`, jT("other", "2.0 PRT credits at the 200+ level taught in Portuguese")), "2.0 PRT credits at the 200+ level taught in Portuguese") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"designators":["PRT"],"levels":[300,400]}`, "including at least 0.5 credit at the 300+ level") + `]}`},

		{"courses to a total of", "**Completion Requirements:** HIS courses to a total of 4.0 credits, including at least 1.0 credit at the 300- or 400-level.",
			`{"total":4,"pool":{"type":"credits","designators":["HIS"]},"rows":[` +
				jFloor(`{"type":"credits","amount":1,"designators":["HIS"],"levels":[300,400]}`, "including at least 1.0 credit at the 300- or 400-level") + `]}`},
		{"restated pool, taught in French", "2.0 FSL or FRE credits taught in French.",
			`{"total":2,"pool":{"type":"credits","designators":["FSL","FRE"]},"rows":[` +
				jFloor(jAnd(`{"type":"credits","amount":2,"designators":["FSL","FRE"]}`, jT("other", "2.0 FSL or FRE credits taught in French")), "2.0 FSL or FRE credits taught in French") + `]}`},
		{"restated pool, taught in German", "2.0 GER credits taught in German",
			`{"total":2,"pool":{"type":"credits","designators":["GER"]},"rows":[` +
				jFloor(jAnd(`{"type":"credits","amount":2,"designators":["GER"]}`, jT("other", "2.0 GER credits taught in German")), "2.0 GER credits taught in German") + `]}`},

		// 10. Per-designator caps.
		{"cap on cognates", "Students complete 4.0 credits, of which up to 1.0 credit may be taken in SPA/LAS courses.",
			`{"total":4,"rows":[` + jCap(`{"type":"credits","designators":["SPA","LAS"],"max_amount":1}`, "of which up to 1.0 credit may be taken in SPA/LAS courses") + `]}`},
		{"cap on cognates, then advice", "Students complete 4.0 credits, of which up to 1.0 cognate credit may be taken in PRT. No other cognates are permitted for a Spanish Minor.",
			`{"total":4,"rows":[` + jCap(`{"type":"credits","designators":["PRT"],"max_amount":1}`, "of which up to 1.0 cognate credit may be taken in PRT") + `],` +
				`"tail":["No other cognates are permitted for a Spanish Minor."]}`},
		{"the minor program requires", "The Minor Program requires 4.0 credits, of which 3.0 credits must be GER courses.",
			`{"total":4,"rows":[` + jFloor(`{"type":"credits","amount":3,"designators":["GER"]}`, "of which 3.0 credits must be GER courses") + `]}`},

		{"one of which, with alternatives", "(7.0 credits, including at least 2.0 credits at the 300+ level and 0.5 credit at the 400-level, one of which must be ENG400H1 or ENG401H1)",
			`{"total":7,"rows":[` +
				jFloor(`{"type":"credits","amount":2,"levels":[300,400]}`, "including at least 2.0 credits at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"levels":[400]}`, "0.5 credit at the 400-level") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"courses":["ENG400H1","ENG401H1"]}`, "one of which must be ENG400H1 or ENG401H1") + `]}`},

		// Text that isn't the total or a floor goes to the classifier.
		{"please note", "(7.0 credits. Please note: each course can only be counted towards one of the requirements below)",
			`{"total":7,"tail":["Please note: each course can only be counted towards one of the requirements below"]}`},
		{"text after the parentheses", "(8.0 credits, including at least 1.0 credit at the 400-level) Students should consult the department.",
			`{"total":8,"rows":[` + jFloor(`{"type":"credits","amount":1,"levels":[400]}`, "including at least 1.0 credit at the 400-level") + `],` +
				`"tail":["Students should consult the department."]}`},
		{"if taken", "(7.0 credits, including PHL100Y1/ PHL101Y1 if taken)",
			`{"total":7,"tail":["including PHL100Y1/ PHL101Y1 if taken"]}`},
		{"clusters the grammar can't check",
			"(4.0 credits, including 1.0 credit at the 300+ level in at least two American Studies Program disciplinary/thematic clusters, categorized as follows: a) Politics and Economics b) Society (e.g. Indigenous Studies, Anthropology, East Asian Studies, Geography) c) Culture (e.g. Cinema Studies, English, Music, Religion) d) History)",
			`{"total":4,"rows":[` + jFloor(jAnd(`{"type":"credits","amount":1,"levels":[300,400]}`,
				jT("other", "including 1.0 credit at the 300+ level in at least two American Studies Program disciplinary/thematic clusters, categorized as follows: a) Politics and Economics b) Society (e.g. Indigenous Studies, Anthropology, East Asian Studies, Geography) c) Culture (e.g. Cinema Studies, English, Music, Religion) d) History")),
				"including 1.0 credit at the 300+ level in at least two American Studies Program disciplinary/thematic clusters, categorized as follows: a) Politics and Economics b) Society (e.g. Indigenous Studies, Anthropology, East Asian Studies, Geography) c) Culture (e.g. Cinema Studies, English, Music, Religion) d) History") + `]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, ok := parseHeader(tt.raw)
			if !ok {
				t.Fatalf("parseHeader(%q) not ok", tt.raw)
			}
			assertJSON(t, hdrView{h.total, h.upper, h.pool, h.rows, h.tail}, tt.want)
		})
	}
}

// Lines that state credits but aren't the program's total.
func TestParseHeaderRejects(t *testing.T) {
	for _, s := range []string{
		"2.5 credits from the following list:",           // an item with a pool (ASFOC1469G)
		"2.0 credits from: SLA117H1; SLA118H1; SLA257H1", // an item (ASCER1200)
		"**Core Biological Physics Courses (12.0 credits):**",
		"Required Courses:",
		"Sub-total = 8.5 credits",
		"0.5 credit at the 400-level",
		"1. 2.0 credits from CRI100H1/ ECO/ HIS/ PHL/ POL/ PSY/ SOC",
		"First Year (2.0 credits): BIO120H1; BIO130H1",
		"",
		"credits",
		"(credits)",
		"(7.0 credits",
		"Total credits:",
		"(7.0 credits, of which)",
	} {
		if h, ok := parseHeader(s); ok {
			t.Errorf("parseHeader(%q) = total %v, want not ok", s, h.total)
		}
	}
}

// Program-level floors and caps on their own line: the "Of these:" bullets
// after the total (ASMAJ2015) and lines like them.
func TestParseHeaderFloors(t *testing.T) {
	tests := []struct{ name, raw, rows, tail string }{
		{"300+ including 400", "- 2.0 credits at the 300+ level including a 0.5 credit at the 400 level.",
			`[` + jFloor(`{"type":"credits","amount":2,"levels":[300,400]}`, "2.0 credits at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"levels":[400]}`, "including a 0.5 credit at the 400 level") + `]`, `null`},
		{"specialist version", "- 4.0 credits at the 300+ level including 1.0 credit at the 400 level.",
			`[` + jFloor(`{"type":"credits","amount":4,"levels":[300,400]}`, "4.0 credits at the 300+ level") + `,` +
				jFloor(`{"type":"credits","amount":1,"levels":[400]}`, "including 1.0 credit at the 400 level") + `]`, `null`},
		{"100-level cap", "- No more than 1.0 credit of 100-level courses may be used to fulfill the program requirements.",
			`[` + jCap(`{"type":"credits","levels":[100],"max_amount":1}`, "No more than 1.0 credit of 100-level courses may be used to fulfill the program requirements") + `]`, `null`},
		{"designators, then of these", "- 4.0 POL/ JPA/ JPF/ JPI/ JPR/ JPS/ JRA credits. Of these, at least 1.0 credit must be at the 300+ level.",
			`[` + jFloor(`{"type":"credits","amount":4,"designators":["POL","JPA","JPF","JPI","JPR","JPS","JRA"]}`, "4.0 POL/ JPA/ JPF/ JPI/ JPR/ JPS/ JRA credits") + `,` +
				jFloor(`{"type":"credits","amount":1,"designators":["POL","JPA","JPF","JPI","JPR","JPS","JRA"],"levels":[300,400]}`, "Of these, at least 1.0 credit must be at the 300+ level") + `]`, `null`},
		{"designators, then advice", "At least 5.0 credits must be NMC and/or NML courses. 1.0 credit may be taken from courses offered by other departments, pending approval by the Associate Chair Undergraduate of NMC.",
			`[` + jFloor(`{"type":"credits","amount":5,"designators":["NMC","NML"]}`, "At least 5.0 credits must be NMC and/or NML courses") + `]`,
			`["1.0 credit may be taken from courses offered by other departments, pending approval by the Associate Chair Undergraduate of NMC."]`},
		{"of the M credits restates the scope", "  - At least 1.0 credit of the 2.5 credits must be at the 300-level.",
			`[` + jFloor(`{"type":"credits","amount":1,"levels":[300]}`, "At least 1.0 credit of the 2.5 credits must be at the 300-level") + `]`, `null`},
		{"note label", "Note: At least 1.0 credit must be 300/400-level courses.",
			`[` + jFloor(`{"type":"credits","amount":1,"levels":[300,400]}`, "At least 1.0 credit must be 300/400-level courses") + `]`, `null`},
		{"note label, no full stop", "Note: At least 1.0 credit must be 300/400-level courses",
			`[` + jFloor(`{"type":"credits","amount":1,"levels":[300,400]}`, "At least 1.0 credit must be 300/400-level courses") + `]`, `null`},
		{"no space after the full stop", "At least 5.0 credits must be NMC and/or NML courses.1.0 credit may be taken from courses offered by other departments, pending approval by the Associate Chair Undergraduate of NMC.",
			`[` + jFloor(`{"type":"credits","amount":5,"designators":["NMC","NML"]}`, "At least 5.0 credits must be NMC and/or NML courses") + `]`,
			`["1.0 credit may be taken from courses offered by other departments, pending approval by the Associate Chair Undergraduate of NMC."]`},
		{"may count toward the program’s completion", "- No more than 1.0 credit of 100 level courses may count toward the program’s completion.",
			`[` + jCap(`{"type":"credits","levels":[100],"max_amount":1}`, "No more than 1.0 credit of 100 level courses may count toward the program’s completion") + `]`, `null`},
		{"note label, closing remark", "Note: At least 2.0 credits must be 300/400-level courses. Of these, at least 0.5 must be at the 400-level (includes Fourth Year Course requirement).",
			`[` + jFloor(`{"type":"credits","amount":2,"levels":[300,400]}`, "At least 2.0 credits must be 300/400-level courses") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"levels":[400]}`, "Of these, at least 0.5 must be at the 400-level") + `]`,
			`["(includes Fourth Year Course requirement)"]`},
		{"one 0.5, closing remark", "Note: At least 2.0 credits must be 300/400-level courses. Of these, at least one 0.5 credit must be at the 400-level (includes Fourth Year Course requirement).",
			`[` + jFloor(`{"type":"credits","amount":2,"levels":[300,400]}`, "At least 2.0 credits must be 300/400-level courses") + `,` +
				jFloor(`{"type":"credits","amount":0.5,"levels":[400]}`, "Of these, at least one 0.5 credit must be at the 400-level") + `]`,
			`["(includes Fourth Year Course requirement)"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, tail, ok := parseHeaderFloors(tt.raw)
			if !ok {
				t.Fatalf("parseHeaderFloors(%q) not ok", tt.raw)
			}
			assertJSON(t, rows, tt.rows)
			assertJSON(t, tail, tt.tail)
		})
	}
	for _, s := range []string{
		// The pool is "courses from outside of the Department": a list
		// the grammar doesn't read.
		"- No more than 1.0 credit of courses from outside of the Department of Political Science may be used to fulfill the program requirements. See the bottom of the completion requirements section for a full list of such acceptable courses.",
		"First Year:",
		"1. POL200Y1",
		"- Some courses may contribute to both the Canadian Politics and Diversity and Identity requirements (in requirements 4 and 6). Refer to the Department website for a full list of courses that meet these requirements.",
	} {
		if rows, _, ok := parseHeaderFloors(s); ok {
			t.Errorf("parseHeaderFloors(%q) = %d rows, want not ok", s, len(rows))
		}
	}
}

// Rows and the pool are separate values: changing one leaves the others.
func TestParseHeaderRowsDontAlias(t *testing.T) {
	h, ok := parseHeader("4.0 ENG credits, including 1.0 credit at the 300+ level, of which 0.5 credit at the 400-level")
	if !ok || len(h.rows) != 2 {
		t.Fatalf("parseHeader: ok %v, %d rows", ok, len(h.rows))
	}
	h.pool.Designators[0] = "XXX"
	h.rows[0].Req.Levels[0] = 999
	assertJSON(t, h.rows, `[`+
		jFloor(`{"type":"credits","amount":1,"designators":["ENG"],"levels":[999,400]}`, "including 1.0 credit at the 300+ level")+`,`+
		jFloor(`{"type":"credits","amount":0.5,"designators":["ENG"],"levels":[400]}`, "of which 0.5 credit at the 400-level")+`]`)

	rows, _, _ := parseHeaderFloors("- 4.0 POL/ JPA/ JPF/ JPI/ JPR/ JPS/ JRA credits. Of these, at least 1.0 credit must be at the 300+ level.")
	rows[0].Req.Designators[0] = "XXX"
	if got := rows[1].Req.Designators[0]; got != "POL" {
		t.Errorf("second row's designators changed with the first's: %q", got)
	}
}
