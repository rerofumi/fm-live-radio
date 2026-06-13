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

type LocalInferenceConfig struct {
	ORTLibraryPath    string `json:"ortLibraryPath"`
	MaxWorkers        int    `json:"maxWorkers"`
	ExecutionProvider string `json:"executionProvider"`
	DeviceID          int    `json:"deviceId"`
}

type StableAudio3Config struct {
	ModelDir   string  `json:"modelDir"`
	OutputDir  string  `json:"outputDir"`
	PromptBase string  `json:"promptBase"`
	Genre      string  `json:"genre"`
	Seconds    float64 `json:"seconds"`
	Steps      int     `json:"steps"`
	SeedMode   string  `json:"seedMode"`
	FixedSeed  uint32  `json:"fixedSeed"`
	CacheLimit int     `json:"cacheLimit"`
}

type IrodoriConfig struct {
	ModelDir      string  `json:"modelDir"`
	NarratorDir   string  `json:"narratorDir"`
	RefWAV        string  `json:"refWav"`
	Seconds       float64 `json:"seconds"`
	NumSteps      int     `json:"numSteps"`
	SeedMode      string  `json:"seedMode"`
	FixedSeed     uint32  `json:"fixedSeed"`
	CfgText       float64 `json:"cfgText"`
	CfgCaption    float64 `json:"cfgCaption"`
	CfgSpeaker    float64 `json:"cfgSpeaker"`
	DurationScale float64 `json:"durationScale"`
}

type AppConfig struct {
	RSSUrls []string `json:"rssUrls"`

	BGMVolume  float64 `json:"bgmVolume"`
	TalkVolume float64 `json:"talkVolume"`

	Talk           TalkConfig           `json:"talk"`
	LLM            LLMConfig            `json:"llm"`
	LocalInference LocalInferenceConfig `json:"localInference"`
	StableAudio3   StableAudio3Config   `json:"stableAudio3"`
	Irodori        IrodoriConfig        `json:"irodori"`
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
	Provider   string `json:"provider,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	Seed       uint32 `json:"seed,omitempty"`
	ModelDir   string `json:"modelDir,omitempty"`
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
}

type SkipRequest struct {
	CurrentKind PlayableKind `json:"currentKind"`
}

type AppStatus struct {
	TalkPrefetching      bool   `json:"talkPrefetching"`
	TalkReady            bool   `json:"talkReady"`
	MusicGenerating      bool   `json:"musicGenerating"`
	MusicReady           bool   `json:"musicReady"`
	LocalGenerationError string `json:"localGenerationError,omitempty"`
}
