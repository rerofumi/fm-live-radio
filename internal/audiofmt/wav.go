package audiofmt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

type PCM16WAV struct {
	PCM        []byte
	SampleRate int
	Channels   int
}

// EncodeWavPCM16 wraps raw PCM16LE bytes into a RIFF/WAVE container.
func EncodeWavPCM16(pcm []byte, sampleRate int, channels int) ([]byte, error) {
	if sampleRate <= 0 || channels <= 0 {
		return nil, errors.New("invalid wav params")
	}
	if len(pcm)%2 != 0 {
		return nil, errors.New("pcm length must be even for pcm16")
	}

	bitsPerSample := uint16(16)
	blockAlign := uint16(channels) * (bitsPerSample / 8)
	byteRate := uint32(sampleRate) * uint32(blockAlign)

	dataSize := uint32(len(pcm))
	riffSize := uint32(4 + (8 + 16) + (8 + dataSize))

	buf := bytes.NewBuffer(make([]byte, 0, int(8+riffSize)))

	// RIFF header
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, riffSize)
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16)) // chunk size
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))  // PCM format
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, byteRate)
	_ = binary.Write(buf, binary.LittleEndian, blockAlign)
	_ = binary.Write(buf, binary.LittleEndian, bitsPerSample)

	// data chunk
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, dataSize)
	buf.Write(pcm)

	return buf.Bytes(), nil
}

func DecodeWavPCM16(data []byte) (PCM16WAV, error) {
	if len(data) < 12 {
		return PCM16WAV{}, errors.New("wav too short")
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return PCM16WAV{}, errors.New("invalid wav header")
	}

	var (
		format        uint16
		channels      uint16
		sampleRate    uint32
		bitsPerSample uint16
		pcm           []byte
		haveFmt       bool
		haveData      bool
	)

	for off := 12; off+8 <= len(data); {
		chunkID := string(data[off : off+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		off += 8
		if chunkSize < 0 || off+chunkSize > len(data) {
			return PCM16WAV{}, errors.New("invalid wav chunk size")
		}
		chunk := data[off : off+chunkSize]
		switch chunkID {
		case "fmt ":
			if len(chunk) < 16 {
				return PCM16WAV{}, errors.New("wav fmt chunk too short")
			}
			format = binary.LittleEndian.Uint16(chunk[0:2])
			channels = binary.LittleEndian.Uint16(chunk[2:4])
			sampleRate = binary.LittleEndian.Uint32(chunk[4:8])
			bitsPerSample = binary.LittleEndian.Uint16(chunk[14:16])
			haveFmt = true
		case "data":
			pcm = append([]byte(nil), chunk...)
			haveData = true
		}
		off += chunkSize
		if chunkSize%2 == 1 {
			off++
		}
	}

	if !haveFmt {
		return PCM16WAV{}, errors.New("wav missing fmt chunk")
	}
	if !haveData {
		return PCM16WAV{}, errors.New("wav missing data chunk")
	}
	if format != 1 {
		return PCM16WAV{}, fmt.Errorf("unsupported wav format: %d", format)
	}
	if channels == 0 || sampleRate == 0 {
		return PCM16WAV{}, errors.New("invalid wav params")
	}
	if bitsPerSample != 16 {
		return PCM16WAV{}, fmt.Errorf("unsupported bits per sample: %d", bitsPerSample)
	}
	blockAlign := int(channels) * 2
	if len(pcm)%blockAlign != 0 {
		return PCM16WAV{}, errors.New("pcm length does not align to frames")
	}
	return PCM16WAV{
		PCM:        pcm,
		SampleRate: int(sampleRate),
		Channels:   int(channels),
	}, nil
}

func SilencePCM16(sampleRate, channels int, duration time.Duration) ([]byte, error) {
	if sampleRate <= 0 || channels <= 0 || duration < 0 {
		return nil, errors.New("invalid silence params")
	}
	frames := int(duration.Seconds() * float64(sampleRate))
	return make([]byte, frames*channels*2), nil
}
