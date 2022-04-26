package utils

import (
	"io/ioutil"
	"path"
)

// UpdateCacheFile - update a cache file
func UpdateCacheFile(dataDir string, fileName string, value []byte) error {
	cacheFileName := path.Join(dataDir, fileName)
	return ioutil.WriteFile(cacheFileName, value, 0644)
}

// LoadCacheFile - load a cache file
func LoadCacheFile(dataDir string, fileName string) ([]byte, error) {
	cacheFileName := path.Join(dataDir, fileName)
	return ioutil.ReadFile(cacheFileName)
}
