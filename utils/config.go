package utils

import (
	"degree-explorer/types"
	"os"
	"sync"
)

type Config struct {
	DBPath         string
	SnapshotPath   string
	TempDir        string
	CoursesDir     string
	ProgramsDir    string
	Campuses       []string
	UpdateSchedule string
	Host           string
	Port           string
	TZ             string
	status         string
	mu             sync.Mutex
}

func (c *Config) SetStatus(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = s
}

func (c *Config) Status() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func LoadConfig() *Config {
	tz, ok := os.LookupEnv("TZ")
	if !ok {
		tz = "America/Toronto"
	}

	return &Config{
		DBPath:       "data/app.db",
		SnapshotPath: "data/dgx.db",
		TempDir:      "tmp",
		CoursesDir:   "tmp/courses",
		ProgramsDir:  "tmp/programs",
		Campuses: []string{
			string(types.UTSG),
			string(types.UTM),
			string(types.UTSC),
		},
		UpdateSchedule: "0 5 * * *",
		Host:           "0.0.0.0",
		Port:           "8080",
		TZ:             tz,
		status:         "starting up",
	}
}
