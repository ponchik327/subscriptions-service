package config

import (
	"flag"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	HTTP struct {
		Host            string        `yaml:"host"             env:"HTTP_HOST"             env-default:"0.0.0.0"`
		Port            int           `yaml:"port"             env:"HTTP_PORT"             env-default:"8080"`
		ReadTimeout     time.Duration `yaml:"read_timeout"     env:"HTTP_READ_TIMEOUT"     env-default:"10s"`
		WriteTimeout    time.Duration `yaml:"write_timeout"    env:"HTTP_WRITE_TIMEOUT"    env-default:"10s"`
		ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env:"HTTP_SHUTDOWN_TIMEOUT" env-default:"5s"`
	} `yaml:"http"`
	Postgres struct {
		DSN            string `yaml:"dsn"             env:"POSTGRES_DSN"             env-required:"true"`
		MaxConns       int32  `yaml:"max_conns"       env:"POSTGRES_MAX_CONNS"       env-default:"10"`
		MigrationsPath string `yaml:"migrations_path" env:"POSTGRES_MIGRATIONS_PATH" env-default:"file://migrations"`
	} `yaml:"postgres"`
	Log struct {
		Level string `yaml:"level" env:"LOG_LEVEL" env-default:"info"`
	} `yaml:"log"`
}

func Load() (*Config, error) {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	if *configPath == "" {
		if v := os.Getenv("CONFIG_PATH"); v != "" {
			*configPath = v
		} else {
			*configPath = "./config/config.yaml"
		}
	}

	var cfg Config
	if err := cleanenv.ReadConfig(*configPath, &cfg); err != nil {
		if err := cleanenv.ReadEnv(&cfg); err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}
