// Package tokenizer implements the Unigram (SentencePiece) tokenizer
// used by Irodori-TTS. It is a Go port of the C++ reference
// (irodori_tts_cpp/src/tokenizer.cpp) with byte_fallback enabled and
// the same UTF-8 segmentation rules.
//
// The model is loaded from a `tokenizer.json` file in the
// HuggingFace tokenizers format (Unigram). The added_tokens table is
// applied after the base vocabulary so that special tokens like <s>,
// <unk> and <PAD|LLM-jp> can be resolved by string even when they
// share ids with normal pieces.
package tokenizer

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"unicode/utf8"
)

// Piece is one entry in the Unigram vocab.
type Piece struct {
	ID    int
	Score float32
}

// Tokenizer is an Unigram tokenizer loaded from a tokenizer.json file.
type Tokenizer struct {
	vocab           map[string]Piece
	tokenToID       map[string]int
	byteTokenToID   map[int]int
	maxPieceBytes   int
	unkID           int
	bosID           int
	padID           int
	addBOS          bool
	byteFallback    bool
	normalizerSpace string // U+2581 lower one-eighth block
}

// FromFile loads an Unigram tokenizer.json file.
func FromFile(path string, addBOS bool) (*Tokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tokenizer: read %s: %w", path, err)
	}
	return FromBytes(data, addBOS)
}

// FromBytes parses tokenizer.json in memory.
func FromBytes(data []byte, addBOS bool) (*Tokenizer, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("tokenizer: parse json: %w", err)
	}
	modelRaw, ok := root["model"]
	if !ok {
		return nil, fmt.Errorf("tokenizer: missing model field")
	}
	var model struct {
		Type         string              `json:"type"`
		UnkID        *int                `json:"unk_id"`
		ByteFallback bool                `json:"byte_fallback"`
		Vocab        [][]json.RawMessage `json:"vocab"`
	}
	if err := json.Unmarshal(modelRaw, &model); err != nil {
		return nil, fmt.Errorf("tokenizer: parse model: %w", err)
	}
	if model.Type != "Unigram" {
		return nil, fmt.Errorf("tokenizer: only Unigram is supported, got %q", model.Type)
	}
	tok := &Tokenizer{
		vocab:           make(map[string]Piece, len(model.Vocab)),
		tokenToID:       make(map[string]int, len(model.Vocab)),
		byteTokenToID:   make(map[int]int, 256),
		unkID:           0,
		bosID:           1,
		padID:           4,
		addBOS:          addBOS,
		byteFallback:    model.ByteFallback,
		normalizerSpace: "\u2581",
	}
	if model.UnkID != nil {
		tok.unkID = *model.UnkID
	}
	for id, entry := range model.Vocab {
		if len(entry) != 2 {
			return nil, fmt.Errorf("tokenizer: vocab[%d] is not [piece, score]", id)
		}
		var piece string
		if err := json.Unmarshal(entry[0], &piece); err != nil {
			return nil, fmt.Errorf("tokenizer: vocab[%d] piece: %w", id, err)
		}
		var score float64
		if err := json.Unmarshal(entry[1], &score); err != nil {
			return nil, fmt.Errorf("tokenizer: vocab[%d] score: %w", id, err)
		}
		tok.vocab[piece] = Piece{ID: id, Score: float32(score)}
		tok.tokenToID[piece] = id
		if len(piece) > tok.maxPieceBytes {
			tok.maxPieceBytes = len(piece)
		}
	}
	// Added tokens overlay
	if addedRaw, ok := root["added_tokens"]; ok {
		var added []struct {
			Content string `json:"content"`
			ID      int    `json:"id"`
		}
		if err := json.Unmarshal(addedRaw, &added); err != nil {
			return nil, fmt.Errorf("tokenizer: parse added_tokens: %w", err)
		}
		for _, a := range added {
			tok.tokenToID[a.Content] = a.ID
		}
	}
	// Byte fallback tables
	for b := 0; b < 256; b++ {
		name := byteTokenName(byte(b))
		if id, ok := tok.tokenToID[name]; ok {
			tok.byteTokenToID[b] = id
		}
	}
	// Resolve <s> / <PAD|LLM-jp>
	if id, ok := tok.tokenToID["<s>"]; ok {
		tok.bosID = id
	}
	if id, ok := tok.tokenToID["<PAD|LLM-jp>"]; ok {
		tok.padID = id
	}
	if tok.addBOS && tok.bosID < 0 {
		return nil, fmt.Errorf("tokenizer: add_bos=true but <s> id is missing")
	}
	if tok.padID < 0 {
		return nil, fmt.Errorf("tokenizer: pad id is missing")
	}
	return tok, nil
}

