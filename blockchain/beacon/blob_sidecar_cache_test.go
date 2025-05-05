package beacon

import (
	"encoding/hex"
	"sync"
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestSidecarCache_AddBlobSidecar(t *testing.T) {
	// Initialize the cache
	cache := NewBlobSidecarCacheManager(1606824023)

	// Create a blob sidecar
	header := &ethpb.SignedBeaconBlockHeader{
		Header: &ethpb.BeaconBlockHeader{
			Slot:       8760639,
			ParentRoot: bytesutil.PadTo([]byte{'P'}, fieldparams.RootLength),
			BodyRoot:   bytesutil.PadTo([]byte{'B'}, fieldparams.RootLength),
			StateRoot:  bytesutil.PadTo([]byte{'S'}, fieldparams.RootLength),
		},
		Signature: bytesutil.PadTo([]byte{'D'}, fieldparams.BLSSignatureLength),
	}
	commitmentInclusionProof := make([][]byte, 17)
	for i := range commitmentInclusionProof {
		commitmentInclusionProof[i] = bytesutil.PadTo([]byte{}, 32)
	}
	blobSidecar := &ethpb.BlobSidecar{
		Index:                    1,
		Blob:                     bytesutil.PadTo([]byte{'C'}, fieldparams.BlobLength),
		KzgCommitment:            bytesutil.PadTo([]byte{'D'}, fieldparams.BLSPubkeyLength),
		KzgProof:                 bytesutil.PadTo([]byte{'E'}, fieldparams.BLSPubkeyLength),
		SignedBlockHeader:        header,
		CommitmentInclusionProof: commitmentInclusionProof,
	}

	// calculate block hash
	blockHash, err := blobSidecar.SignedBlockHeader.Header.HashTreeRoot()
	require.NoError(t, err)
	blockHashStr := hex.EncodeToString(blockHash[:])

	ch := cache.SubscribeToBlobByBlockHash(blockHashStr, 1)
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for blobSidecar := range ch {
			// Check that the blob sidecar was added to the cache
			require.NotNil(t, blobSidecar)
			require.Equal(t, blobSidecar.Index, uint64(1))
		}

	}()
	time.Sleep(1 * time.Millisecond)
	// Add the blob sidecar to the cache
	err = cache.AddBlobSidecar(blobSidecar)
	require.NoError(t, err)
	time.Sleep(1 * time.Millisecond)

	// Unsubscribe from the blob sidecar
	cache.UnsubscribeFromBlobByBlockHash(blockHashStr)
	_, ok := <-ch
	require.False(t, ok)

	wg.Wait()
}

func TestSidecarCache_RaceCondition(t *testing.T) {
	cache := NewBlobSidecarCacheManager(1606824023)

	root := bytesutil.PadTo([]byte("root"), fieldparams.RootLength)

	wg := sync.WaitGroup{}

	wg.Add(2)

	go func() {
		defer wg.Done()

		cache.SubscribeToBlobByBlockHash(hex.EncodeToString(root), 1)
	}()
	go func() {
		defer wg.Done()

		cache.UnsubscribeFromBlobByBlockHash(hex.EncodeToString(root))
	}()

	wg.Wait()
}
