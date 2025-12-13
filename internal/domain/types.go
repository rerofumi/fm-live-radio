package domain

// NOTE: These structs are exported so Wails can generate TS bindings.

type TalkConfig struct {
	Enabled           bool `json:"enabled"`
	CycleBgmCount     int  `json:"cycleBgmCount"`
	TargetDurationSec int  `json:"targetDurationSec"`
	SilenceGapMinMs   int  `json:"silenceGapMinMs"`
	SilenceGapMaxMs   int  `json:"silenceGapMaxMs"`
}

type LLMConfig struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

type AppConfig struct {
	BGMRootPath   string     `json:"bgmRootPath"`
	SelectedGenre string     `json:"selectedGenre"`
	RSSUrls       []string   `json:"rssUrls"`
	GeminiAPIKey  string     `json:"geminiApiKey"`
	Talk          TalkConfig `json:"talk"`
	LLM           LLMConfig  `json:"llm"`
}

type History struct {
	UsedArticleUrls []string `json:"usedArticleUrls"`
	UpdatedAt       string   `json:"updatedAt"`
}

type PlayableKind string

const (
	PlayableKindBGM     PlayableKind = "bgm"
	PlayableKindTalk    PlayableKind = "talk"
	PlayableKindSilence PlayableKind = "silence"
)

type PlayableSource struct {
	Genre      string `json:"genre,omitempty"`
	FilePath   string `json:"filePath,omitempty"`
	RssURL     string `json:"rssUrl,omitempty"`
	ArticleURL string `json:"articleUrl,omitempty"`
}

type PlayableItem struct {
	ID             string         `json:"id"`
	Kind           PlayableKind   `json:"kind"`
	URL            string         `json:"url,omitempty"`
	MIME           string         `json:"mime,omitempty"`
	Title          string         `json:"title"`
	Artist         string         `json:"artist,omitempty"`
	TopicTitle     string         `json:"topicTitle,omitempty"`
	DurationHintMs int            `json:"durationHintMs,omitempty"`
	Source         PlayableSource `json:"source,omitempty"`
}

type NextItemRequest struct {
	SelectedGenre string `json:"selectedGenre"`
}
