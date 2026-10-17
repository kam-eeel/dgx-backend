package utils

import (
	"degree-explorer/types"
)

type Config struct {
	DBPath      string
	DataDir     string
	CoursesDir  string
	ProgramsDir string
	Campuses    []string
	ListenOn    string
	Port        string
}

func LoadConfig() *Config {
	return &Config{
		DBPath:      "app.db",
		DataDir:     "data",
		CoursesDir:  "data/courses",
		ProgramsDir: "data/programs",
		Campuses: []string{
			string(types.UTSG),
			string(types.UTM),
			string(types.UTSC),
		},
		ListenOn: "127.0.0.1",
		Port:     "8080",
	}
}
