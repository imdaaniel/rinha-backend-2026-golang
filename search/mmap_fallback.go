//go:build !linux
// +build !linux

package search

import (
	"fmt"
	"os"
)

func mmapFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty binary index file")
	}
	return data, nil
}
