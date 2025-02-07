package util

import (
	"log/slog"
	"os"

	"github.com/spf13/viper"
)

const (
	ConfigName = "config"
	ConfigType = "toml"
)

type Provider struct {
	Name    string
	BaseURL string
	APIKey  string
}

type Model struct {
	Alias        string
	Name         string
	Provider     string
	Stream       bool
	SystemPrompt bool
	Temperature  bool
}

type Rejection struct {
	ExceptSessions bool
	Sessions       []int64
	ExceptModels   bool
	Models         []string
}

type Config struct {
	Token                 string     // Telegram bot token
	Admins                []int64    // list of Telegram user IDs
	Users                 []int64    // list of Telegram user IDs
	Groups                []int64    // list of Telegram group IDs
	Providers             []Provider // list of OpenAI API endpoint providers
	Models                []Model
	Blocklist             []Rejection
	DefaultModel          string
	DefaultTemperature    float32
	MaxTokensPerResponse  int
	MaxChatRecordsPerUser int
	UseTelegramify        bool
	Debug                 bool
}

func GetDataDir() string {
	return os.Getenv("ICHIGOD_DATA_DIR")
}

func LoadConfig() (config Config, err error) {
	dataDir := GetDataDir()
	slog.Debug("loading configuration", "data_dir", dataDir)

	viper.SetConfigName(ConfigName)
	viper.SetConfigType(ConfigType)
	if dataDir != "" {
		viper.AddConfigPath(dataDir)
	}
	viper.AddConfigPath("/etc/ichigod/")
	viper.AddConfigPath("$HOME/.config/ichigod/")
	viper.AddConfigPath(".")

	viper.SetDefault("DefaultTemperature", 0.2)
	viper.SetDefault("MaxTokensPerResponse", 4000)
	viper.SetDefault("MaxChatRecordsPerUser", 32)
	viper.SetDefault("UseTelegramify", true)
	viper.SetDefault("Debug", false)

	if err = viper.ReadInConfig(); err != nil {
		slog.Error("failed to read config file", "error", err)
		return
	}
	err = viper.Unmarshal(&config)
	if err != nil {
		slog.Error("failed to unmarshal config", "error", err)
		return
	}

	slog.Debug("configuration loaded",
		"admins", len(config.Admins),
		"users", len(config.Users),
		"groups", len(config.Groups),
		"providers", len(config.Providers),
		"models", len(config.Models),
		"default_model", config.DefaultModel,
		"default_temperature", config.DefaultTemperature)
	return
}

func (c *Config) GetProviderByName(name string) *Provider {
	for _, provider := range c.Providers {
		if provider.Name == name {
			return &provider
		}
	}
	return nil
}
