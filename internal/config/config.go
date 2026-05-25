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
	Argon Argon `mapstructure:"argon"`
	Vault Vault `mapstructure:"vault"`
}

type Argon struct {
	Time    int `mapstructure:"time" default:"3"`
	Memory  int `mapstructure:"memory" default:"65536"`
	Threads int `mapstructure:"threads" default:"4"`
	KeyLen  int `mapstructure:"key_len" default:"32"`
}

type Vault struct {
	Address        string        `mapstructure:"address"`
	MountPath      string        `mapstructure:"mount_path" default:"cowbird"`
	AuthMethod     string        `mapstructure:"auth_method"`
	RequestTimeout time.Duration `mapstructure:"request_timeout" default:"10s"`
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

	cfg := Config{}
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
