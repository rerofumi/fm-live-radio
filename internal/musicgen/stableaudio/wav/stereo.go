package wav

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// WriteStereoPCM16 writes interleaved int32 samples (assumed to be scaled in [-32768, 32767])
// as a 16-bit signed PCM stereo WAV file at the given path.
func WriteStereoPCM16(path string, samples []int32, sampleRate int) error {
	if sampleRate <= 0 {
		return fmt.Errorf("wav: sample rate must be positive, got %d", sampleRate)
	}
	if len(samples)%2 != 0 {
		return fmt.Errorf("wav: samples length must be even for stereo, got %d", len(samples))
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
		bytesPerSample = 2 // 16-bit
		bitsPerSample  = 16
		formatPCM      = 1
		numChannels    = 2
	)
	numFrames := len(samples) / 2
	dataBytes := numFrames * numChannels * bytesPerSample
	fileSize := headerSize + dataBytes - 8
	blockAlign := numChannels * bytesPerSample
	byteRate := sampleRate * blockAlign

	// RIFF header
	if _, err := f.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(fileSize)); err != nil {
		return err
	}
	if _, err := f.Write([]byte("WAVE")); err != nil {
		return err
	}
	// fmt chunk
	if _, err := f.Write([]byte("fmt ")); err != nil {
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
	if _, err := f.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(dataBytes)); err != nil {
		return err
	}

	// PCM samples
	buf := make([]byte, len(samples)*2)
	for i, v := range samples {
		// Clamp to int16 range
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		s := int16(v)
		buf[i*2] = byte(s & 0xff)
		buf[i*2+1] = byte((s >> 8) & 0xff)
	}
	if _, err := f.Write(buf); err != nil {
		return fmt.Errorf("wav: write samples: %w", err)
	}
	return nil
}
