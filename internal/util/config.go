package util

import (
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
	Alias    string
	Name     string
	Provider string
}

type Config struct {
	Token                 string     // Telegram bot token
	Admins                []int64    // list of Telegram user IDs
	Users                 []int64    // list of Telegram user IDs
	Providers             []Provider // list of OpenAI API endpoint providers
	Models                []Model
	DefaultModel          string
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

	viper.SetConfigName(ConfigName)
	viper.SetConfigType(ConfigType)
	if dataDir != "" {
		viper.AddConfigPath(dataDir)
	}
	viper.AddConfigPath("/etc/ichigod/")
	viper.AddConfigPath("$HOME/.config/ichigod/")
	viper.AddConfigPath(".")
	if err = viper.ReadInConfig(); err != nil {
		return
	}
	err = viper.Unmarshal(&config)
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
