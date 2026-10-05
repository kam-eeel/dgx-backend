package types

type Campus string

const (
	ONLINE Campus = "online"
	UTSG   Campus = "utsg"
	UTM    Campus = "utm"
	UTSC   Campus = "utsc"
)

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
