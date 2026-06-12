package generation

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
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
	initializedEP   epConfig
	currentEP       = epConfig{Provider: "auto", DeviceID: 0}
	lastWarning     string
)

type epConfig struct {
	Provider string
	DeviceID int
}

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
		filepath.Join("third_party", "onnxruntime-gpu", "onnxruntime-win-x64-gpu-1.26.0", "lib", "onnxruntime.dll"),
		filepath.Join("third_party", "onnxruntime", "onnxruntime-win-x64-1.26.0", "lib", "onnxruntime.dll"),
		filepath.Join("onnxruntime.dll"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func ConfigureExecutionProvider(provider string, deviceID int) error {
	initMu.Lock()
	defer initMu.Unlock()

	next := normalizeEPConfig(provider, deviceID)
	if initialized {
		if next != initializedEP {
			return fmt.Errorf("onnx runtime already initialized with execution provider %q (device %d); restart the app to switch providers", initializedEP.Provider, initializedEP.DeviceID)
		}
		return nil
	}
	currentEP = next
	lastWarning = ""
	return nil
}

func LastWarning() string {
	initMu.Lock()
	defer initMu.Unlock()
	return lastWarning
}

func ClearWarning() {
	initMu.Lock()
	defer initMu.Unlock()
	lastWarning = ""
}

func ResolveEPConfig(provider string, deviceID int) epConfig {
	if envProvider := strings.TrimSpace(os.Getenv("FM_RADIO_ORT_EP")); envProvider != "" {
		provider = envProvider
	}
	if envDeviceID := strings.TrimSpace(os.Getenv("FM_RADIO_ORT_DEVICE_ID")); envDeviceID != "" {
		if parsed, err := strconv.Atoi(envDeviceID); err == nil {
			deviceID = parsed
		}
	}
	return normalizeEPConfig(provider, deviceID)
}

func normalizeEPConfig(provider string, deviceID int) epConfig {
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "", "auto":
		provider = "auto"
	case "cpu", "cuda":
	default:
		provider = "cpu"
	}
	if deviceID < 0 {
		deviceID = 0
	}
	return epConfig{Provider: provider, DeviceID: deviceID}
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
		if initializedEP != currentEP {
			return fmt.Errorf("onnx runtime already initialized with execution provider %q (device %d); restart the app to switch providers", initializedEP.Provider, initializedEP.DeviceID)
		}
		return nil
	}

	initOnce.Do(func() {
		ensureDLLSearchPath(resolved)
		ort.SetSharedLibraryPath(resolved)
		log.Printf("INFO: using ONNX Runtime shared library: %s", resolved)
		initErr = ort.InitializeEnvironment()
		if initErr == nil {
			initialized = true
			initializedPath = resolved
			initializedEP = currentEP
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
	initializedPath = ""
	initializedEP = epConfig{}
	lastWarning = ""
	return ort.DestroyEnvironment()
}

type Session = ort.DynamicAdvancedSession

func NewSession(modelPath string, inputNames, outputNames []string) (*Session, error) {
	if err := Init(""); err != nil {
		return nil, fmt.Errorf("ort: initialise: %w", err)
	}
	opts, err := newSessionOptions()
	if err != nil {
		return nil, err
	}
	if opts != nil {
		defer opts.Destroy()
	}
	return ort.NewDynamicAdvancedSession(modelPath, inputNames, outputNames, opts)
}

func newSessionOptions() (*ort.SessionOptions, error) {
	initMu.Lock()
	cfg := currentEP
	initMu.Unlock()

	switch cfg.Provider {
	case "cpu":
		ClearWarning()
		return nil, nil
	case "cuda":
		ClearWarning()
		return buildCUDAOptions(cfg.DeviceID)
	case "auto":
		opts, err := buildCUDAOptions(cfg.DeviceID)
		if err == nil {
			ClearWarning()
			return opts, nil
		}
		msg := fmt.Sprintf("CUDA unavailable, falling back to CPU: %v", err)
		recordWarning(msg)
		return nil, nil
	default:
		return nil, nil
	}
}

func buildCUDAOptions(deviceID int) (*ort.SessionOptions, error) {
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("create ort session options: %w", err)
	}
	cudaOpts, err := ort.NewCUDAProviderOptions()
	if err != nil {
		_ = opts.Destroy()
		return nil, wrapCUDAError("create cuda provider options", err)
	}
	defer cudaOpts.Destroy()

	if err := cudaOpts.Update(map[string]string{
		"device_id": strconv.Itoa(deviceID),
	}); err != nil {
		_ = opts.Destroy()
		return nil, wrapCUDAError("configure cuda provider options", err)
	}
	if err := opts.AppendExecutionProviderCUDA(cudaOpts); err != nil {
		_ = opts.Destroy()
		return nil, wrapCUDAError("append cuda execution provider", err)
	}
	return opts, nil
}

func wrapCUDAError(stage string, err error) error {
	return fmt.Errorf("%s: %w. Check GPU ONNX Runtime DLL, CUDA 13.2, cuDNN 9.x, and PATH", stage, err)
}

func ensureDLLSearchPath(libraryPath string) {
	dir := filepath.Dir(libraryPath)
	if dir == "" {
		return
	}
	current := os.Getenv("PATH")
	for _, entry := range filepath.SplitList(current) {
		if strings.EqualFold(strings.TrimSpace(entry), dir) {
			return
		}
	}
	if current == "" {
		_ = os.Setenv("PATH", dir)
		return
	}
	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+current)
}

func recordWarning(msg string) {
	initMu.Lock()
	defer initMu.Unlock()
	if msg == lastWarning {
		return
	}
	lastWarning = msg
	log.Printf("WARN: %s", msg)
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
