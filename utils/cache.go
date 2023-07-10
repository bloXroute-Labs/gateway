package utils

import (
	"bufio"
	"io"
	"os"
	"path"
)

// UpdateCacheFile - update a cache file
func UpdateCacheFile(dataDir string, fileName string, value []byte) error {
	cacheFileName := path.Join(dataDir, fileName)
	f, err := os.OpenFile(cacheFileName, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	writer := bufio.NewWriter(f)
	_, err = writer.Write(value)
	if err != nil {
		return err
	}

	return writer.Flush()
}

// LoadCacheFile - load a cache file
func LoadCacheFile(dataDir string, fileName string) ([]byte, error) {
	cacheFileName := path.Join(dataDir, fileName)
	f, err := os.Open(cacheFileName)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(bufio.NewReader(f))
}
