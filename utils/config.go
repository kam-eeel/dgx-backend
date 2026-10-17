package utils

import (
	"degree-explorer/types"
	"sync"
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
		status:         "starting up",
	}
}
