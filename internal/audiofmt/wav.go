package audiofmt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
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

// LoudnessEnvelope is a precomputed RMS/peak envelope for a PCM16 WAV file.
// Values are normalized into [0, 1] by dividing the absolute sample value by
// 32768 (full-scale for signed 16-bit PCM).
type LoudnessEnvelope struct {
	WindowMS    int
	SampleRate  int
	DurationSec float64
	RMS         []float64
	Peak        []float64
}

// ComputeWavLoudnessEnvelopeFile reads the file at path as a PCM16LE WAV and
// returns an RMS/peak envelope using windows of windowMs milliseconds.
// Returns an error for non-WAV / unsupported formats.
func ComputeWavLoudnessEnvelopeFile(path string, windowMs int) (LoudnessEnvelope, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LoudnessEnvelope{}, err
	}
	return ComputeWavLoudnessEnvelope(data, windowMs)
}

// ComputeWavLoudnessEnvelope computes an RMS/peak envelope from raw WAV bytes.
// Only 16-bit PCM (format=1) WAV is supported (DecodeWavPCM16 enforces this).
func ComputeWavLoudnessEnvelope(data []byte, windowMs int) (LoudnessEnvelope, error) {
	if windowMs <= 0 {
		windowMs = 50
	}
	wav, err := DecodeWavPCM16(data)
	if err != nil {
		return LoudnessEnvelope{}, err
	}
	if wav.SampleRate <= 0 || wav.Channels <= 0 {
		return LoudnessEnvelope{}, errors.New("invalid wav params")
	}

	bytesPerFrame := wav.Channels * 2
	if bytesPerFrame <= 0 || len(wav.PCM)%bytesPerFrame != 0 {
		return LoudnessEnvelope{}, errors.New("pcm length does not align to frames")
	}
	totalFrames := len(wav.PCM) / bytesPerFrame
	durationSec := float64(totalFrames) / float64(wav.SampleRate)

	framesPerWindow := wav.SampleRate * windowMs / 1000
	if framesPerWindow <= 0 {
		framesPerWindow = 1
	}

	// Ceiling division so the trailing partial window is included.
	numWindows := (totalFrames + framesPerWindow - 1) / framesPerWindow
	rms := make([]float64, 0, numWindows)
	peak := make([]float64, 0, numWindows)

	const fullScale = 32768.0

	pcm := wav.PCM
	channels := wav.Channels

	for w := 0; w < numWindows; w++ {
		startFrame := w * framesPerWindow
		endFrame := startFrame + framesPerWindow
		if endFrame > totalFrames {
			endFrame = totalFrames
		}
		var sumSq float64
		var maxAbs int32
		var sampleCount int
		for f := startFrame; f < endFrame; f++ {
			base := f * bytesPerFrame
			for c := 0; c < channels; c++ {
				off := base + c*2
				// little-endian signed 16-bit
				s := int16(binary.LittleEndian.Uint16(pcm[off : off+2]))
				abs := int32(s)
				if abs < 0 {
					abs = -abs
				}
				if abs > maxAbs {
					maxAbs = abs
				}
				v := float64(s)
				sumSq += v * v
				sampleCount++
			}
		}
		if sampleCount == 0 {
			rms = append(rms, 0)
			peak = append(peak, 0)
			continue
		}
		mean := sumSq / float64(sampleCount)
		r := math.Sqrt(mean) / fullScale
		if r < 0 {
			r = 0
		}
		if r > 1 {
			r = 1
		}
		p := float64(maxAbs) / fullScale
		if p < 0 {
			p = 0
		}
		if p > 1 {
			p = 1
		}
		rms = append(rms, r)
		peak = append(peak, p)
	}

	return LoudnessEnvelope{
		WindowMS:    windowMs,
		SampleRate:  wav.SampleRate,
		DurationSec: durationSec,
		RMS:         rms,
		Peak:        peak,
	}, nil
}
