// Package wav writes 16-bit signed PCM mono WAV files matching the
// C++ reference (irodori_tts_cpp/src/wav_writer.cpp). Samples are
// expected to lie in [-1, 1] and are clamped before quantisation.
package wav

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// WriteMonoPCM16 writes samples as a 16-bit signed PCM mono WAV file at
// the given path. The file is overwritten if it already exists.
// Parent directories are created.
func WriteMonoPCM16(path string, samples []float32, sampleRate int) error {
	if sampleRate <= 0 {
		return fmt.Errorf("wav: sample rate must be positive, got %d", sampleRate)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("wav: mkdir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("wav: create: %w", err)
	}
	defer f.Close()

	const (
		headerSize     = 44
		bytesPerSample = 2
		bitsPerSample  = 16
		formatPCM      = 1
		numChannels    = 1
	)
	dataBytes := headerSize + len(samples)*bytesPerSample
	blockAlign := numChannels * bytesPerSample
	byteRate := sampleRate * blockAlign
	if uint64(dataBytes) > uint64(0xffffffff) {
		return fmt.Errorf("wav: file too large for RIFF (>4GiB)")
	}

	// RIFF header
	if err := writeStr(f, "RIFF"); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(dataBytes-8)); err != nil {
		return err
	}
	if err := writeStr(f, "WAVE"); err != nil {
		return err
	}
	// fmt chunk
	if err := writeStr(f, "fmt "); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(formatPCM)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(numChannels)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(byteRate)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(blockAlign)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(bitsPerSample)); err != nil {
		return err
	}
	// data chunk
	if err := writeStr(f, "data"); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(len(samples)*bytesPerSample)); err != nil {
		return err
	}
	// PCM samples
	buf := make([]byte, 2)
	for _, v := range samples {
		if v > 1.0 {
			v = 1.0
		} else if v < -1.0 {
			v = -1.0
		}
		var s int16
		if v <= -1.0 {
			s = -0x8000
		} else {
			s = int16(v * 32767.0)
		}
		buf[0] = byte(s & 0xff)
		buf[1] = byte((s >> 8) & 0xff)
		if _, err := f.Write(buf); err != nil {
			return fmt.Errorf("wav: write sample: %w", err)
		}
	}
	return nil
}

func writeStr(f *os.File, s string) error {
	if _, err := f.Write([]byte(s)); err != nil {
		return fmt.Errorf("wav: write %q: %w", s, err)
	}
	return nil
}

// ReadWAV reads a WAV file, converts it to mono float32, and returns the samples and sample rate.
func ReadWAV(path string) ([]float32, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("wav: read file: %w", err)
	}
	if len(data) < 44 {
		return nil, 0, fmt.Errorf("wav: file too short")
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("wav: invalid RIFF/WAVE header")
	}

	var fmtOffset, dataOffset int
	var fmtSize, dataSize int

	offset := 12
	for offset+8 <= len(data) {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		if chunkID == "fmt " {
			fmtOffset = offset + 8
			fmtSize = chunkSize
		} else if chunkID == "data" {
			dataOffset = offset + 8
			dataSize = chunkSize
			break
		}
		offset += 8 + chunkSize
	}

	if fmtOffset == 0 || dataOffset == 0 {
		return nil, 0, fmt.Errorf("wav: missing fmt or data chunks")
	}

	if fmtSize < 16 {
		return nil, 0, fmt.Errorf("wav: invalid fmt chunk size")
	}

	formatTag := binary.LittleEndian.Uint16(data[fmtOffset : fmtOffset+2])
	numChannels := binary.LittleEndian.Uint16(data[fmtOffset+2 : fmtOffset+4])
	sampleRate := int(binary.LittleEndian.Uint32(data[fmtOffset+4 : fmtOffset+8]))
	bitsPerSample := binary.LittleEndian.Uint16(data[fmtOffset+14 : fmtOffset+16])

	// formatTag 1 is PCM, 3 is Float
	if formatTag != 1 && formatTag != 3 {
		return nil, 0, fmt.Errorf("wav: unsupported format tag %d (only PCM/Float supported)", formatTag)
	}

	bytesPerSample := int(bitsPerSample) / 8
	if bytesPerSample <= 0 {
		return nil, 0, fmt.Errorf("wav: invalid bits per sample %d", bitsPerSample)
	}

	totalSamples := dataSize / bytesPerSample
	if dataOffset+dataSize > len(data) {
		dataSize = len(data) - dataOffset
		totalSamples = dataSize / bytesPerSample
	}

	rawSamples := make([]float32, totalSamples)
	for i := 0; i < totalSamples; i++ {
		idx := dataOffset + i*bytesPerSample
		var val float32
		if formatTag == 1 {
			if bitsPerSample == 8 {
				val = (float32(data[idx]) - 128.0) / 128.0
			} else if bitsPerSample == 16 {
				s := int16(binary.LittleEndian.Uint16(data[idx : idx+2]))
				val = float32(s) / 32768.0
			} else if bitsPerSample == 24 {
				b0 := data[idx]
				b1 := data[idx+1]
				b2 := data[idx+2]
				s := int32(b0) | (int32(b1) << 8) | (int32(b2) << 16)
				if s&0x800000 != 0 {
					s |= ^0xffffff
				}
				val = float32(s) / 8388608.0
			} else if bitsPerSample == 32 {
				s := int32(binary.LittleEndian.Uint32(data[idx : idx+4]))
				val = float32(s) / 2147483648.0
			} else {
				return nil, 0, fmt.Errorf("wav: unsupported bits per sample %d for PCM", bitsPerSample)
			}
		} else if formatTag == 3 {
			if bitsPerSample == 32 {
				bits := binary.LittleEndian.Uint32(data[idx : idx+4])
				val = math.Float32frombits(bits)
			} else {
				return nil, 0, fmt.Errorf("wav: unsupported bits per sample %d for Float", bitsPerSample)
			}
		}
		rawSamples[i] = val
	}

	var monoSamples []float32
	if numChannels == 1 {
		monoSamples = rawSamples
	} else if numChannels == 2 {
		monoSamples = make([]float32, len(rawSamples)/2)
		for i := 0; i < len(monoSamples); i++ {
			monoSamples[i] = (rawSamples[i*2] + rawSamples[i*2+1]) / 2.0
		}
	} else {
		monoSamples = make([]float32, len(rawSamples)/int(numChannels))
		for i := 0; i < len(monoSamples); i++ {
			monoSamples[i] = rawSamples[i*int(numChannels)]
		}
	}

	return monoSamples, sampleRate, nil
}

// Resample changes the sampling rate of input from fromRate to toRate.
func Resample(input []float32, fromRate, toRate int) []float32 {
	if fromRate == toRate {
		return input
	}
	ratio := float64(fromRate) / float64(toRate)
	outLen := int(float64(len(input)) / ratio)
	output := make([]float32, outLen)
	for i := 0; i < outLen; i++ {
		pos := float64(i) * ratio
		idx := int(pos)
		frac := pos - float64(idx)
		if idx+1 < len(input) {
			output[i] = input[idx]*(1.0-float32(frac)) + input[idx+1]*float32(frac)
		} else if idx < len(input) {
			output[i] = input[idx]
		}
	}
	return output
}
