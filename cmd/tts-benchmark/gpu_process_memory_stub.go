//go:build !windows

package main

import (
	"context"
	"errors"
)

func newWDDMProcessMemoryStream(context.Context, gpuProcessMemoryTarget) (gpuProcessMemoryStream, error) {
	return gpuProcessMemoryStream{}, errors.New("WDDM process memory collector is unavailable on non-Windows")
}

func queryWDDMAdapterLUID(string) (string, error) {
	return "", errors.New("DXGI adapter LUID is unavailable on non-Windows")
}
