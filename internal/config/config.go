package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/creasty/defaults"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

type Config struct {
	Vault     Vault     `mapstructure:"vault"`
	Generator Generator `mapstructure:"generator"`
}

type Vault struct {
	Address        string        `mapstructure:"address"`
	MountPath      string        `mapstructure:"mount_path" default:"cowbird"`
	AuthMethod     string        `mapstructure:"auth_method"`
	RequestTimeout time.Duration `mapstructure:"request_timeout" default:"10s"`
}

// Generator holds the user's last-used password/passphrase generator settings,
// persisted so the generator dialog reopens with the same choices.
type Generator struct {
	Mode             string `mapstructure:"mode" default:"password"` // "password" or "passphrase"
	Length           int    `mapstructure:"length" default:"20"`
	Lower            bool   `mapstructure:"lower" default:"true"`
	Upper            bool   `mapstructure:"upper" default:"true"`
	Digits           bool   `mapstructure:"digits" default:"true"`
	Symbols          bool   `mapstructure:"symbols" default:"true"`
	ExcludeAmbiguous bool   `mapstructure:"exclude_ambiguous" default:"false"`
	Words            int    `mapstructure:"words" default:"5"`
	Separator        string `mapstructure:"separator" default:"-"`
	Capitalize       bool   `mapstructure:"capitalize" default:"true"`
	IncludeNumber    bool   `mapstructure:"include_number" default:"true"`
}

func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	path := filepath.Join(home, ".config/cowbird/config.toml")

	viper.SetConfigFile(path)
	viper.SetEnvPrefix("COWBIRD")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if !os.IsNotExist(err) {
			return Config{}, err
		}
		cfg := Config{}
		if err := defaults.Set(&cfg); err != nil {
			return Config{}, err
		}
		if err := Save(cfg); err != nil {
			return Config{}, err
		}
	}

	// Seed defaults first, then overlay the file. mapstructure leaves fields
	// absent from the source map untouched, so a config written before a new
	// sub-section existed (e.g. [generator]) still receives that section's
	// defaults rather than zero values.
	cfg := Config{}
	if err := defaults.Set(&cfg); err != nil {
		return Config{}, err
	}
	if err := viper.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(cfg Config) error {
	path := viper.ConfigFileUsed()
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		path = filepath.Join(home, ".config/cowbird/config.toml")
		viper.SetConfigFile(path)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	var m map[string]any
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &m,
		TagName: "mapstructure",
	})
	if err != nil {
		return err
	}
	if err := decoder.Decode(cfg); err != nil {
		return err
	}

	if err := viper.MergeConfigMap(m); err != nil {
		return err
	}

	return viper.WriteConfigAs(path)
}
