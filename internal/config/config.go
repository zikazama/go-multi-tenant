package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	RabbitMQ RabbitMQConfig `yaml:"rabbitmq"`
	Database DatabaseConfig `yaml:"database"`
	Workers  int            `yaml:"workers"`
}

type RabbitMQConfig struct {
	URL string `yaml:"url"`
}

type DatabaseConfig struct {
	URL string `yaml:"url"`
}

func Load() (*Config, error) {
	cfg := &Config{
		Workers: 3, // Default value
	}

	// Try to load from config.yaml
	if data, err := os.ReadFile("config.yaml"); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	// Override with environment variables if present
	if url := os.Getenv("RABBITMQ_URL"); url != "" {
		cfg.RabbitMQ.URL = url
	}
	if url := os.Getenv("DATABASE_URL"); url != "" {
		cfg.Database.URL = url
	}

	// Set defaults if not configured
	if cfg.RabbitMQ.URL == "" {
		cfg.RabbitMQ.URL = "amqp://guest:guest@localhost:5672/"
	}
	if cfg.Database.URL == "" {
		cfg.Database.URL = "postgres://postgres:postgres@localhost:5432/jatis?sslmode=disable"
	}

	return cfg, nil
}