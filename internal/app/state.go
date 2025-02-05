package app

import (
	mapset "github.com/deckarep/golang-set/v2"
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

type FlattenRejection struct {
	Sessions mapset.Set[int64]
	Models   mapset.Set[string]
}

type Session struct {
	Model           string // model alias
	ChatRecords     []ChatRecord
	State           SessionState
	StopChannel     chan struct{}
	ResponseChannel chan string
	AvailableModels mapset.Set[string]
	Temperature     float32
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

func New(config *util.Config) (state *State) {
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

	allModelsSet := mapset.NewSet[string]()
	allUsers := append(append(config.Admins, config.Users...), config.Groups...)
	allUsersSet := mapset.NewSet[int64](allUsers...)
	for _, model := range config.Models {
		state.CachedModelMap[model.Alias] = &model
		allModelsSet.Add(model.Alias)
	}

	rejections := make([]FlattenRejection, 0, len(config.Blocklist))
	for _, configRejection := range config.Blocklist {
		rejection := FlattenRejection{}
		sessions := mapset.NewSet[int64](configRejection.Sessions...)
		models := mapset.NewSet[string](configRejection.Models...)
		if !configRejection.ExceptSessions {
			rejection.Sessions = sessions
		} else {
			rejection.Sessions = allUsersSet.Difference(sessions)
		}
		if !configRejection.ExceptModels {
			rejection.Models = models
		} else {
			rejection.Models = allModelsSet.Difference(models)
		}
		rejections = append(rejections, rejection)
	}

	for _, user := range allUsers {
		session := &Session{
			Model:           config.DefaultModel,
			ChatRecords:     make([]ChatRecord, 0, 16),
			State:           StateIdle,
			StopChannel:     make(chan struct{}),
			ResponseChannel: make(chan string),
			AvailableModels: allModelsSet.Clone(),
			Temperature:     config.DefaultTemperature,
		}

		state.SessionMap[user] = session

		for _, rejection := range rejections {
			if rejection.Sessions.Contains(user) {
				session.AvailableModels = session.AvailableModels.Difference(rejection.Models)
			}
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
