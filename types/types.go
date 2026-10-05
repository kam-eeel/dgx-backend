package types

type Campus string
type ParseResult bool

const (
	ONLINE          Campus      = "online"
	UTSG            Campus      = "utsg"
	UTM             Campus      = "utm"
	UTSC            Campus      = "utsc"
	ParseSuccessful ParseResult = true
	ParseFailure    ParseResult = false
)

var UTSGBreadthReqs = []string{
	"Creative and Cultural Representations (1)",
	"Thought, Belief and Behaviour (2)",
	"Society and its Institutions (3)",
	"Living Things and Their Environment (4)",
	"The Physical and Mathematical Universes (5)",
}

var UTSCBreadthReqs = []string{
	"Arts, Literature and Language",
	"Natural Sciences",
	"History, Philosophy and Cultural Studies",
	"Quantitative Reasoning",
	"Social and Behavioural Sciences",
}

var UTMDistributionReqs = []string{
	"Humanities",
	"Science",
	"Social Science",
}

type Course struct {
	Code            string  `json:"code"`       // ex. CSC148H1
	Designator      string  `json:"designator"` // ex. CSC
	Number          string  `json:"number"`     // ex. 148
	Level           int64   `json:"level"`      // 100-400
	Weight          float64 `json:"weight"`     // 0.5 or 1.0
	Campus          string  `json:"campus"`     // online (0), utsg (1), utsc (3), utm (5)
	Title           string  `json:"title"`
	Prerequisites   *string `json:"prerequisites"`
	Corequisites    *string `json:"corequisites"`
	Exclusions      *string `json:"exclusions"`
	BreadthReq      *string `json:"breadth_req"`
	DistributionReq *string `json:"distribution_req"`
}
