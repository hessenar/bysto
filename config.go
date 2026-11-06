package bysto

import (
	"fmt"

	"github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
)

const envPrefix = "APP_"

type Config struct {
	RecordsCount        int     `env:"RECORDS_COUNT,required" validate:"gt=0"`
	StorageSectionCount int     `env:"STORAGE_SECTION_COUNT" envDefault:"8" validate:"gt=0"`
	StorageSectionSize  int     `env:"STORAGE_SECTION_SIZE" envDefault:"65536" validate:"gt=0"`
	RebuildThreshold    float64 `env:"REBUILD_THRESHOLD" envDefault:"80" validate:"gt=0,lte=100"`
}

func LoadConfig(prefix string) (Config, error) {
	var cfg Config
	if len(prefix) == 0 {
		prefix = envPrefix
	}
	if err := env.ParseWithOptions(&cfg, env.Options{Prefix: prefix}); err != nil {
		return Config{}, fmt.Errorf("parse config from env: %w", err)
	}
	if err := validator.New().Struct(cfg); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}
	return cfg, nil
}
