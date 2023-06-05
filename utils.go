package main

import (
	"encoding/json"
	"os"
)

func LoadConfig() (*Config, error) {
	data, err := os.ReadFile("static/data/count.json")
	if err != nil {
		return nil, err
	}

	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func SaveConfig(config *Config) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}

	return os.WriteFile("static/data/count.json", data, 0644)
}
