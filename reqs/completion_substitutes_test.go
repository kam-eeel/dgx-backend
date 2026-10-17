package reqs

import "testing"

func TestParseCompletionSubstitutes(t *testing.T) {
	tests := []struct {
		name, raw string
		rows      []string
		fields    []string
	}{
		{"an exemption's replacement bounds the courses it replaces by the pool it names, with the same credits (ASMIN0120, shortened)",
			"(2.0 credits)\n1) ( FSL120H1, FSL122H1)/ FSL121Y1\n2) 1.0 credit of FSL or FRE courses at the 200+ level\nNote: Students exempted from any of the required courses must replace these courses with the equivalent number of credits of additional FSL courses at the 200+ level.",
			[]string{
				jRow(1, "allocation", 0, "", "", `{"type":"credits","amount":2}`,
					"(2.0 credits)"),
				jRow(2, "allocation", 1, "1", "", `{"type":"or","items":[{"type":"and","items":[{"type":"course","code":"FSL120H1"},{"type":"course","code":"FSL122H1"}]},{"type":"course","code":"FSL121Y1"},{"type":"and","items":[{"type":"credits","amount":1,"designators":["FSL"],"levels":[200,300,400]},{"type":"text","kind":"placement","text":"Students exempted from any of the required courses must replace these courses with the equivalent number of credits of additional FSL courses at the 200+ level"}]}]}`,
					"1) ( FSL120H1, FSL122H1)/ FSL121Y1\nStudents exempted from any of the required courses must replace these courses with the equivalent number of credits of additional FSL courses at the 200+ level"),
				jRow(3, "allocation", 1, "2", "", `{"type":"credits","amount":1,"designators":["FSL","FRE"],"levels":[200,300,400]}`,
					"2) 1.0 credit of FSL or FRE courses at the 200+ level"),
			}, nil},
		{"a course that also fulfils the requirement joins the row's courses (ASMAJ0608, shortened)",
			"(0.5 credit)\nTo be completed before the end of Second Year:\n4. CSC108H1/ CSC120H1/ CSC148H1. A student who has completed CSC110Y1 also fulfills this program requirement.",
			[]string{
				jRow(1, "allocation", 0, "", "", `{"type":"credits","amount":0.5}`,
					"(0.5 credit)"),
				jRow(2, "allocation", 1, "4", "To be completed before the end of Second Year", `{"type":"or","items":[{"type":"course","code":"CSC108H1"},{"type":"course","code":"CSC120H1"},{"type":"course","code":"CSC148H1"},{"type":"course","code":"CSC110Y1"}]}`,
					"4. CSC108H1/ CSC120H1/ CSC148H1. A student who has completed CSC110Y1 also fulfills this program requirement."),
			}, nil},
		{"a substitute named for a requirement stands beside the course it replaces (ASSPE1944, shortened)",
			"Third Year: (2.5 credits)\n1. APM346H1, MAT334H1/ MAT354H1, PHY350H1, PHY354H1, PHY356H1\nNotes:\n2. Students may use MAT351Y1 instead of APM346H1 for Requirement 1 in Third Year.",
			[]string{
				jRow(1, "allocation", 0, "1", "Third Year", `{"type":"and","items":[{"type":"or","items":[{"type":"course","code":"APM346H1"},{"type":"course","code":"MAT351Y1"}]},{"type":"or","items":[{"type":"course","code":"MAT334H1"},{"type":"course","code":"MAT354H1"}]},{"type":"course","code":"PHY350H1"},{"type":"course","code":"PHY354H1"},{"type":"course","code":"PHY356H1"}]}`,
					"1. APM346H1, MAT334H1/ MAT354H1, PHY350H1, PHY354H1, PHY356H1\nStudents may use MAT351Y1 instead of APM346H1 for Requirement 1 in Third Year"),
			}, nil},
		{"a course counted in lieu of a 300-level elective is the pool's alternative, the pool less 0.5 and the course (ASSPE1478, shortened)",
			"_**Program Course Requirements: 4.5 credits**_\n_Third Year and Higher:_\n1. 4.5 credits from 300+ level ECO courses\n_Notes_:\n2. ECO210H1 is highly recommended, especially for students interested in graduate studies. Students may count ECO210H1 towards the Specialist program in lieu of a 0.5 credit 300-level ECO elective.",
			[]string{
				jRow(1, "allocation", 0, "", "", `{"type":"credits","amount":4.5}`,
					"_**Program Course Requirements: 4.5 credits**_"),
				jRow(2, "allocation", 1, "1", "Third Year and Higher", `{"type":"or","items":[{"type":"credits","amount":4.5,"designators":["ECO"],"levels":[300,400]},{"type":"and","items":[{"type":"credits","amount":4,"designators":["ECO"],"levels":[300,400]},{"type":"course","code":"ECO210H1"}]}]}`,
					"1. 4.5 credits from 300+ level ECO courses\nStudents may count ECO210H1 towards the Specialist program in lieu of a 0.5 credit 300-level ECO elective"),
			}, []string{`"notes":["ECO210H1 is highly recommended, especially for students interested in graduate studies"]`}},
		{"a named equivalence the row already lists is the row's raw and notes, not a row of text (ASSPE2038, shortened)",
			"(1.0 credit)\n5. ECO220Y1/ ECO227Y1/( STA220H1, STA255H1)*\n*The combination of ( STA220H1, STA255H1) is considered equivalent to ECO220Y1 for the purposes of completing Rotman Commerce program requirements only. It is not considered equivalent by the department of Economics.",
			[]string{
				jRow(1, "allocation", 0, "", "", `{"type":"credits","amount":1}`,
					"(1.0 credit)"),
				jRow(2, "allocation", 1, "5", "", `{"type":"or","items":[{"type":"course","code":"ECO220Y1"},{"type":"course","code":"ECO227Y1"},{"type":"and","items":[{"type":"course","code":"STA220H1"},{"type":"course","code":"STA255H1"}]}]}`,
					"5. ECO220Y1/ ECO227Y1/( STA220H1, STA255H1)*\n*The combination of ( STA220H1, STA255H1) is considered equivalent to ECO220Y1 for the purposes of completing Rotman Commerce program requirements only. It is not considered equivalent by the department of Economics.", `"notes":["It is not considered equivalent by the department of Economics"]`),
			}, nil},
		{"courses that may not be counted toward the program are a limit of 0 under the total (ASMIN1645, shortened)",
			"(4.0 credits)\n1. 4.0 ENG credits\nNotes:\n- ENG386H1, ENG387H1, ENG388H1, ENG389H1, ENG394H1, ENG395H1, ENG496H1, ENG497H1, and ENG498H1 may not be counted toward fulfilling the requirements for the Minor program in English.",
			[]string{
				jRow(1, "allocation", 0, "", "", `{"type":"credits","amount":4}`,
					"(4.0 credits)"),
				jRow(2, "allocation", 1, "1", "", `{"type":"credits","amount":4,"designators":["ENG"]}`,
					"1. 4.0 ENG credits"),
				jRow(3, "limit", 1, "", "", `{"type":"credits","courses":["ENG386H1","ENG387H1","ENG388H1","ENG389H1","ENG394H1","ENG395H1","ENG496H1","ENG497H1","ENG498H1"],"max_amount":0}`,
					"ENG386H1, ENG387H1, ENG388H1, ENG389H1, ENG394H1, ENG395H1, ENG496H1, ENG497H1, and ENG498H1 may not be counted toward fulfilling the requirements for the Minor program in English"),
			}, nil},
		{"a course that may not be used for this requirement is a limit of 0 under the item before it (ASMAJ2289, shortened)",
			"(1.0 credit)\n2. 1.0 credit from MAT223H1/ MAT224H1/ MAT240H1, MAT235Y1\n( STA237H1 and STA238H1 are strongly recommended. MAT221H1 may not be used for this requirement.)",
			[]string{
				jRow(1, "allocation", 0, "", "", `{"type":"credits","amount":1}`,
					"(1.0 credit)"),
				jRow(2, "allocation", 1, "2", "", `{"type":"credits","amount":1,"courses":["MAT223H1","MAT224H1","MAT240H1","MAT235Y1"]}`,
					"2. 1.0 credit from MAT223H1/ MAT224H1/ MAT240H1, MAT235Y1"),
				jRow(3, "limit", 2, "", "", `{"type":"credits","courses":["MAT223H1","MAT224H1","MAT240H1"],"max_amount":0.5}`,
					"2. 1.0 credit from MAT223H1/ MAT224H1/ MAT240H1, MAT235Y1"),
				jRow(4, "limit", 2, "", "", `{"type":"credits","courses":["MAT221H1"],"max_amount":0}`,
					"MAT221H1 may not be used for this requirement"),
			}, []string{`"notes":["STA237H1 and STA238H1 are strongly recommended"]`}},
		{"a chain of fallbacks is one or: the course, the handbook's courses, the breadth category if none is available, an approved course (ASMAJ0385, shortened)",
			"(0.5 credit)\n4. DTS300H1 or a 0.5 credit to satisfy the Quantitative Reasoning competency of the program, to be chosen from courses in the Centre for Jewish Studies Undergraduate Course Handbook developing this competency. If none is available, a 0.5 credit from Breadth Requirement Category #5: The Physical & Mathematical Universe, or a 0.5 credit approved by the Undergraduate Director.",
			[]string{
				jRow(1, "allocation", 0, "", "", `{"type":"or","items":[{"type":"credits","amount":0.5},{"type":"text","kind":"other","text":"(0.5 credit)"}]}`,
					"(0.5 credit)"),
				jRow(2, "allocation", 1, "4", "", `{"type":"or","items":[{"type":"course","code":"DTS300H1"},{"type":"text","kind":"content","text":"a 0.5 credit to satisfy the Quantitative Reasoning competency of the program, to be chosen from courses in the Centre for Jewish Studies Undergraduate Course Handbook developing this competency"},{"type":"and","items":[{"type":"credits","amount":0.5,"breadth":[5]},{"type":"text","kind":"other","text":"If none is available"}]},{"type":"text","kind":"permission","text":"a 0.5 credit approved by the Undergraduate Director"}]}`,
					"4. DTS300H1 or a 0.5 credit to satisfy the Quantitative Reasoning competency of the program, to be chosen from courses in the Centre for Jewish Studies Undergraduate Course Handbook developing this competency. If none is available, a 0.5 credit from Breadth Requirement Category #5: The Physical & Mathematical Universe, or a 0.5 credit approved by the Undergraduate Director."),
			}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSON(t, ParseCompletion(tt.raw), jComp(tt.raw, tt.rows, tt.fields...))
		})
	}
}
