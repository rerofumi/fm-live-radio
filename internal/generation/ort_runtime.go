package generation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

var (
	initMu          sync.Mutex
	initOnce        sync.Once
	initErr         error
	initialized     bool
	initializedPath string
)

func ResolveORTLibraryPath(cfgPath string) string {
	for _, v := range []string{
		strings.TrimSpace(cfgPath),
		strings.TrimSpace(os.Getenv("FM_RADIO_ORT_LIB")),
		strings.TrimSpace(os.Getenv("IRODORI_ORT_LIB")),
		strings.TrimSpace(os.Getenv("SA3_ORT_LIB")),
	} {
		if v != "" {
			return v
		}
	}
	for _, candidate := range []string{
		filepath.Join("third_party", "onnxruntime", "onnxruntime-win-x64-1.26.0", "lib", "onnxruntime.dll"),
		filepath.Join("onnxruntime.dll"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func Init(libraryPath string) error {
	initMu.Lock()
	defer initMu.Unlock()

	resolved := ResolveORTLibraryPath(libraryPath)
	if resolved == "" {
		return ErrORTNotConfigured
	}
	if initialized {
		if !strings.EqualFold(initializedPath, resolved) {
			return fmt.Errorf("onnx runtime already initialized with a different library path: %s", initializedPath)
		}
		return nil
	}

	initOnce.Do(func() {
		ort.SetSharedLibraryPath(resolved)
		initErr = ort.InitializeEnvironment()
		if initErr == nil {
			initialized = true
			initializedPath = resolved
		}
	})
	return initErr
}

func Shutdown() error {
	initMu.Lock()
	defer initMu.Unlock()
	if !initialized {
		return nil
	}
	initialized = false
	return ort.DestroyEnvironment()
}

type Session = ort.DynamicAdvancedSession

func NewSession(modelPath string, inputNames, outputNames []string) (*Session, error) {
	if err := Init(""); err != nil {
		return nil, fmt.Errorf("ort: initialise: %w", err)
	}
	return ort.NewDynamicAdvancedSession(modelPath, inputNames, outputNames, nil)
}

func NewFloat32Tensor(data []float32, shape []int64) (*ort.Tensor[float32], error) {
	return ort.NewTensor(shape, data)
}

func NewEmptyFloat32Tensor(shape []int64) (*ort.Tensor[float32], error) {
	return ort.NewEmptyTensor[float32](shape)
}

func NewInt64Tensor(data []int64, shape []int64) (*ort.Tensor[int64], error) {
	return ort.NewTensor(shape, data)
}

func NewBoolTensor(data []bool, shape []int64) (*ort.Tensor[bool], error) {
	return ort.NewTensor(shape, data)
}

func NewInt32Tensor(data []int32, shape []int64) (*ort.Tensor[int32], error) {
	return ort.NewTensor(shape, data)
}

func NewEmptyInt32Tensor(shape []int64) (*ort.Tensor[int32], error) {
	return ort.NewEmptyTensor[int32](shape)
}
