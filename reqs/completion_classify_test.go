package reqs

import (
	"fmt"
	"reflect"
	"testing"
)

// Every sentence below is taken from the completion text of the program in
// the comment, with list markers and emphasis left as the scraper wrote them
// where they matter.
func TestClassifySentence(t *testing.T) {
	tests := []struct {
		in   string
		want sentenceRole
	}{
		// Allocations, totals and plain items.
		{"(8.0 credits, including at least one 0.5 credit at the 400-level)", roleRequirement},                                                                            // ASMAJ1689: the total leads
		{"7.0 ENG credits including 2.0 credits from 300+ level Literature and Language courses, at least 0.5 credit of which must be at the 400-level", roleRequirement}, // ASMAJ1647A
		{"**Total credits: 8.0**", roleRequirement}, // ASMAJ2030
		{"2. CSC207H1, CSC236H1/ CSC240H1, CSC258H1, CSC263H1/ CSC265H1, STA247H1/ STA237H1/ STA255H1/ STA257H1", roleRequirement},                                              // ASMAJ1689
		{"3. 3.0 credits of courses in total selected from among the following groups:", roleRequirement},                                                                       // ASMAJ1689
		{"7. Additional POL/ JPA/ JPF/ JPI/ JPR/ JPS/ JRA courses to a total of 7.0 credits.", roleRequirement},                                                                 // ASMAJ2015
		{"1. 2.0 credits from among the courses in Group A, B, and C, with no more than 0.5 credit from Group C", roleRequirement},                                              // ASMAJ1076: the cap is the item's own clause
		{"6. 2.0 credits of electives chosen from the following courses below, of which at least 1.0 credit must be at the 300- or 400-level:", roleRequirement},                // ASMAJ1532
		{"5. 1.5 credits (at least 0.5 credit must be at the 300+ level) from: BCH210H1; BIO260H1/ HMB265H1; CJH332H1", roleRequirement},                                        // ASMAJ1538
		{"_Core Courses_: Any 3 courses (1.5 credits) from GGR201H1/ JEG201H1, GGR203H1, GGR205H1/ JEG205H1, GGR206H1/ JEG206H1", roleRequirement},                              // ASMAJ2030
		{"a. CSC110Y1 and CSC111H1 must be completed in order to complete the Specialist program.", roleRequirement},                                                            // ASSPE1689
		{"Students with a strong background in an object-oriented language such as Python, Java or C++ may omit CSC108H1 and proceed directly with CSC148H1.", roleRequirement}, // ASMAJ1689
		{"3. At least second-year proficiency in Chinese, Korean or Japanese", roleRequirement},                                                                                 // ASMAJ1058
		{"Courses must fulfill the following requirements:", roleAdvisory},                                                                                                      // ASMAJ1647A
		{"Higher Years:", roleRequirement}, // ASMAJ2015: a heading
		{"( STA257H1, STA261H1)/ ( STA237H1, STA238H1); while either pair of courses is accepted, we strongly recommend ( STA257H1, STA261H1).", roleRequirement},                                                                                                                         // ASMAJ0608: advice in a later clause
		{"If you take CSC240H1 without CSC165H1, there is no need to replace the missing half-credit for program completion; but please see Note (a).", roleRequirement},                                                                                                                  // ASMAJ1689: an exception to the total
		{"Further Topics: 1.0 credit of any APM/MAT 400-level courses, MAT401H1/ MAT402H1 recommended", roleRequirement},                                                                                                                                                                  // ASSPE1580
		{"2. EAS209H1, highly recommended to be taken in the second year;", roleRequirement},                                                                                                                                                                                              // ASMAJ1058: timing advice on a required course
		{"1.0 credit from remaining STA300+ level courses not used toward other program requirements in the following list: STA303H1", roleRequirement},                                                                                                                                   // ASMAJ2289 (shortened)
		{"- Czech: 1.0 credit can be fulfilled through an independent study or another course approved in joint consultation with CERES and Slavic undergraduate coordinators.", roleRequirement},                                                                                         // ASMAJ1626: an approval
		{"Students may request to substitute 0.5 credit from requirement 2 with an alternate course by submitting a nomination form including: (1) the relevant pathway", roleRequirement},                                                                                                // ASMIN1255 (shortened): an approval
		{"Second year: PHL271H1, TRN203H1, and TRN204H1 are required courses that students are recommended to take in second year (their first year of registration in the program).", roleRequirement},                                                                                   // ASMAJ1618: "required courses" vetoes the advice
		{"In addition, the Faculty of Arts and Science requires Art History Majors who do not complete FAH275H1, FAH338H1, or ARH440H1 to complete at least one 0.5 credit in Arts and Science courses in Breadth Category 5: The Physical and Mathematical Universes.", roleRequirement}, // ASMAJ0908
		{"- at least 1.0 credit at the 300-/400-level from Group A", roleRequirement},                                                                                                                                                                                                     // ASMAJ1689: a constraint only by its block, under "These 3.0 credits must include:"
		{"5. Additional philosophy courses, to a total of 7.0 credits, including 2.0 PHL credits at the 300+ level, of which one 0.5 credit must be at the 400-level.", roleRequirement},                                                                                                  // ASMAJ0231
		{"(11.0 credits of which at least 4.0 credits must be at the 300+ level, including at least 1.0 credit at the 400-level.)", roleRequirement},                                                                                                                                      // ASSPE0728
		{"Required Courses (12.0 credits, including at least 1.0 credit at the 400 level)", roleRequirement},                                                                                                                                                                              // ASSPE1053
		{"(13.0-13.5 credits, including at least 1.5 credits at the 400-level)", roleRequirement},                                                                                                                                                                                         // ASSPE1687
		{"Students complete 4.0 credits, of which up to 1.0 cognate credit may be taken in PRT.", roleRequirement},                                                                                                                                                                        // ASMIN0623
		{"Applications: Any 5 courses (2.5 credits) from Group A; up to 1.0 credit can be from Group B; up to 0.5 credit can be from Group C", roleRequirement},                                                                                                                           // ASMAJ1252
		{"In addition, CIN230H1 (a Breadth Category 3 course) or 0.5 credit from Breadth Category 5; and at least 3.0 credits from Groups B through G, of which 2.0 credits must be at the 300/400-level", roleRequirement},                                                               // ASMAJ0797: a code lead has no constraint cue of its own

		// Constraints: floors and must-includes.
		{"These 3.0 credits must include:", roleConstraint}, // ASMAJ1689
		{"The choices in 3 must satisfy the requirement for an integrative, inquiry-based activity by including one of the following courses: CSC301H1, CSC302H1, CSC316H1", roleConstraint}, // ASMAJ1689
		{"Note: At least 1.0 credit must be 300/400-level courses.", roleConstraint},                                                                                                         // ASFOC1667A: binding under a note label
		{"At least 1.5 credits must be in FRE or FSL courses taken in the Faculty of Arts & Science.", roleConstraint},                                                                       // ASCER0120
		{"7. Of the courses chosen, at least 2.0 credits must be at the 300+ level, of which a 0.5 credit must be at the 400-level.", roleConstraint},                                        // ASMAJ0115
		{"- 1.0 credit must be at the 300-level", roleConstraint},                                                                                                                            // ASMAJ1446B
		{"All 4.0 credits must be NMC and/or NML courses", roleConstraint},                                                                                                                   // ASMIN1019
		{"At least 0.5 credit must be at the 300+ level", roleConstraint},                                                                                                                    // ASMAJ1076
		{"Of these 2.0 credits, at least 1.0 must be at the 300-level or above.", roleConstraint},                                                                                            // ASMAJ1228
		{"Of these:", roleConstraint}, // ASMAJ2015
		{"4.0 credits must be 300/400-level (including at least 1.0 credit at the 400-level) of which at least 1.0 credit must be from Group A and another from Group B", roleConstraint}, // ASSPE1707

		// Limits.
		{"- no more than 0.5 credit from CSC311H1/ STA314H1", roleLimit},                                                                                                                                    // ASMAJ1689
		{"- No more than 1.0 credit of 100-level courses may be used to fulfill the program requirements.", roleLimit},                                                                                      // ASMAJ2015
		{"Please note only 0.5 of ECO220Y1 will be accepted towards the program.", roleLimit},                                                                                                               // ASMAJ2015
		{"MAT221H1 may not be used for this requirement.", roleLimit},                                                                                                                                       // ASMAJ2289
		{"Language courses cannot be used to fulfil the requirements of the Minor program.", roleLimit},                                                                                                     // ASMIN1058
		{"Regardless of exemptions, a maximum of 1.0 total program credits may be drawn from courses taught in English.", roleLimit},                                                                        // ASMAJ0525
		{"Up to 1.0 credit can be used from GGR493Y1, based on suitability of placement for this program, and approved by department.", roleLimit},                                                          // ASMAJ2030
		{"Only 1.0 credit from 100-level ENG courses may be counted.", roleLimit},                                                                                                                           // ASMIN1645
		{"4. No more than 2.0 credits taught in English", roleLimit},                                                                                                                                        // ASMAJ1400
		{"Of these 4.0 credits, up to 1.0 credit may be taken from cognate departmental or college offerings: PRT courses at any level", roleRequirement},                                                   // ASMAJ0623: a pool widened with a cap
		{"ENG386H1, ENG387H1, ENG388H1, ENG389H1, ENG394H1, ENG395H1, ENG496H1, ENG497H1, and ENG498H1 may not be counted toward fulfilling the requirements for the Minor program in English.", roleLimit}, // ASMIN1645
		{"At least 0.5 credit, but no more than 1.0 credit from each subgroup:", roleRequirement},                                                                                                           // ASSPE0509: an amount lead; the assembler pulls the cap out

		// Named lists.
		{"- **Group A:** CSC209H1; any 300-/ 400-level CSC course _except_ those listed in Group B", roleListDef}, // ASMAJ1689
		{"**Group C: Environmental Policy & Society (no more than 0.5 credit from Group C)**", roleListDef},       // ASMAJ1076: the label wins over the cap in it
		{"_Group I:_", roleListDef}, // ASMAJ2030
		{"a. Hydrology/GIS/Remote Sensing: GGR273H1, GGR274H1, GGR315H1/ GGR337H1, GGR373H1", roleListDef},                  // ASMAJ2030
		{"The following courses fulfill the Diversity and Identity requirement: POL101H1, POL195H1, POL197H1", roleListDef}, // ASMAJ2015
		{"From Group A: 1.0 credit or DRM220Y1", roleRequirement},                                                           // ASMIN2148: draws on a list
		{"2) All of: LIN228H1, LIN229H1, LIN232H1, LIN241H1, LIN251H1", roleRequirement},                                    // ASMAJ0506: a choice, not a list
		{"Hinduism: RLG311H1, RLG358H1, RLG361H1, RLG362H1, RLG363H1", roleListDef},                                         // ASSPE1525 (shortened)

		// Advisory text.
		{"Students in this program have the option to complete the Arts & Science Internship Program (ASIP) stream.", roleAdvisory},                                                             // ASMAJ2015
		{"c. Consult the Undergraduate Office for advice about choosing among CSC108H1 and CSC148H1, and between CSC165H1 and CSC240H1.", roleAdvisory},                                         // ASMAJ1689
		{"d. We recommend that students take MAT148H1/ MAT158H1 and MAT149H1/ MAT159H1, as they provide the best preparation for upper-year courses in computer science", roleAdvisory},         // ASMAJ1689
		{"- First-year students are encouraged, but not required, to take AST121H1.", roleAdvisory},                                                                                             // ASSPE0271
		{"( MAT223H1/ MAT224H1/ MAT240H1 recommended in first year)", roleAdvisory},                                                                                                             // ASMAJ2289
		{"Refer to the Department website for a full list of courses that meet these requirements.", roleAdvisory},                                                                              // ASMAJ2015
		{"Check with the department for current offerings.", roleAdvisory},                                                                                                                      // ASFOC0652A
		{"- Courses may appear in more than one requirement but may only count once toward program completion", roleAdvisory},                                                                   // ASMAJ2525: the default rule restated
		{"Students must complete CIN105Y1 and CIN201Y1 before taking any fourth-year courses.", roleAdvisory},                                                                                   // ASMAJ0797: timing
		{"- Students can take a maximum of 3.0 credits from Group B per year.", roleAdvisory},                                                                                                   // ASMAJ2148: timing
		{"IRE /IRW courses cannot be used to fulfill enrolment/program requirements in Economics.", roleAdvisory},                                                                               // ASMAJ1536: another program's rule
		{"The Astronomy & Physics Specialist program combines rigorous training in the full spectrum of core physics subfields with their application in astronomy.", roleAdvisory},             // ASSPE0271
		{"Requests must be reviewed and approved by the Computer Science Undergraduate Office and will be evaluated on a case-by-case basis.", roleAdvisory},                                    // ASMAJ1689: procedure
		{"(Note: students must request permission to take a graduate course.)", roleAdvisory},                                                                                                   // ASFOC1689D
		{"Refer to the RC website for CR/NCR details.", roleAdvisory},                                                                                                                           // ASSPE2038
		{"HMB200H1 cannot be used in place of PSY290H1", roleAdvisory},                                                                                                                          // ASMAJ1160
		{"It is however recommended to include at least ESS262H1/ EEB240H1/ JEG100H1 and ESS224H1.", roleAdvisory},                                                                              // ASMIN0509
		{"Checking prerequisites is especially important with respect to ECO courses, many of which require the completion of first year Mathematics courses as pre-requisites.", roleAdvisory}, // ASMAJ2660
		{"Note: courses used to satisfy one group requirement cannot be counted toward another group requirement.", roleAdvisory},                                                               // ASMAJ0908: the default rule restated
		{"First year (recommended):", roleAdvisory},                                                                                                                                             // ASMAJ1667
		{"Recommended: 0.5 credit from any HPS 100-level courses", roleAdvisory},                                                                                                                // ASMIN0667
		{"Existing \"PHS\" coded courses remain unchanged.", roleAdvisory},                                                                                                                      // ASMAJ0365

		// Program combinations.
		{"3. _The Neuroscience major cannot be paired with any other Human Biology Program managed major program._", roleExclusion},                                                                            // ASMAJ1472
		{"Students enrolled in this program cannot be simultaneously enrolled in any other CIRHR program.", roleExclusion},                                                                                     // ASMAJ1536
		{"Students cannot combine the Computer Science Major (ASMAJ1689) program with the Computer Science Specialist (ASSPE1689) or the Computer Science Minor (ASMIN1689) for their degree.", roleExclusion}, // ASMAJ1689

		// Transfer credit.
		{"Transfer credits (except for those attained through a University of Toronto exchange program) cannot comprise more than 1.0 credit at the 300-/ 400-level, and cannot be used to satisfy the requirement for an integrative, inquiry-based activity.", roleTransfer}, // ASMAJ1689
		{"- No more than 1.0 transfer credits will be eligible for consideration in the Certificate.", roleTransfer},                                                                                                                                                           // ASCER0338: transfer beats "no more than"
		{"- A maximum of 0.5 transfer credit of an equivalent course from UTSC or UTM may be applied toward the certificate requirements.", roleTransfer},                                                                                                                      // ASCER2400
		{"Transfer credits from AP and IB psychology are not accepted.", roleTransfer},                                                                                                                                                                                         // ASMAJ2575
		{"(Only transfer credits that carry exclusions to CHM136H1 and/or CHM135H1 will be accepted)", roleAdvisory},                                                                                                                                                           // ASMAJ0365: no cap; the course itself is required

		// CR/NCR.
		{"Courses with a CR/NCR status may not be used to satisfy program requirements.", roleCrNcr},                           // ASMAJ1532
		{"- Courses that have been taken as Credit/No Credit cannot be used to fulfill Certificate requirements.", roleCrNcr},  // ASCER0338
		{"ENG100H1, ENG102H1, and courses with a CR/NCR final mark are not accepted toward any English program.", roleCrNcr},   // ASMAJ1647A
		{"- Students may use CR/NCR on the course they use toward the ethics and social responsibility credit.", roleAdvisory}, // ASSPE1165

		// Sharing.
		{"Some courses may contribute to both the Geographic Distribution and Temporal requirements (requirements 1 and 2).", roleOverlap},                           // ASMAJ0652
		{"- Some courses may contribute to both the Canadian Politics and Diversity and Identity requirements (in requirements 4 and 6).", roleOverlap},              // ASMAJ2015
		{"Note: Some offerings of POL336H1; POL357H1; POL497H1 may also count towards requirement 2 of this focus at the department’s discretion.", roleRequirement}, // ASFOC2015C (shortened): membership
		{"Some offerings of HIS357H1, HIS389H1, HIS466H1 and HIS496H1 may also count towards this focus.", roleRequirement},                                          // ASFOC0652A: membership, not sharing

		// From the spec review.
		{"The focus that is available for students to pursue is “Focus in Quantitative Finance – ASFOC2290A\" and requires completion of 3.5 credits", roleAdvisory}, // ASSPE2290: another program
		{"IMM435H1 is capped at 40 students", roleAdvisory}, // ASMAJ1002: an enrolment notice
		{"Students who took GER463Y1 cannot take GER260Y1/ GER261H1/ GER262H1 or GER360H1; students who took GER260Y1/ GER261H1/ GER262H1 or GER360H1 cannot take GER463Y1", roleAdvisory},                                                                                                                                                                      // ASMIN1163: course exclusions
		{"Students with MAT157Y1 cannot take MAT246H1 (it is an exclusion)", roleAdvisory},                                                                                                                                                                                                                                                                      // ASSPE2599
		{"STA302H1/ STA303H1 are not accepted in lieu of ECO375H1", roleAdvisory},                                                                                                                                                                                                                                                                               // ASSPE1478: like "cannot be used in place of"
		{"Students cannot take more than 1.0 credit at the 400-level without written permission from the Undergraduate Program Administrator", roleLimit},                                                                                                                                                                                                       // ASSPE1013: a cap, not a course exclusion
		{"If you have completed any of these suggested related courses, please contact cs.undergrad@utoronto.ca to determine whether it may be appropriate to count a related course towards Focus requirements", roleRequirement},                                                                                                                              // ASFOC1689H: an approval route beats "suggested"
		{"At least 2.0 credits at the 300+ level from Group A, at least 0.5 credit must be at the 400-level", roleRequirement},                                                                                                                                                                                                                                  // ASMAJ0826: an "At least N" lead
		{"At least 3.5 credits from the following list including at least 2.5 with the BMS/JSM/JWB designator:", roleRequirement},                                                                                                                                                                                                                               // ASMAJ1300
		{"At least 2.0 credits from the following list including at least 1.5 with the BMS/JSM/JWB designator:", roleRequirement},                                                                                                                                                                                                                               // ASMIN1300
		{"At least 1.5 credits from the following, of which 0.5 credit must be at the 400-level: MSE302H1, MSE343H1, CHM446H1/ CHM456H1, CHM458H1, MSE403H1, MSE415H1, MSE419H1, MSE430H1, MSE431H1, MSE440H1, MSE459H1, MSE461H1, MSE465H1, MSE540H1", roleRequirement},                                                                                        // ASSPE2424
		{"At least 9.0 FAH credits, and 2.0 credits in one or more languages including at least one German, French, or Italian, though an acceptable alternative modern language such as Dutch or Russian (or Chinese and/or another Asian language) may be acceptable", roleRequirement},                                                                       // ASSPE0908
		{"An additional 2.0 credits in Sociology at the 200 or 300-level (0.5 of which must be taken at the 300-level, i.e., 1.5 credits at the 200 or 300-level and 0.5 at the 300-level)", roleRequirement},                                                                                                                                                   // ASMAJ1013
		{"An additional 3.0 credits in Sociology at the 200 or 300-level (1.0 of which must be taken at the 300-level, i.e., 2.0 credits at the 200 or 300-level and 1.0 at the 300-level)", roleRequirement},                                                                                                                                                   // ASSPE1013
		{"Additional 2.5 credits, of which a maximum 1.0 credit may be environmental courses offered by academic units other than the School of the Environment (from Group A below)", roleRequirement},                                                                                                                                                         // ASMIN1254
		{"Additional credits from 300/400-level CHM/ MAT/ another science, including at least three of CHM317H1, CHM338H1, CHM343H1, CHM379H1, CHM395Y1/ CHM499Y1/ PHC489Y1, CHM410H1", roleRequirement},                                                                                                                                                        // ASSPE1376
		{"2.0 credits in courses where the primary language of study is not English, including at least 1.0 credit at the 300+ level", roleRequirement},                                                                                                                                                                                                         // ASSPE1023: a relative clause in the lead
		{"Completion Requirements: HIS courses to a total of 4.0 credits, including at least 1.0 credit at the 300- or 400-level", roleRequirement},                                                                                                                                                                                                             // ASMIN0652
		{"1.0 credit must be taken from GER270H1/ GER272H1/ GER290H1/ GER391H1", roleRequirement},                                                                                                                                                                                                                                                               // ASMIN2453: allocates among sibling items
		{"POL222H1 (or one of its exclusions: ECO220Y1/ PSY201H1/ SOC202H1/ STA220H1; only 0.5 credit of ECO220Y1 can count toward this Focus)", roleRequirement},                                                                                                                                                                                               // ASFOC2015A: the cap in brackets is the item's own
		{"In addition, CIN230H1 (a Breadth Category 3 course) or 0.5 credit from Breadth Category 5; and at least 3.0 credits from Groups B through G, of which 2.0 credits must be at the 300/400-level, and only a maximum of 1.5 credits can be from Group G: Cross-Listed", roleRequirement},                                                                // ASMAJ0797
		{"An additional 3.0 credits from any combination of courses listed in the three groups below*, with no more than 1.0 credit from Group C: Environmental Science", roleRequirement},                                                                                                                                                                      // ASMAJ1254
		{"Up to a 0.5 credit from non-HIS courses may be substituted for requirement 2, with permission of the department", roleRequirement},                                                                                                                                                                                                                    // ASFOC0652A: a substitution
		{"Up to 3.0 credits from approved courses involving the ancient Greek and Roman civilizations taught by other departments, notably Art History and Philosophy, may be substituted for the 5.5 credits in CLA courses from Requirement 4", roleRequirement},                                                                                              // ASMAJ0382
		{"Up to 2.0 credits in GRK or LAT may be substituted for the 5.5 credits in CLA courses from Requirement 4", roleRequirement},                                                                                                                                                                                                                           // ASMAJ0382
		{"An elective course (maximum of 1.0 credit) dealing with some aspect of Black Canadian Studies not captured in the above list may be accepted with approval of the Canadian Studies Program Director at University College at canadian.studies@utoronto.ca", roleRequirement},                                                                          // ASCER0828
		{"Of these 9.0 credits, up to 2.0 credits at the 200+ level may be taken from the following cognate departmental or college offerings: EUR, GGR, HIS, LAS, POL, SPA", roleRequirement},                                                                                                                                                                  // ASSPE0338: a pool widened with a cap
		{"Up to 1.0 credit may be from other departments", roleRequirement},                                                                                                                                                                                                                                                                                     // ASMAJ0506
		{"Up to 1.0 credit may be chosen from approved courses offered by other departments (see list of Cognate Courses)", roleRequirement},                                                                                                                                                                                                                    // ASMIN2750
		{"Up to 1.0 credit of this requirement can also be completed from HST211H1/ PHS211H1, INS200H1, SDS355H1, SOC218H1, UNI101H1, UNI102H1, UNI103H1, UNI104H1", roleRequirement},                                                                                                                                                                           // ASMIN0728
		{"Students completing the Health & Disease Major (ASMAJ2013) cannot also complete Applied Genetics and Biotechnology Major (ASMAJ1053), the Global Health Major (ASMAJ2575), the Human Biology Major (ASMAJ2035), or the Neuroscience Major (ASMAJ1472)", roleExclusion},                                                                                // ASMAJ2013
		{"Students enrolled in this Minor program cannot be enrolled in any other program in Economics", roleExclusion},                                                                                                                                                                                                                                         // ASMIN1438
		{"Note: Students registered for a German Specialist or Major may not simultaneously be registered for a German Minor program", roleExclusion},                                                                                                                                                                                                           // ASMIN1405
		{"Note: Students enrolled in the German Studies Specialist of Major Program may not simultaneously enroll for a Business German Minor Program", roleExclusion},                                                                                                                                                                                          // ASMIN2453
		{"Students may not use more than 2.0 credits towards their program from JPM400Y1/ PCL298H1/ PCL474Y1/ PCL497H1/ PCL498Y1", roleLimit},                                                                                                                                                                                                                   // ASSPE0605
		{"Students may not use more than 1.0 credit of BCH372Y1/ BCH374Y1/ BCH472Y1/ BCH473Y1 to fulfill program requirements", roleLimit},                                                                                                                                                                                                                      // ASSPE1762
		{"1. Students cannot take more than a 0.5 credit at the 400-level without written permission from the Undergraduate Program Administrator", roleLimit},                                                                                                                                                                                                  // ASMAJ1013
		{"This requirement cannot be met by more than 1.0 credit from any area", roleLimit},                                                                                                                                                                                                                                                                     // ASMAJ0652
		{"STA220H1 and STA255H1 are not accepted for the major", roleLimit},                                                                                                                                                                                                                                                                                     // ASMAJ1478
		{"Courses with lectures taught in English – PRT250H1 or PRT375H1, for instance – are not included for consideration in the Certificate", roleLimit},                                                                                                                                                                                                     // ASCER0338
		{"Students are not permitted to take PCL472Y1 or PCL474Y1", roleLimit},                                                                                                                                                                                                                                                                                  // ASMAJ2082
		{"Note: Visual Studies Minor students in the Faculty of Arts & Science are not permitted to take VIS300H0, VIS403H1, VIS404H1, VIS405H1, VIS430Y1, and VIS431Y1", roleLimit},                                                                                                                                                                            // ASMIN0660
		{"A nomination for a course the student has not yet taken must be submitted at least one semester in advance", roleAdvisory},                                                                                                                                                                                                                            // ASCER1500: nomination timing
		{"Students must also consult with the Department of Pharmacology and Toxicology at least 3 months prior to the intended date of enrolment", roleAdvisory},                                                                                                                                                                                               // ASSPE0605
		{"Students must meet all prerequisites for upper-year math courses, and should choose their sequencing of courses accordingly", roleAdvisory},                                                                                                                                                                                                           // ASSPE2599
		{"Note that not all of the 400-level PHC courses are offered every academic year", roleAdvisory},                                                                                                                                                                                                                                                        // ASSPE1211
		{"Some required courses at upper-year levels may be offered only on the St. George or on the University of Toronto Mississauga campus in any given year", roleAdvisory},                                                                                                                                                                                 // ASSPE2722
		{"These waivers will only be considered for students in the Specialist in Statistical Sciences: Methods and Practice, Focus in Evolutionary Biology", roleAdvisory},                                                                                                                                                                                     // ASSPE2270
		{"Note: Priority is reserved for Drama Majors and Specialists for Group B Studio courses", roleAdvisory},                                                                                                                                                                                                                                                // ASMIN2148
		{"* These courses may be taken in the first year or subsequent years, and are not required for entrance into the specialist program", roleAdvisory},                                                                                                                                                                                                     // ASSPE0482
		{"The course is not required in the actuarial major program but a number of spots are available for actuarial major students", roleAdvisory},                                                                                                                                                                                                            // ASMAJ0608
		{"International students in the actuarial major program are not eligible for work permits for internships", roleAdvisory},                                                                                                                                                                                                                               // ASMAJ0608
		{"The University of Toronto requires that any student who is using a combination of programs to graduate (e.g. two majors, a major and two minors) must complete a total of at least 12.0 distinct credits that are used to satisfy program requirements", roleAdvisory},                                                                                // ASMAJ0608: a degree-level rule
		{"It is possible that a student with a combination of actuarial science major and statistics major may not comply with this 12.0 credit rule (depending on how students select their courses within the statistics major) and may need to take additional courses to satisfy this rule", roleAdvisory},                                                  // ASMAJ0608
		{"Students who move from a different degree program (HBA/HBSc) within Arts & Science to the Rotman Commerce program (BCOM) would be able to count these MGT courses towards their Rotman Commerce degree (conditional on being accepted to the program based on the requirements listed in the Rotman Commerce section of the Calendar)", roleAdvisory}, // ASCER2400
		{"Note: this Focus requires completion of a 0.5 credit or more in addition to the 7.5 credits required for the International Relations Major", roleAdvisory},                                                                                                                                                                                            // ASFOC1469A
		{"A program designed for students looking to add breadth to a program in another, perhaps unrelated discipline", roleAdvisory},                                                                                                                                                                                                                          // ASMIN1423
		{"Note 2: HIS262H1, Canada: A Short Story of Here, is designed for non-History students, this introductory survey fulfills the \"Society and Its Institutions\" breadth requirement", roleAdvisory},                                                                                                                                                     // ASMAJ0652
		{"Please note: each course can only be counted towards one of the requirements below", roleAdvisory},                                                                                                                                                                                                                                                    // ASMAJ0235: the default rule restated
		{"Group C Courses: These courses are offered by other academic units", roleAdvisory},                                                                                                                                                                                                                                                                    // ASMAJ0826
		{"Courses taken in our unit-run Summer Abroad programs will be considered; however, they are not guaranteed to be approved", roleRequirement},                                                                                                                                                                                                           // ASCER0338: an approval route
		{"If you have taken or are planning on taking a course at another campus and would like it to count exceptionally towards your program requirements, please e-mail the Undergraduate Program Assistant at religion.undergrad@utoronto.ca", roleRequirement},                                                                                             // ASSPE0151
		{"Students who have completed criminology/sociolegal type courses in the Faculty of Arts and Science that are not included in Group C may email the Program Coordinator to find out if the courses can be used towards the Group C program requirements", roleRequirement},                                                                              // ASSPE0826
		{"To substitute courses not currently on offer, please contact the Forestry Undergraduate Administrator to discuss course options from other departments", roleRequirement},                                                                                                                                                                             // ASMAJ1100
		{"Of the required gateway courses, POL347H1 and POL386H1 have pre-requisites of a course that deals substantially with politics; students interested in these courses, therefore, should confer with the instructor as to whether their course work includes a pre-requisite for these courses", roleAdvisory},                                          // ASMAJ0135: advice before the "Of the ..." scope
		{"Advanced courses, like CHM151Y1, PHY151H1, ( MAT148H1, MAT149H1/ MAT137Y1) or ( MAT158H1, MAT159H1/ MAT157Y1), can be used as credits for both foundation sciences sections, meaning, half of the credit will count for Compulsory Foundation Sciences, and the other half will count for Additional Foundation Sciences", roleRequirement},           // ASSPE0509: a weight split, not an overlap
		{"No more than 0.5 credit in BCH can be used towards this requirement", roleLimit},                                                                                                                                                                                                                                                                      // ASMAJ1003
		{"Additional courses that may strengthen your background in this program can be taken, but will not count towards your program: ANA300Y1/ ANA301H1/ BCH340H1/ CHM310H1", roleLimit},                                                                                                                                                                     // ASSPE2340: no amount, so the relative clause counts
		{"Students must complete a total of 8.0 credits including a minimum of 2.0 credits from 300+level CRI courses, of these 0.5 CRI credit must be at the 400-level", roleRequirement},                                                                                                                                                                      // ASMAJ0826
		{"(e) 300-level courses may be replaced by 400-level courses (inquire in advance)", roleRequirement},                                                                                                                                                                                                                                                    // ASSPE1400
		{"Note: (a) Students registered for a German Specialist or Major may not simultaneously be registered for a German Minor program", roleExclusion},                                                                                                                                                                                                       // ASMIN1400
		// Earliest cue wins (both constructed).
		{"At least 1.0 credit must be at the 300-level, and no more than 0.5 credit from Group C may count", roleConstraint},
		{"No more than 0.5 credit from Group C may count, and at least 1.0 credit must be at the 300-level", roleLimit},
		// Attributive "recommended"/"suggested" doesn't silence a cap (constructed).
		{"No more than 1.0 credit from the recommended electives may count towards the program", roleLimit},
		{"Only 0.5 credit from the suggested courses may be counted", roleLimit},
		{"Up to 1.0 credit from the list of suggested courses may count", roleLimit},
		{"No more than 1.0 credit from recommended courses may count towards the program", roleLimit},
		// Advice on a substitution route binds (ASMAJ1533); a permission that
		// only comes with advice doesn't (constructed).
		{"Students are advised to discuss potential course substitutions for First Year with the Program Office", roleAdvisory},                        // ASMAJ1533: pins complNotBindingRes "advised to discuss"
		{"Third-year students can take AST424H1 (Introduction to Research) provided they have at least a B+ average in program courses", roleAdvisory}, // ASSPE0271: pins complNotBindingRes "can take CODE ... provided they have"
		{"Students can take either course as their required independent project, or may take JPM400Y1 as an additional elective", roleAdvisory},        // ASSPE2082: pins complNotBindingRes "either course ... additional elective"
		{"A significant part of the course must involve the application of digital technologies to humanities scholarship", roleAdvisory},              // ASMIN1337: pins complNotBindingRes "a significant part of the course must involve"
		{"Students are encouraged to take a graduate course with permission of the instructor", roleAdvisory},
		// A question about counting another course is an approval route
		// (ASMAJ0151, shortened).
		{"If a student wishes a different language to count toward completing these programs they should consult with the Associate Chair, Undergraduate", roleRequirement},
		// A placement after consultation that names no pool is advice (audit
		// pass 2, ASMAJ1626).
		{"After consultation with the Program Coordinator, exceptions may be made in certain cases regarding the sequence of language courses", roleAdvisory}, // ASMAJ1626 (shortened)
		{"Please consult with CEES and Slavic departments to get approval", roleRequirement},
		{"Please contact ug.office@utoronto.ca to get approval", roleRequirement},                                                                                                                               // constructed: an address before the approval
		{"No exceptions can be made", roleAdvisory},                                                                                                                                                             // constructed: no consultation
		{"Please contact the department to get approval to enrol in the course", roleAdvisory},                                                                                                                  // constructed: enrolment procedure                                                                                                                    // ASMAJ1626
		{"Students wishing to apply their Arts & Science Internship Program (ASIP) towards the requirement to complete PCL472Y1/ JPM400Y1 should reach out to undergrad.pharmtox@utoronto.ca", roleRequirement}, // ASSPE2082
		{"Please contact the Undergraduate Administrator at ug.office.env@utoronto.ca to request that these exceptions be posted on Degree Explorer", roleAdvisory},                                             // ASMAJ1076: granted already
		{"For all other programs, consult with the offering department before enrolling in courses", roleAdvisory},                                                                                              // ASMAJ1536
		{"Students who do not wish to attain APGO certification are advised to speak to the Associate Chair, Undergraduate Affairs concerning their course selection", roleAdvisory},                            // ASSPE0509
		{"Students are encouraged to discuss possible options with the Director of the International Relations Program", roleAdvisory},                                                                          // ASCER1469
		// A pointer to an outside list of courses approved for program credit
		// widens the pool: an approval route, though it names a contact.
		{"Please, consult with Associate Chair of Undergraduate Studies or our website for an updated list of courses pre-approved for program credit", roleRequirement},                                                          // ASMAJ0338
		{"A complete and updated list of eligible courses is available at the departmental website https://www.spanport.utoronto.ca/ or through consultation with the Associate Chair of Undergraduate Studies", roleRequirement}, // ASMAJ0623
		{"A complete list of eligible courses is available at the departmental website or by contacting the Associate Chair of Undergraduate Studies", roleRequirement},                                                           // ASSPE0338
		{"Consult our website https://www.spanport.utoronto.ca/ for the updated list of courses pre-approved for program credit or reach out to our Associate Chair of Undergraduate Studies", roleRequirement},                   // ASSPE0623
		{"Students with previous knowledge of Spanish can start taking language courses at the 200- or 300-level, depending on their placement test results, and after consultation with the department", roleAdvisory},           // ASMAJ0623: placement stays advice
		{"*NOTE: This list of eligible courses is not exhaustive and some courses may not be offered every year", roleAdvisory},                                                                                                   // ASMAJ0135: describes a list, points nowhere
		// Advice that reads like a rule: suggestions, notices about entry and
		// prerequisites, course descriptions, past students' experience and
		// timing (design 3.6).
		{"Students should especially consider MAT221H1/ MAT223H1/ MAT240H1 (Linear Algebra), ( MAT235H1, MAT236H1)/ MAT235Y1/ MAT237Y1 (Multivariate Calculus), MAT246H1 (Abstract Mathematics), or even consider doing a Minor program in Mathematics", roleAdvisory}, // ASMAJ1478
		{"Students should note that completing ( ECO231H1 and ECO232H1) limits future enrolment options in ECO courses", roleAdvisory},                                                                                                                                 // ASMAJ1469
		{"Eligibility for all Economics programs is based, in part, on attaining a minimum grade in ECO101H1 and ECO102H1 (or ECO105Y1)", roleAdvisory},                                                                                                                // ASMIN1478
		{"Many 300-level and higher courses in economics require ECO220Y1", roleAdvisory},                                                                                                                                                                              // ASMIN1478
		{"While not a formal part of the Economics Minor program, students MUST meet the specified prerequisites for 300+ economics courses, and should seriously consider taking ECO220Y1", roleAdvisory},                                                             // ASMIN1478
		{"All Major programs in the co-sponsoring life science departments require BIO120H1, however, it is not formally a part of this Specialist Program's requirements", roleAdvisory},                                                                              // ASSPE1868
		{"CSC240H1 is an accelerated and enriched version of CSC165H1 plus CSC236H1, intended for students with a strong mathematical background, or who develop an interest after taking CSC165H1", roleAdvisory},                                                     // ASMIN1689
		{"Former BCB students benefitted from taking CSB352H1, Bioinformatic Methods", roleAdvisory},                                                                                                                                                                   // ASSPE1868
		{"They also found it to be helpful to take BCH441H1/ MGY441H1 prior to BCB410H1 due to BCB410H1's R programming requirement", roleAdvisory},                                                                                                                    // ASSPE1868
		{"Students can also benefit from taking CSB280H1, Data Science for Cell and Systems Biology", roleAdvisory},
		{"BCB students are allowed to enrol in up to 2.5 CSC credits at the 300/400-level", roleAdvisory}, // ASSPE1868                                                                                                                                                    // ASSPE1868
		{"CSC108H1/ CSC148H1/ CSC110Y1 may be taken in First Year", roleAdvisory},
		{"Students seeking an enriched introduction to the theory of computing may choose to enrol in CSC240H1 in their first year", roleAdvisory}, // ASSPE1689                                                                                                                                                                                      // ASSPE2599
		// ... but not a requirement with a timing remark, nor an exception to
		// what is required.
		{"BIO120H1/ BIO130H1 (may be taken in Year 1), BIO220H1 and 1.5 credits from Cognition and the Brain stream courses:", roleRequirement},                                // ASMAJ1446B
		{"Students with a strong background in an object-oriented language such as Python, Java or C++ may omit CSC108H1 and proceed directly with CSC148H1", roleRequirement}, // ASMIN1689
		// A list marker alone says nothing (constructed).
		{"1.", roleAdvisory},
		{"a)", roleAdvisory},
		{"(i)", roleAdvisory},
		{"•", roleAdvisory},
		{"  ii)  ", roleAdvisory},
		{"(2)", roleAdvisory},
		// "accepted"/"included" without an approver is a cap (constructed).
		{"No more than 1.0 credit may be accepted from other universities", roleLimit},
		{"A maximum of 0.5 credit may be included from Group B", roleLimit},
		// Approvals with an approver (real).
		{"Courses from other departments may be accepted on a case-by-case basis", roleRequirement},                                           // ASMAJ0463
		{"Health related Anthropology Special Topics courses may be chosen when offered at the third or fourth year levels", roleRequirement}, // ASFOC1775A: complApprovalRes "may be chosen when offered"
		// Corpus-deciding alternatives, one real sentence each.
		{"The list is meant to be illustrative of the range of cognate topics, but is not necessarily complete", roleAdvisory}, // ASFOC1689D: pins complHeadingVerbRe
		{"Courses must be chosen in a way that satisfies the disciplinary/thematic variety described above", roleRequirement},  // ASMAJ0135: pins complBindingRe "must"
		{"Recommended Sequence of Courses:", roleAdvisory},                                                                     // ASMAJ0135: pins complRecommendRe "recommend"
		{"Students are highly encouraged to take ECO101H1 and ECO102H1 to obtain SOA VEE (Validation by Educational Experience) credit for Economics, and MGT201H1 and ACT349H1 to obtain VEE credit for Accounting and Finance", roleAdvisory}, // ASMAJ0608: pins complRecommendRe "encouraged"
		{"Suggested Related Courses:", roleAdvisory}, // ASFOC1689A: pins complRecommendRe "suggested"
		{"Students interested in actuarial practices should consider taking ACT371H1, ACT372H1, ACT470H1, ACT473H1, ACT475H1", roleAdvisory},                                                                                             // ASMAJ0608: pins complRecommendRe "should consider"
		{"Students with very advanced previous knowledge of German may in consultation with the Department take a combination of other 300 or 400-level GER topics courses (taught in German) to meet the requirement", roleRequirement}, // ASCER1401: pins complFilterRe levels
		{"Note: LIN courses from other campuses are considered courses from other Departments", roleAdvisory},                                                                                                                            // ASMAJ0506: the campus rule restated (complAsideRes)
		{"First year (recommended): PHL100Y1/ PHL101Y1", roleAdvisory},                                                                                                                                                                   // ASMAJ0231: complAsideRes
		{"PSL300H1 and PSL301H1 require MAT 100-level/ PHY 100-level courses", roleAdvisory},                                                                                                                                             // ASSPE0605: a course's prerequisite
		{"BIO220H1 is required for all EEB 300+ level courses (Group A and B) and there are no exceptions", roleAdvisory},                                                                                                                // ASMIN1555
		{"Neuroscience Centric Courses: HMB200H1, HMB300H1, HMB310H1", roleAdvisory},                                                                                                                                                     // ASMAJ1472, shortened
		{"Additional BCH research project courses can be taken as part of the 20.0 credits needed for your degree", roleAdvisory},                                                                                                        // ASSPE1762
		{"Additional BCH research project courses can be taken as part of the 20.0 credits needed for your degree, but cannot be counted towards the program completion", roleLimit},                                                     // ASMAJ1762: a rule, not a remark                                                                                                                         // ASMAJ0506: pins complFilterRe designators
		{"1.0 credit of related courses offered by other academic units may be counted towards the fulfillment of the program", roleRequirement},                                                                                         // ASMAJ0151: pins complAmountRe decimals
		{"First year in program: LCT202Y1", roleRequirement},                                                                                                                                                                             // ASMAJ1023: pins complNotListHeadRe "year"
		{"1) All of: LIN101H1, LIN102H1", roleRequirement},                                                                                                                                                                               // ASMAJ0506: pins complNotListHeadRe choices
		{"Only 0.5 credit of Credit/No Credit coursework or coursework taken outside of FRE or FSL courses taken with the Department of French may be accepted, and must be pre-approved by the Department of French", roleCrNcr},        // ASCER0120: pins complCrNcrRe "Credit/No Credit"
		{"Other introductory language classes may be approved by the Director of the International Relations Program", roleRequirement},                                                                                                  // ASCER1469: pins complApprovalRes "may be approved"
		{"Students may be permitted to take Math graduate courses", roleRequirement},                                                                                                                                                     // ASMAJ1165: pins complApprovalRes "may be permitted"
		{"Courses taken in Summer Abroad programs or at UTM or UTSC will need to be pre-approved", roleRequirement},                                                                                                                      // ASCER1200: pins complApprovalRes "pre-approved"
		{"Students can request substitutions from the IRP Director", roleRequirement},                                                                                                                                                    // ASFOC1469A: pins complApprovalRes "request substitutions"
		{"Students must submit a nomination package to the Undergraduate Associate Director of the School of the Environment including: (1) the relevant cluster; (2) the course code and description; (3) the course syllabus (for completed courses); (4) a brief rationale (500 words max) addressing why this course belongs to the relevant cluster, how it addresses multiple dimensions of sustainability (environmental, social, political, ethical, cultural, or economic), and how it satisfies a gap in the student's Certificate learning objectives", roleAdvisory}, // ASCER1500: pins complProcedureRe "must submit"
		{"ii) Students not placed in FSL220H1 or higher by the Placement test must first complete FSL122H1 before starting this program", roleAdvisory},                                                                                                                                                       // ASMAJ0525: pins complProcedureRe "must ... before"
		{"iii) Students exempted from any of the required FSL courses based on placement test result or Department decision must replace these with the equivalent number of credits of additional FSL or FRE courses", roleRequirement},                                                                      // ASMAJ0525: pins complAdviceVetoRe "must"
		{"No course, however, may be counted twice for the purposes of program completion", roleAdvisory},                                                                                                                                                                                                     // ASMAJ1205: pins complNotBindingRes "counted twice"
		{"Each course can count toward only one requirement, even if listed as options to multiple requisites of the program", roleAdvisory},                                                                                                                                                                  // ASMAJ1165: pins complNotBindingRes "count toward only one requirement"
		{"1. Courses can only count toward one requirement, even if listed as options to multiple requisites of the program", roleAdvisory},                                                                                                                                                                   // ASMAJ1053: pins complNotBindingRes "only count toward one requirement"
		{"Students interested in pursuing a research project in fourth year ( AST425Y1) should consult the undergraduate chair", roleAdvisory},                                                                                                                                                                // ASMAJ1423: pins complAdvisoryRes "consult"
		{"Students with previous knowledge of Portuguese can start taking language courses at the 200- or 300-level, depending on their placement test results, and after consultation with the department", roleAdvisory},                                                                                    // ASMAJ0338: pins complAdvisoryRes "placement test"
		{"Note: There are no specific first-year requirements; however, first-year students are welcome to take CJS200H1, CJS201H1, Hebrew Language courses ( MHB155H1 and MHB156H1), and Yiddish ( GER260Y1 or GER261H1 and GER262H1), which count towards the Jewish Studies Major", roleAdvisory},          // ASMAJ0385: pins complAdvisoryRes "no specific"
		{"Students who complete the Arts & Science Internship Program (ASIP) stream will also meet this requirement", roleRequirement},                                                                                                                                                                        // ASMAJ1689: pins complBindingProseRes "meet this requirement"
		{"This Focus is part of the Cell and Molecular Biology Major and begins with the requirements of First Year and of lines 1-3 of Higher Years of the Cell and Molecular Biology Major Program", roleRequirement},                                                                                       // ASMAJ1003A: pins complBindingProseRes "begins with the requirements of"
		{"Only 0.5 credit of HPS390H1/ MAT390H1/ HPS391H1/ MAT391H1/ APM306Y1/ MAT329Y1 may count toward the major completion requirement of \"further topics\" except for students enrolled in the Education and Society Minor, who may apply 1.0 credit of MAT329Y1 toward their Minor program", roleLimit}, // ASMAJ1165: pins complLimitRes "only N ... may count"
		{"Up to 1.0 credit in identified or pre-approved cognate courses may count (i.e. courses offered by other departments containing a relevant Germanic component and a final paper on a Germanic topic); please inquire at the Department and get approved in advance", roleLimit},                      // ASMAJ1400: pins complLimitRes "up to ... may"
		{"It cannot be counted towards a History program, program requirements or as a pre-requisite for upper-level History courses", roleLimit},                                                                                                                                                             // ASMAJ0652: pins complLimitRes "cannot be counted towards"
		{"Courses taught at UTM or UTSC are not included for consideration", roleLimit},                                                                                                                                                                                                                       // ASCER0338: pins complLimitRes "not included for consideration"
		{"Courses with codes that do not start with FSL or FRE (e.g., FCS185H1 or JFL388H1) are typically conducted partially or entirely in English and are therefore not included for consideration in the Certificate", roleAdvisory},                                                                      // ASCER0120: the designator filter restated (complAsideRes)
		{"should be taken together with GER200Y1, must be taken prior to completing GER300Y1", roleAdvisory},                                                                                                                                                                                                  // ASMAJ1400: a course's timing (complAsideMustRe)
		{"Students taking PCL477H1 must take BCH210H1 prior", roleAdvisory},                                                                                                                                                                                                                                   // ASSPE0605: a course's prerequisite (complAsideMustRe)
		{"*course(s) no longer offered", roleAdvisory}, // ASSPE0652: "(s)" is stripped as a bracket
		{"ANT courses are those offered with the following prefixes: ANT, ARH, INS, JAA, JAL, and JGA", roleAdvisory},
		{"The 1.5 credits can be chosen to reflect the particular academic interests of each student. *If not taken for Requirement 13 above", roleAdvisory},                               // ASSPE0365
		{"if not taken in list 3", roleAdvisory},                                                                                                                                           // ASFOC1687A: the exclusive claims restated
		{"* if not taken for the requirement immediately above", roleAdvisory},                                                                                                             // ASMIN1552
		{"a capstone integrative course", roleAdvisory},                                                                                                                                    // ASSPE1525: a descriptor stripAsides keeps
		{"Students may use the CR/NCR option towards any of the courses listed in Requirement 1", roleAdvisory},                                                                            // ASMAJ1944: a permission with no ban
		{"except for students enrolled in the Education and Society Minor, who may apply 1.0 credit of MAT329Y1 toward their Minor program", roleAdvisory},                                 // ASMAJ1165: another program's rule
		{"Enrolment in any of PCL474Y1 or JPM400Y1 is limited and requires permission from the Department of Pharmacology and Toxicology", roleAdvisory},                                   // ASSPE2573: enrolment
		{"Please note that the 300 or 400-level STA course used to replace STA130H1 cannot be a course that is already being used to meet a program completion requirement", roleAdvisory}, // ASSPE2290: the exclusive claims restated
		{"Only 0.5 credit of HPS390H1/ MAT390H1/ HPS391H1/ MAT391H1/ APM306Y1/ MAT329Y1 may count toward the major completion requirement of \"further topics\" except for students enrolled in the Education and Society Minor, who may apply 1.0 credit of MAT329Y1 toward their Minor program", roleLimit}, // ASMAJ1165: the cap stays one
		{"Note: First-year students should take EAS103H1 and EAS105H1, a first-year language course (unless placed in an upper-year language course by the department), and may take up to 1.0 Society-Culture credit at the 200-level.", roleAdvisory},                                                       // ASMAJ1058
		{"If you completed CSC148H1 before being admitted to the Minor, please note that this course is an exclusion to EEB125H1, ENG286H1, and GGR274H1", roleAdvisory},                                                                                                                                      // ASMIN0160
		{"ACT390H1 is a required course in the actuarial science specialist program", roleAdvisory},                                                                                                                                                                                                           // ASMAJ0608: another program (complVetoDescriptionRe)
		{"This list comprises courses which are required for the specialist as well as suggested courses; together they fulfill groups 1A, 1B, 2A and 2B of the APGO requirements", roleAdvisory},                                                                                                             // ASSPE1253
		{"including PHL100Y1/ PHL101Y1 if taken", roleAdvisory}, // ASMAJ0231
		// Restatements of the rows and advice in one wording (complRestatedRes).
		{"MGT100H1, MGT201H1, MGT230H1, MGT250H1 have considerable overlap with RSM100H1, RSM219H1, RSM230H1 and RSM250H1 taught in Rotman Commerce", roleAdvisory}, // ASCER2400
		{"if not taken in list 1", roleAdvisory}, // ASFOC1689F: the claims rule
		{"The course taken in lieu of STA130H1 cannot be one that is already being used to satisfy Requirement 9 in the completion requirements for Higher Years", roleAdvisory}, // ASMAJ0608
		{"Sub-total = 9.5 or 10.0 credits (depending on options chosen in #7)", roleAdvisory},                                                                                    // ASSPE1006
		{"An Integrative, Inquiry-Based Activity Requirement must be satisfied.", roleRequirement},                                                                               // restatedProgramNotes joins it to the raw of the row that states it
		{"Enrolment in either PCL472Y1 or JPM400Y1 is limited and requires permission from the Department of Pharmacology and Toxicology.", roleAdvisory},                        // ASSPE2082
		{"Health Studies (ASFOC2270A): (2.0 credits) HMB342H1, at least 0.5 credit from HST209H1/ PHS209H1", roleAdvisory},                                                       // ASSPE2270: the focus's own program
		{"CSC236H1 may be taken without CSC165H1 for students who completed CSC111H1", roleAdvisory},                                                                             // ASSPE1687
		{"should be taken together with GER200Y1, must be taken prior to completing GER300Y1", roleAdvisory},                                                                     // ASSPE1400: timing
		{"Students may use the CR/NCR option towards any of the courses listed in Requirement 5.", roleAdvisory},                                                                 // ASSPE1944: nothing bans it
	}
	for _, tt := range tests {
		if got := classifySentence(tt.in); got != tt.want {
			t.Errorf("classifySentence(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestSplitComplSentences(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		// A code before the full stop ends the sentence.
		{"5. POL222H1 (or one of ECO220Y1, PSY201H1, SOC202H1, STA220H1). Please note only 0.5 of ECO220Y1 will be accepted towards the program.",
			[]string{"5. POL222H1 (or one of ECO220Y1, PSY201H1, SOC202H1, STA220H1)", "Please note only 0.5 of ECO220Y1 will be accepted towards the program"}},
		{"1.0 credit from: POL101H1, POL106H1, POL107H1, POL109H1. A maximum of 0.5 credit may be substituted from the following first-year courses: POL191H1",
			[]string{"1.0 credit from: POL101H1, POL106H1, POL107H1, POL109H1", "A maximum of 0.5 credit may be substituted from the following first-year courses: POL191H1"}},
		// Abbreviations and amounts don't end a sentence.
		{"Students in Public Health are strongly encouraged to take a course that includes an experiential learning component (e.g. PHS400Y1/ PHS450Y1). See St. George listings.",
			[]string{"Students in Public Health are strongly encouraged to take a course that includes an experiential learning component (e.g. PHS400Y1/ PHS450Y1)", "See St. George listings"}},
		{"3. 2.0 additional credits in Peace, Conflict and Justice courses (i.e. PCJ prefix classes) at the 300 or 400 level, including at least 0.5 at the 400 level.",
			[]string{"3. 2.0 additional credits in Peace, Conflict and Justice courses (i.e. PCJ prefix classes) at the 300 or 400 level, including at least 0.5 at the 400 level"}},
		// "N.B." is a label, not a sentence.
		{"N.B. Students not placed in FSL220H1 or higher by the Placement test must first complete FSL122H1 before starting this program.",
			[]string{"NB: Students not placed in FSL220H1 or higher by the Placement test must first complete FSL122H1 before starting this program"}},
		// A missing space after the full stop before an amount.
		{"At least 5.0 credits must be NMC and/or NML courses.1.0 credit may be taken from courses offered by other departments.",
			[]string{"At least 5.0 credits must be NMC and/or NML courses", "1.0 credit may be taken from courses offered by other departments"}},
		// An italic note glued to an item.
		{"( CHM135H1 and CHM136H1)/ CHM151Y1 _Transfer credits will be accepted in lieu of the chemistry requirements only if they carry a direct exclusion or equivalency to a pre-approved chemistry course._",
			[]string{"( CHM135H1 and CHM136H1)/ CHM151Y1", "Transfer credits will be accepted in lieu of the chemistry requirements only if they carry a direct exclusion or equivalency to a pre-approved chemistry course"}},
		// A line wholly in brackets.
		{"( STA237H1 and STA238H1 are strongly recommended. MAT221H1 may not be used for this requirement.)",
			[]string{"STA237H1 and STA238H1 are strongly recommended", "MAT221H1 may not be used for this requirement"}},
		// A sentence that starts with an amount; "e.g." before one does not end a sentence.
		{"All courses in the Centre for Jewish Studies Undergraduate Course Handbook count towards this requirement. 1.0 out of the 3.5 credits must be at the 300+ level.",
			[]string{"All courses in the Centre for Jewish Studies Undergraduate Course Handbook count towards this requirement", "1.0 out of the 3.5 credits must be at the 300+ level"}},
		{"Any electives, e.g. 2.0 credits in PHL.", []string{"Any electives, e.g. 2.0 credits in PHL"}}, // constructed
		// A list marker in emphasis.
		{"**3.** Additional HIS, JIH, JHA, JHM, JHN, JMH, JSH or Equivalent courses (listed below) to a total of 7.0 credits. No more than 1.0 credit from the Equivalent courses can contribute to program completion.",
			[]string{"3. Additional HIS, JIH, JHA, JHM, JHN, JMH, JSH or Equivalent courses (listed below) to a total of 7.0 credits", "No more than 1.0 credit from the Equivalent courses can contribute to program completion"}},
		// A full stop after a code before a bracketed code.
		{"MAT133Y1 (70%)/ MAT157Y1; CSC108H1/ CSC148H1. ( MAT130H1/ MAT135H1, MAT136H1)/ MAT157Y1 is strongly recommended.",
			[]string{"MAT133Y1 (70%)/ MAT157Y1; CSC108H1/ CSC148H1", "( MAT130H1/ MAT135H1, MAT136H1)/ MAT157Y1 is strongly recommended"}}, // ASMIN2289 (shortened)
		// A full stop after a bracket before an amount.
		{"4. 4.0 credits chosen from the following list; at least 3.0 credits of which must be Department for the Study of Religion courses (RLG, JAR, JCR, JNR, JPR, JRC, JRN, or JSR designators). 3.0 credits must be taken at the 300-level or higher.",
			[]string{"4. 4.0 credits chosen from the following list; at least 3.0 credits of which must be Department for the Study of Religion courses (RLG, JAR, JCR, JNR, JPR, JRC, JRN, or JSR designators)", "3.0 credits must be taken at the 300-level or higher"}}, // ASMAJ1525
		// A full stop after a code before a level.
		{"_First Year Geography Courses:_ Any 2 courses (1.0 credit) from GGR107H1 (recommended), JEG100H1, GGR101H1, GGR112H1, GGR124H1, GGR172H1, GGR196H1, GGR197H1, GGR198H1, GGR199H1. 200/300-level GGR courses may also be used to meet this requirement. Contact the Undergraduate Administrator to update your program.",
			[]string{"First Year Geography Courses: Any 2 courses (1.0 credit) from GGR107H1 (recommended), JEG100H1, GGR101H1, GGR112H1, GGR124H1, GGR172H1, GGR196H1, GGR197H1, GGR198H1, GGR199H1", "200/300-level GGR courses may also be used to meet this requirement", "Contact the Undergraduate Administrator to update your program"}}, // ASMAJ1252
		// Inline lettered notes.
		{"Note: (a) Students registered for a German Specialist or Major may not simultaneously be registered for a German Minor program; (b) students with prior German knowledge are required to take a placement test and to see the adviser; (c) the exclusion of GER400H1 and GER401H1 does not apply if the student's entry level is post- GER100Y1.",
			[]string{"Note: (a) Students registered for a German Specialist or Major may not simultaneously be registered for a German Minor program", "(b) students with prior German knowledge are required to take a placement test and to see the adviser", "(c) the exclusion of GER400H1 and GER401H1 does not apply if the student's entry level is post- GER100Y1"}}, // ASMIN1400
		{"", nil},
	}
	for _, tt := range tests {
		if got := splitComplSentences(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitComplSentences(%q) =\n%q\nwant\n%q", tt.in, got, tt.want)
		}
	}
}

func TestSentenceRoleString(t *testing.T) {
	if got := roleCrNcr.String(); got != "crNcr" {
		t.Errorf("roleCrNcr.String() = %q", got)
	}
	if got := sentenceRole(99).String(); got != "sentenceRole(99)" {
		t.Errorf("sentenceRole(99).String() = %q", got)
	}
}

var sentenceRoleNames = [...]string{"requirement", "constraint", "limit", "listDef", "advisory", "exclusion", "transfer", "crNcr", "overlap"}

func (r sentenceRole) String() string {
	if r >= 0 && int(r) < len(sentenceRoleNames) {
		return sentenceRoleNames[r]
	}
	return fmt.Sprintf("sentenceRole(%d)", int(r))
}
