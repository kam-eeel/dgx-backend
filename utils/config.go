package utils

import (
	"degree-explorer/types"
)

type Config struct {
	DBPath         string
	SnapshotPath   string
	DataDir        string
	CoursesDir     string
	ProgramsDir    string
	Campuses       []string
	UpdateSchedule string
	ListenOn       string
	Port           string
	TZ             string
}

func LoadConfig() *Config {
	return &Config{
		DBPath:       "app.db",
		SnapshotPath: "dgx.db",
		DataDir:      "data",
		CoursesDir:   "data/courses",
		ProgramsDir:  "data/programs",
		Campuses: []string{
			string(types.UTSG),
			string(types.UTM),
			string(types.UTSC),
		},
		UpdateSchedule: "0 5 * * *",
		ListenOn:       "127.0.0.1",
		Port:           "8080",
		TZ:             "America/Toronto",
	}
}
