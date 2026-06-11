package generation

import "errors"

var (
	ErrORTNotConfigured      = errors.New("onnx runtime library path is not configured")
	ErrProviderNotConfigured = errors.New("local generation provider is not configured")
	ErrModelNotFound         = errors.New("model directory is missing required files")
)