// BOSID returns the resolved <s> token id.
func (t *Tokenizer) BOSID() int { return t.bosID }

// PadID returns the resolved PAD token id.
func (t *Tokenizer) PadID() int { return t.padID }

// UNKID returns the resolved <unk> token id.
func (t *Tokenizer) UNKID() int { return t.unkID }

// Encode runs SentencePiece Unigram Viterbi encoding with byte
// fallback. It returns the raw token ids (no BOS prefix).
func (t *Tokenizer) Encode(text string) []int {
	norm := t.normalize(text)
	boundaries := utf8Boundaries(norm)
	n := len(boundaries) - 1
	negInf := float32(math.Inf(-1))
	best := make([]float32, n+1)
	prev := make([]int, n+1)
	prevID := make([]int, n+1)
	prevByte := make([]bool, n+1)
	for i := range best {
		best[i] = negInf
	}
	best[0] = 0

	for i := 0; i < n; i++ {
		if math.IsInf(float64(best[i]), -1) {
			continue
		}
		start := boundaries[i]
		// Multi-byte vocab matches.
		for j := i + 1; j <= n; j++ {
			end := boundaries[j]
			if end-start > t.maxPieceBytes {
				break
			}
			piece := norm[start:end]
			if p, ok := t.vocab[piece]; ok {
				score := best[i] + p.Score
				if score > best[j] {
					best[j] = score
					prev[j] = i
					prevID[j] = p.ID
					prevByte[j] = false
				}
			}
		}
		// Byte fallback: each byte of the current UTF-8 char becomes a
		// <0xNN> token.
		if !t.byteFallback {
			continue
		}
		byteEnd := boundaries[i+1]
		for b := start; b < byteEnd; b++ {
			u := int(norm[b])
			fallbackID, has := t.byteTokenToID[u]
			if !has {
				fallbackID = t.unkID
			}
			// Match the C++ reference penalty shape: -100 - 0.001*offset
			score := best[i] - 100.0 - float32(b-start)*0.001
			if score > best[i+1] {
				best[i+1] = score
				prev[i+1] = i
				prevID[i+1] = fallbackID
				prevByte[i+1] = true
			}
		}
	}

	if math.IsInf(float64(best[n]), -1) {
		return []int{t.unkID}
	}
	ids := make([]int, 0, n)
	for cur := n; cur > 0; cur = prev[cur] {
		ids = append(ids, prevID[cur])
		if prev[cur] == cur {
			break
		}
	}
	// Reverse in place
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids
}

// EncodePadded is Encode with a fixed max length. Returns (ids, mask).
// Positions beyond the actual length are filled with the pad id and
// mask 0.
func (t *Tokenizer) EncodePadded(text string, maxLength int) (ids []int64, mask []bool) {
	raw := t.Encode(text)
	if t.addBOS {
		raw = append([]int{t.bosID}, raw...)
	}
	if len(raw) > maxLength {
		raw = raw[:maxLength]
	}
	ids = make([]int64, maxLength)
	mask = make([]bool, maxLength)
	for i := range ids {
		ids[i] = int64(t.padID)
	}
	for i, v := range raw {
		ids[i] = int64(v)
		mask[i] = true
	}
	return ids, mask
}

// normalize replaces ASCII spaces with the U+2581 lower one-eighth
// block and prepends one such block (matching SentencePiece's
// "metaspace" normaliser).
func (t *Tokenizer) normalize(text string) string {
	var b strings.Builder
	b.Grow(len(text) + len(t.normalizerSpace))
	b.WriteString(t.normalizerSpace)
	for _, r := range text {
		if r == ' ' {
			b.WriteString(t.normalizerSpace)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// utf8Boundaries returns the byte offsets of each UTF-8 codepoint
// start in s plus a trailing len(s).
func utf8Boundaries(s string) []int {
	if len(s) == 0 {
		return []int{0}
	}
	out := make([]int, 0, len(s)+1)
	out = append(out, 0)
	for i := 0; i < len(s); {
		_, size := utf8.DecodeRuneInString(s[i:])
		if size <= 0 {
			size = 1
		}
		if i+size > len(s) {
			size = len(s) - i
		}
		i += size
		out = append(out, i)
	}
	return out
}

func byteTokenName(b byte) string {
	const hex = "0123456789ABCDEF"
	out := []byte("<0x00>")
	out[3] = hex[(b>>4)&0xf]
	out[4] = hex[b&0xf]
	return string(out)
}
