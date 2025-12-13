package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"fm-live-radio/internal/audiofmt"
)

var ErrNotConfigured = errors.New("gemini api key not configured")

type GeminiClient struct {
	APIKey string
	Model  string
	Voice  string

	Client *http.Client
}

type generateContentReq struct {
	Contents []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
	GenerationConfig struct {
		ResponseModalities []string `json:"responseModalities"`
		SpeechConfig       struct {
			VoiceConfig struct {
				PrebuiltVoiceConfig struct {
					VoiceName string `json:"voiceName"`
				} `json:"prebuiltVoiceConfig"`
			} `json:"voiceConfig"`
		} `json:"speechConfig"`
	} `json:"generationConfig"`
}

type generateContentResp struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				InlineData struct {
					MIMEType string `json:"mimeType"`
					Data     string `json:"data"`
				} `json:"inlineData"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (c *GeminiClient) SynthesizeWav(ctx context.Context, text string) ([]byte, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, ErrNotConfigured
	}
	model := strings.TrimSpace(c.Model)
	if model == "" {
		model = "gemini-2.5-flash-preview-tts"
	}
	voice := strings.TrimSpace(c.Voice)
	if voice == "" {
		voice = "Kore"
	}

	hc := c.Client
	if hc == nil {
		// TTS may take a while; keep this reasonably high.
		hc = &http.Client{Timeout: 90 * time.Second}
	}

	var reqBody generateContentReq
	reqBody.Contents = []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}{
		{Parts: []struct {
			Text string `json:"text"`
		}{{Text: text}}},
	}
	reqBody.GenerationConfig.ResponseModalities = []string{"AUDIO"}
	reqBody.GenerationConfig.SpeechConfig.VoiceConfig.PrebuiltVoiceConfig.VoiceName = voice

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := "https://generativelanguage.googleapis.com/v1beta/models/" + model + ":generateContent"
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("x-goog-api-key", strings.TrimSpace(c.APIKey))

	// Simple retry for transient timeouts.
	var hresp *http.Response
	for attempt := 0; attempt < 2; attempt++ {
		hresp, err = hc.Do(hreq)
		if err == nil {
			break
		}
		// If the context is done, don't retry.
		select {
		case <-ctx.Done():
			return nil, err
		default:
		}
		// small backoff
		time.Sleep(time.Duration(500*(attempt+1)) * time.Millisecond)
	}
	if err != nil {
		return nil, err
	}
	defer hresp.Body.Close()

	if hresp.StatusCode < 200 || hresp.StatusCode >= 300 {
		// avoid logging key; just return a generic error w/ limited body
		b, _ := io.ReadAll(io.LimitReader(hresp.Body, 2048))
		return nil, errors.New("gemini tts http error: " + strings.TrimSpace(string(b)))
	}

	var out generateContentResp
	if err := json.NewDecoder(hresp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return nil, errors.New("gemini tts empty response")
	}
	b64 := out.Candidates[0].Content.Parts[0].InlineData.Data
	if strings.TrimSpace(b64) == "" {
		return nil, errors.New("gemini tts missing audio data")
	}
	pcm, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}

	// Docs indicate raw PCM s16le 24kHz mono.
	wav, err := audiofmt.EncodeWavPCM16(pcm, 24000, 1)
	if err != nil {
		return nil, err
	}
	return wav, nil
}
