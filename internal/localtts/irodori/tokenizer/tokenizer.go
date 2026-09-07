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
	Score float64
}

// Tokenizer is an Unigram tokenizer loaded from a tokenizer.json file.
type Tokenizer struct {
	vocab           map[string]Piece
	tokenToID       map[string]int
	addedTokens     map[string]int
	byteTokenToID   map[int]int
	maxPieceBytes   int
	unkID           int
	bosID           int
	padID           int
	addBOS          bool
	byteFallback    bool
	metaspace       string
	prependMeta     bool
	normalizeSpace  bool
	normalizePrefix bool
	prefixIfNotNL   bool
	legacyV3        bool
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
	// The v3 tokenizer applies its metaspace replacement in the normalizer,
	// while v4 applies it in a Metaspace pre-tokenizer with prepend_scheme=never.
	// Read those settings instead of assuming the v3 behavior for every model.
	type metaspaceConfig struct {
		Type          string `json:"type"`
		Replacement   string `json:"replacement"`
		PrependScheme string `json:"prepend_scheme"`
	}
	var pre metaspaceConfig
	if raw, ok := root["pre_tokenizer"]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, &pre); err != nil {
			return nil, fmt.Errorf("tokenizer: parse pre_tokenizer: %w", err)
		}
		if pre.Type != "Metaspace" {
			return nil, fmt.Errorf("tokenizer: unsupported pre_tokenizer %q", pre.Type)
		}
	}
	var norm struct {
		Type        string `json:"type"`
		Normalizers []struct {
			Type    string          `json:"type"`
			Pattern json.RawMessage `json:"pattern"`
			Content string          `json:"content"`
		} `json:"normalizers"`
	}
	normalizerPresent := false
	if raw, ok := root["normalizer"]; ok && string(raw) != "null" {
		normalizerPresent = true
		if err := json.Unmarshal(raw, &norm); err != nil {
			return nil, fmt.Errorf("tokenizer: parse normalizer: %w", err)
		}
		if norm.Type != "Sequence" {
			return nil, fmt.Errorf("tokenizer: unsupported normalizer %q", norm.Type)
		}
	}
	space := pre.Replacement
	if space == "" {
		space = "▁"
	}
	tok := &Tokenizer{
		vocab:          make(map[string]Piece, len(model.Vocab)),
		tokenToID:      make(map[string]int, len(model.Vocab)),
		addedTokens:    make(map[string]int),
		byteTokenToID:  make(map[int]int, 256),
		unkID:          0,
		bosID:          -1,
		padID:          -1,
		addBOS:         addBOS,
		byteFallback:   model.ByteFallback,
		metaspace:      space,
		prependMeta:    pre.PrependScheme == "always" || pre.PrependScheme == "first",
		normalizeSpace: normalizerPresent || pre.Type == "Metaspace",
		legacyV3:       normalizerPresent && pre.Type == "",
	}
	if pre.Type == "Metaspace" && pre.PrependScheme == "always" {
		tok.prependMeta = true
	}
	if pre.Type == "Metaspace" && pre.PrependScheme == "never" {
		tok.prependMeta = false
	}
	for _, n := range norm.Normalizers {
		if n.Type != "Replace" {
			return nil, fmt.Errorf("tokenizer: unsupported normalizer entry %q", n.Type)
		}
		var pattern string
		if err := json.Unmarshal(n.Pattern, &pattern); err != nil {
			var object map[string]string
			if err := json.Unmarshal(n.Pattern, &object); err != nil {
				return nil, fmt.Errorf("tokenizer: parse normalizer pattern: %w", err)
			}
			pattern = object["Regex"]
		}
		if pattern == " " {
			// v3 uses an explicit normalizer replacement for ASCII spaces.
			tok.metaspace = n.Content
		} else if strings.Contains(pattern, "^") {
			tok.normalizePrefix = true
			tok.prefixIfNotNL = strings.Contains(pattern, "(?<!\\n)")
		} else {
			return nil, fmt.Errorf("tokenizer: unsupported normalizer pattern %q", pattern)
		}
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
		tok.vocab[piece] = Piece{ID: id, Score: score}
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
			tok.addedTokens[a.Content] = a.ID
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
	if id, ok := tok.tokenToID["<pad>"]; ok {
		tok.padID = id
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
	if t.legacyV3 {
		return t.encodeLegacyV3(text)
	}
	// AddedToken matching happens before normalizer/pre-tokenizer processing in
	// tokenizers. Split literal added tokens first so <s>/<pad>/chat markers are
	// emitted as one token and do not gain a metaspace prefix.
	parts := t.splitAddedTokens(text)
	ids := make([]int, 0, len(text))
	for _, part := range parts {
		if part.id >= 0 {
			ids = append(ids, part.id)
			continue
		}
		ids = append(ids, t.encodeNormalized(part.text)...)
	}
	return ids
}

// encodeLegacyV3 preserves the pre-WP-2 Unigram implementation for the v3
// model. REQ-02 requires the existing v3 token column to remain unchanged;
// v4's corrected byte fallback and score precision are deliberately scoped
// to the versioned v4 path above.
func (t *Tokenizer) encodeLegacyV3(text string) []int {
	norm := t.normalizeLegacyV3(text)
	boundaries := utf8Boundaries(norm)
	n := len(boundaries) - 1
	negInf := float32(math.Inf(-1))
	best := make([]float32, n+1)
	prev := make([]int, n+1)
	prevID := make([]int, n+1)
	for i := range best {
		best[i] = negInf
	}
	best[0] = 0
	for i := 0; i < n; i++ {
		if math.IsInf(float64(best[i]), -1) {
			continue
		}
		start := boundaries[i]
		for j := i + 1; j <= n; j++ {
			end := boundaries[j]
			if end-start > t.maxPieceBytes {
				break
			}
			if p, ok := t.vocab[norm[start:end]]; ok {
				score := best[i] + float32(p.Score)
				if score > best[j] {
					best[j] = score
					prev[j] = i
					prevID[j] = p.ID
				}
			}
		}
		if !t.byteFallback {
			continue
		}
		byteEnd := boundaries[i+1]
		for b := start; b < byteEnd; b++ {
			fallbackID, has := t.byteTokenToID[int(norm[b])]
			if !has {
				fallbackID = t.unkID
			}
			score := best[i] - 100.0 - float32(b-start)*0.001
			if score > best[i+1] {
				best[i+1] = score
				prev[i+1] = i
				prevID[i+1] = fallbackID
			}
		}
	}
	if math.IsInf(float64(best[n]), -1) {
		return []int{t.unkID}
	}
	ids := make([]int, 0, n)
	for cur := n; cur > 0; cur = prev[cur] {
		ids = append(ids, prevID[cur])
	}
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids
}

func (t *Tokenizer) normalizeLegacyV3(text string) string {
	var b strings.Builder
	b.Grow(len(text) + len(t.metaspace))
	b.WriteString(t.metaspace)
	for _, r := range text {
		if r == ' ' {
			b.WriteString(t.metaspace)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (t *Tokenizer) encodeNormalized(text string) []int {
	norm := t.normalize(text)
	n := len(norm)
	negInf := math.Inf(-1)
	best := make([]float64, n+1)
	prev := make([]int, n+1)
	prevID := make([]int, n+1)
	prevByte := make([]bool, n+1)
	for i := range best {
		best[i] = negInf
	}
	best[0] = 0

	for i := 0; i < n; i++ {
		if math.IsInf(best[i], -1) {
			continue
		}
		// Multi-byte vocab matches only at valid UTF-8 boundaries.
		for j := i + 1; j <= n && j-i <= t.maxPieceBytes; j++ {
			piece := norm[i:j]
			if !utf8.ValidString(piece) {
				continue
			}
			if !utf8.ValidString(norm[:i]) || (j < n && !utf8.ValidString(norm[:j])) {
				break
			}
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
		u := int(norm[i])
		fallbackID, has := t.byteTokenToID[u]
		if !has {
			fallbackID = t.unkID
		}
		// Match the reference penalty shape. Unlike the old implementation,
		// each byte advances the DP by one byte, preserving all UTF-8 bytes.
		score := best[i] - 100.0
		if score > best[i+1] {
			best[i+1] = score
			prev[i+1] = i
			prevID[i+1] = fallbackID
			prevByte[i+1] = true
		}
	}

	if math.IsInf(best[n], -1) {
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

// EncodePaddedChecked is the error-returning form used by callers that need
// the same invalid-length contract as PretrainedTextTokenizer.batch_encode.
func (t *Tokenizer) EncodePaddedChecked(text string, maxLength int) ([]int64, []bool, error) {
	if maxLength <= 0 {
		return nil, nil, fmt.Errorf("tokenizer: max_length must be > 0, got %d", maxLength)
	}
	ids, mask := t.EncodePadded(text, maxLength)
	return ids, mask, nil
}

// normalize replaces ASCII spaces with the U+2581 lower one-eighth
// block and prepends one such block (matching SentencePiece's
// "metaspace" normaliser).
func (t *Tokenizer) normalize(text string) string {
	var b strings.Builder
	b.Grow(len(text) + len(t.metaspace))
	if t.normalizePrefix || t.prependMeta {
		if !t.prefixIfNotNL || len(text) == 0 || text[0] != '\n' {
			b.WriteString(t.metaspace)
		}
	}
	for _, r := range text {
		if t.normalizeSpace && r == ' ' {
			b.WriteString(t.metaspace)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type addedPart struct {
	text string
	id   int
}

func (t *Tokenizer) splitAddedTokens(text string) []addedPart {
	if len(t.addedTokens) == 0 {
		return []addedPart{{text: text, id: -1}}
	}
	parts := make([]addedPart, 0, 2)
	plainStart := 0
	for i := 0; i < len(text); {
		best := ""
		bestID := -1
		for token, id := range t.addedTokens {
			if len(token) > len(best) && strings.HasPrefix(text[i:], token) {
				best, bestID = token, id
			}
		}
		if bestID < 0 {
			_, size := utf8.DecodeRuneInString(text[i:])
			if size <= 0 {
				size = 1
			}
			i += size
			continue
		}
		if i > plainStart {
			parts = append(parts, addedPart{text: text[plainStart:i], id: -1})
		}
		parts = append(parts, addedPart{text: best, id: bestID})
		i += len(best)
		plainStart = i
	}
	if plainStart < len(text) || len(parts) == 0 {
		parts = append(parts, addedPart{text: text[plainStart:], id: -1})
	}
	return parts
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
