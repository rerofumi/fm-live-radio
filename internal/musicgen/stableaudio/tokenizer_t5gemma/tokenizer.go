package tokenizer_t5gemma

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type AddedToken struct {
	ID      int64  `json:"id"`
	Content string `json:"content"`
}

type HFTokenizer struct {
	Model struct {
		Vocab  map[string]int64 `json:"vocab"`
		Merges [][]string       `json:"merges"`
	} `json:"model"`
	AddedTokens []AddedToken `json:"added_tokens"`
}

type Tokenizer struct {
	vocab      map[string]int64
	addedVocab map[string]int64
	mergeRanks map[string]int // Key: token1 + "\x00" + token2, value: index
	padID      int64
	bosID      int64
	eosID      int64
}

// FromFile loads a HuggingFace BPE tokenizer.json file.
func FromFile(path string) (*Tokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tokenizer: read %s: %w", path, err)
	}
	return FromBytes(data)
}

// FromBytes parses tokenizer.json in memory and prepares the BPE structures.
func FromBytes(data []byte) (*Tokenizer, error) {
	var hf HFTokenizer
	if err := json.Unmarshal(data, &hf); err != nil {
		return nil, fmt.Errorf("tokenizer: parse json: %w", err)
	}

	tok := &Tokenizer{
		vocab:      hf.Model.Vocab,
		addedVocab: make(map[string]int64, len(hf.AddedTokens)),
		mergeRanks: make(map[string]int, len(hf.Model.Merges)),
		padID:      0,
		bosID:      2,
		eosID:      1,
	}

	// Build added vocabulary
	for _, at := range hf.AddedTokens {
		tok.addedVocab[at.Content] = at.ID
	}

	// Build merge ranks
	for idx, merge := range hf.Model.Merges {
		if len(merge) == 2 {
			key := merge[0] + "\x00" + merge[1]
			tok.mergeRanks[key] = idx
		}
	}

	// Verify standard special IDs
	if id, ok := tok.addedVocab["<pad>"]; ok {
		tok.padID = id
	} else if id, ok := tok.vocab["<pad>"]; ok {
		tok.padID = id
	}

	if id, ok := tok.addedVocab["<bos>"]; ok {
		tok.bosID = id
	} else if id, ok := tok.vocab["<bos>"]; ok {
		tok.bosID = id
	}

	if id, ok := tok.addedVocab["<eos>"]; ok {
		tok.eosID = id
	} else if id, ok := tok.vocab["<eos>"]; ok {
		tok.eosID = id
	}

	return tok, nil
}

// Encode tokenizes text into T5Gemma BPE IDs, prepending BOS and appending EOS.
func (t *Tokenizer) Encode(text string) []int64 {
	// 1. Normalize spaces to U+2581 ( SentencePiece block )
	normalized := strings.ReplaceAll(text, " ", "\u2581")

	// 2. Initial token splitting ( runes or byte fallback )
	var tokens []string
	for _, char := range normalized {
		sChar := string(char)
		if _, ok := t.vocab[sChar]; ok {
			tokens = append(tokens, sChar)
		} else {
			// Byte fallback: each UTF-8 byte becomes <0xNN>
			utf8Bytes := []byte(sChar)
			for _, b := range utf8Bytes {
				hexName := fmt.Sprintf("<0x%02X>", b)
				tokens = append(tokens, hexName)
			}
		}
	}

	// 3. BPE Merge Loop
	for {
		bestPairIdx := -1
		bestPairRank := 999999999

		for i := 0; i < len(tokens)-1; i++ {
			key := tokens[i] + "\x00" + tokens[i+1]
			if rank, ok := t.mergeRanks[key]; ok {
				if rank < bestPairRank {
					bestPairRank = rank
					bestPairIdx = i
				}
			}
		}

		if bestPairIdx == -1 {
			break
		}

		// Merge best pair
		p1 := tokens[bestPairIdx]
		p2 := tokens[bestPairIdx+1]
		merged := p1 + p2

		var newTokens []string
		for i := 0; i < len(tokens); {
			if i < len(tokens)-1 && tokens[i] == p1 && tokens[i+1] == p2 {
				newTokens = append(newTokens, merged)
				i += 2
			} else {
				newTokens = append(newTokens, tokens[i])
				i++
			}
		}
		tokens = newTokens
	}

	// 4. Map to IDs ( including BOS and EOS )
	ids := make([]int64, 0, len(tokens)+2)
	ids = append(ids, t.bosID)

	for _, token := range tokens {
		if id, ok := t.vocab[token]; ok {
			ids = append(ids, id)
		} else if id, ok := t.addedVocab[token]; ok {
			ids = append(ids, id)
		} else {
			ids = append(ids, t.vocab["<unk>"])
		}
	}

	ids = append(ids, t.eosID)
	return ids
}

// EncodePadded runs Encode and pads/truncates the sequence to maxLen.
// It returns the padded IDs and the float32 attention mask (1.0 for valid, 0.0 for padding).
func (t *Tokenizer) EncodePadded(text string, maxLen int) ([]int64, []float32) {
	ids := t.Encode(text)
	if len(ids) > maxLen {
		ids = ids[:maxLen]
	}

	paddedIDs := make([]int64, maxLen)
	// Fill with Pad ID
	for i := range paddedIDs {
		paddedIDs[i] = t.padID
	}

	mask := make([]float32, maxLen)
	for i, id := range ids {
		paddedIDs[i] = id
		mask[i] = 1.0
	}

	return paddedIDs, mask
}
