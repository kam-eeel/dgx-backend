package utils

import "degree-explorer/types"

type Config struct {
	DBPath   string
	DataDir  string
	Campuses []string
}

func LoadConfig() *Config {
	return &Config{
		DBPath:  "app.db",
		DataDir: "data",
		Campuses: []string{
			string(types.UTSG),
			string(types.UTM),
			string(types.UTSC),
		},
	}
}
