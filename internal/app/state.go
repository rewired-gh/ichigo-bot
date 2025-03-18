package app

import (
	"database/sql"
	"log/slog"

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
	DBID    int // only used for DB operations
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
	ID              int64
	Model           string // model alias
	ChatRecords     []ChatRecord
	State           SessionState
	StopChannel     chan struct{}
	ResponseChannel chan string
	AvailableModels mapset.Set[string]
	Temperature     float32
	Prompt          string
}

type Response struct {
	UserID  int64
	Content string
}

type State struct {
	Config            *util.Config
	CachedProviderMap map[string]*openai.Client // map of provider name to provider
	CachedModelMap    map[string]*util.Model    // map of model alias to model
	CachedPromptMap   map[string]string         // map of prompt name to prompt
	SessionMap        map[int64]*Session        // map of user ID to session
	Bot               *botapi.BotAPI            // nullable
	EditThrottler     chan struct{}
	DB                *sql.DB
}

func New(config *util.Config) (state *State) {
	state = &State{
		Config:            config,
		CachedProviderMap: make(map[string]*openai.Client),
		CachedModelMap:    make(map[string]*util.Model),
		CachedPromptMap:   make(map[string]string),
		SessionMap:        make(map[int64]*Session),
		EditThrottler:     util.NewThrottler(2000),
	}

	for _, prompt := range config.Prompts {
		state.CachedPromptMap[prompt.Name] = prompt.Content
	}

	for _, provider := range config.Providers {
		clientConfig := openai.DefaultConfig(provider.APIKey)
		clientConfig.BaseURL = provider.BaseURL
		state.CachedProviderMap[provider.Name] = openai.NewClientWithConfig(clientConfig)
	}

	// Open (or create) the sqlite DB in the data directory.
	state.DB = OpenSessionDB(util.GetDataDir())

	allModelsSet := mapset.NewSet[string]()
	allUsers := append(append(config.Admins, config.Users...), config.Groups...)
	allUsersSet := mapset.NewSet(allUsers...)
	for _, model := range config.Models {
		state.CachedModelMap[model.Alias] = &model
		allModelsSet.Add(model.Alias)
	}

	rejections := make([]FlattenRejection, 0, len(config.Blocklist))
	for _, configRejection := range config.Blocklist {
		rejection := FlattenRejection{}
		sessions := mapset.NewSet(configRejection.Sessions...)
		models := mapset.NewSet(configRejection.Models...)
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
			ID:              user,
			Model:           config.DefaultModel,
			ChatRecords:     make([]ChatRecord, 0, 16),
			State:           StateIdle,
			StopChannel:     make(chan struct{}),
			ResponseChannel: make(chan string),
			AvailableModels: allModelsSet.Clone(),
			Temperature:     config.DefaultTemperature,
			Prompt:          config.DefaultSystemPrompt,
		}

		// Load persisted session (if any).
		stored, err := LoadSession(state.DB, user)
		if err == nil {
			if _, ok := state.CachedModelMap[stored.Model]; ok {
				session.Model = stored.Model
			}
			session.Temperature = stored.Temperature
			session.Prompt = stored.Prompt
			if len(stored.ChatRecords) > 0 {
				session.ChatRecords = stored.ChatRecords
			}
		} else if err == sql.ErrNoRows {
			// No session in DB: create session row with default values.
			slog.Warn("no session found in DB", "user_id", user)
			UpdateSessionMetadata(state.DB, user, session.Model, session.Temperature, session.Prompt)
		} else {
			slog.Error("failed to load session", "user_id", user, "error", err)
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
