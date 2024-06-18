package common

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ethereum/go-ethereum/rlp"
)

// ResolvePath resolves the given path relative to the module root.
func ResolvePath(relPath string) string {
	_, b, _, _ := runtime.Caller(0)
	basepath := filepath.Dir(b)
	return filepath.Join(basepath, "..", relPath)
}

// ReadMockBSCBlobSidecars reads the RLP file and decodes its content.
func ReadMockBSCBlobSidecars() ([]byte, BlobSidecars, error) {
	filePath := ResolvePath("./common/test/bsc_blob_sidecars_len_1.rlp")

	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
		return nil, BlobSidecars{}, err
	}
	defer file.Close()

	rlpData, err := io.ReadAll(file)
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
		return nil, BlobSidecars{}, err
	}

	var sidecars BlobSidecars
	err = rlp.DecodeBytes(rlpData, &sidecars)
	if err != nil {
		log.Fatalf("Failed to decode RLP data: %v", err)
		return nil, BlobSidecars{}, err
	}

	return rlpData, sidecars, nil
}
