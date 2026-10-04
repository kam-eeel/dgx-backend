package main

type Config struct {
	DBPath   string
	DataDir  string
	Campuses []string
}

func LoadConfig() *Config {
	return &Config{
		DBPath:   "app.db",
		DataDir:  "data",
		Campuses: []string{"utsg", "utm", "utsc"},
	}
}
