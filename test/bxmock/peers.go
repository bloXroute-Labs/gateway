package bxmock

import (
	"io/ioutil"
	"os"
)

// PeerFilePath is the path at which the tets peer file is stored at
const PeerFilePath = "testpeers"
const basicPeerFile = "node:127.0.0.1:1810:127.0.0.1:United States:ATR:AWS:us-east-1:false\n"

// SetupBasicPeerFile writes an empty peer file for the peer manager to read
func SetupBasicPeerFile() {
	err := ioutil.WriteFile(PeerFilePath, []byte(basicPeerFile), 0644)
	if err != nil {
		panic(err)
	}
}

// CleanupPeerFile deletes the test peer file from disk
func CleanupPeerFile() {
	_ = os.RemoveAll(PeerFilePath)
}
