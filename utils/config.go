package utils

import (
	"degree-explorer/types"
	"errors"
	"os"
)

type Config struct {
	DBPath      string
	DataDir     string
	CoursesDir  string
	ProgramsDir string
	Campuses    []string
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	if err == nil {
		return info.IsDir()
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	return false
}

func LoadConfig() *Config {
	cfg := &Config{
		DBPath:      "app.db",
		DataDir:     "data",
		CoursesDir:  "data/courses",
		ProgramsDir: "data/programs",
		Campuses: []string{
			string(types.UTSG),
			string(types.UTM),
			string(types.UTSC),
		},
	}

	if !dirExists(cfg.CoursesDir) {
		os.MkdirAll(cfg.CoursesDir, 0755)
	}
	if !dirExists(cfg.ProgramsDir) {
		os.MkdirAll(cfg.ProgramsDir, 0755)
	}

	return cfg
}
