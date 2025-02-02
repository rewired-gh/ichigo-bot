package app

import (
	botapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rewired-gh/ichigo-bot/internal/util"
	"github.com/sashabaranov/go-openai"
)

type ChatRole int

const (
	RoleUser ChatRole = iota
	RoleBot
)

type ChatRecord struct {
	Role    ChatRole
	Content string
	// TODO: add more fields
}

type SessionState int

const (
	StateIdle SessionState = iota
	StateResponding
)

type Session struct {
	Model           string // model alias
	ChatRecords     []ChatRecord
	State           SessionState
	StopChannel     chan struct{}
	ResponseChannel chan string
}

type Response struct {
	UserID  int64
	Content string
}

type State struct {
	Config            *util.Config
	CachedProviderMap map[string]*openai.Client // map of provider name to provider
	CachedModelMap    map[string]*util.Model    // map of model alias to model
	SessionMap        map[int64]*Session        // map of user ID to session
	Bot               *botapi.BotAPI            // nullable
	EditThrottler     chan struct{}
}

func NewState(config *util.Config) (state *State) {
	state = &State{
		Config:            config,
		CachedProviderMap: make(map[string]*openai.Client),
		CachedModelMap:    make(map[string]*util.Model),
		SessionMap:        make(map[int64]*Session),
		EditThrottler:     util.NewThrottler(3),
	}

	for _, provider := range config.Providers {
		clientConfig := openai.DefaultConfig(provider.APIKey)
		clientConfig.BaseURL = provider.BaseURL
		state.CachedProviderMap[provider.Name] = openai.NewClientWithConfig(clientConfig)
	}

	for _, model := range config.Models {
		state.CachedModelMap[model.Alias] = &model
	}

	allUsers := append(append(config.Admins, config.Users...), config.Groups...)
	for _, user := range allUsers {
		state.SessionMap[user] = &Session{
			Model:           config.DefaultModel,
			ChatRecords:     make([]ChatRecord, 0, 16),
			State:           StateIdle,
			StopChannel:     make(chan struct{}),
			ResponseChannel: make(chan string),
		}
	}

	return
}

func (r *ChatRecord) ToOpenAIChatMessage() openai.ChatCompletionMessage {
	role := openai.ChatMessageRoleAssistant
	if r.Role == RoleUser {
		role = openai.ChatMessageRoleUser
	}
	return openai.ChatCompletionMessage{
		Role:    role,
		Content: r.Content,
	}
}
