package utils

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// GetAppMemoryUsage retreive the allocated heap bytes
func GetAppMemoryUsage() (int, error) {
	bytes, err := getRss()
	if err != nil {
		return 0, err
	}
	return int(bToMb(bytes)), err
}

func getRss() (int64, error) {
	buf, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, err
	}

	fields := strings.Split(string(buf), " ")
	if len(fields) < 2 {
		return 0, errors.New("cannot parse statm")
	}

	rss, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}

	return rss * int64(os.Getpagesize()), err
}

// Convert Byte to Mega-Byte
func bToMb(b int64) int64 {
	return b / 1024 / 1024
}
