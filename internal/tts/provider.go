package tts

import "context"

type Provider interface {
	SynthesizeWav(ctx context.Context, text string) ([]byte, error)
}
