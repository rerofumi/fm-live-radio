package audiofmt

import (
	"bytes"
	"encoding/binary"
	"errors"
)

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
