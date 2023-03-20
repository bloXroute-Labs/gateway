package beacon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/interfaces"
	types "github.com/prysmaticlabs/prysm/v3/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v3/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v3/runtime/version"
)

const (
	maxReorgLength       = 20
	minValidChainLength  = 10
	defaultMaxSize       = 100
	defaultCleanInterval = 10 * time.Minute
)

// blockRef represents block info used for storing best block
type blockRef struct {
	height uint64 // TODO: rename here and everywhere height to slot incl func definitions
	hash   ethcommon.Hash
}

// String formats blockRef for concise printing
func (b blockRef) String() string {
	return fmt.Sprintf("%v[%v]", b.height, b.hash.TerminalString())
}

type blockRefChain []blockRef

func (bc blockRefChain) head() *blockRef {
	if len(bc) == 0 {
		return &blockRef{}
	}
	return &bc[0]
}

func (bc blockRefChain) tail() *blockRef {
	if len(bc) == 0 {
		return nil
	}
	return &bc[len(bc)-1]
}

// String formats blockRefChain for concise printing
func (bc blockRefChain) String() string {
	return fmt.Sprintf("chainstate(best: %v, oldest: %v)", bc.head(), bc.tail())
}

// Chain represents and stores blockchain state info in memory
type Chain struct {
	chainLock  sync.RWMutex // lock for updating chainstate
	headerLock sync.RWMutex // lock for block headers/heights

	genesisTime uint64

	// if reconciling a fork takes longer than this value, then trim the chain to this length
	maxReorg int

	// if a missing block is preventing updating the chain head, once a valid chain of this length is possible, discard the old chain
	minValidChain int

	// if a missing block too old discard to add it
	ignoreSlotCount int

	heightToBlockHeaders cmap.ConcurrentMap
	blockHashMetadata    cmap.ConcurrentMap
	blockHashToBody      cmap.ConcurrentMap

	chainState blockRefChain

	clock utils.RealClock
}

// BlockSource indicates the origin of a block message in the blockchain
type BlockSource string

// enumerate types of BlockSource
const (
	BSBDN        BlockSource = "BDN"
	BSBlockchain BlockSource = "Blockchain"
)

type blockMetadata struct {
	height     uint64
	sentToBDN  bool
	confirmed  bool
	cnfMsgSent bool
}

type ethBeaconHeader struct {
	Header  *ethpb.SignedBeaconBlockHeader
	hash    ethcommon.Hash
	version int
}

// NewChain returns a new chainstate struct for usage
func NewChain(ctx context.Context, genesisTime uint64, ignoreSlotCount int) *Chain {
	return newChain(ctx, genesisTime, ignoreSlotCount, maxReorgLength, minValidChainLength, defaultCleanInterval, defaultMaxSize)
}

func newChain(ctx context.Context, genesisTime uint64, ignoreSlotCount int, maxReorg, minValidChain int, cleanInterval time.Duration, maxSize int) *Chain {
	c := &Chain{
		chainLock:            sync.RWMutex{},
		headerLock:           sync.RWMutex{},
		genesisTime:          genesisTime,
		heightToBlockHeaders: cmap.New(),
		blockHashMetadata:    cmap.New(),
		blockHashToBody:      cmap.New(),
		chainState:           make([]blockRef, 0),
		maxReorg:             maxReorg,
		minValidChain:        minValidChain,
		ignoreSlotCount:      ignoreSlotCount,
		clock:                utils.RealClock{},
	}
	go c.cleanBlockStorage(ctx, cleanInterval, maxSize)
	return c
}

func (c *Chain) cleanBlockStorage(ctx context.Context, cleanInterval time.Duration, maxSize int) {
	ticker := c.clock.Ticker(cleanInterval)
	for {
		select {
		case <-ticker.Alert():
			c.clean(maxSize)
		case <-ctx.Done():
			return
		}
	}
}

// AddBlock adds the provided block from the source into storage, updating the chainstate if the block comes from a reliable source. AddBlock returns the number of new canonical hashes added to the head if a reorganization happened. TODO: consider computing difficulty in here?
func (c *Chain) AddBlock(b interfaces.ReadOnlySignedBeaconBlock, source BlockSource) (int, error) {
	c.chainLock.Lock()
	defer c.chainLock.Unlock()

	height := uint64(b.Block().Slot())
	h, err := b.Block().HashTreeRoot()
	if err != nil {
		return 0, fmt.Errorf("could not get hash: %v", err)
	}

	blockHash := ethcommon.BytesToHash(h[:])

	parentRoot := b.Block().ParentRoot()
	parentHash := ethcommon.BytesToHash(parentRoot[:])

	// update metadata if block already stored, otherwise update all block info
	if c.HasBlock(blockHash) {
		c.storeBlockMetadata(blockHash, height, source == BSBlockchain, false)
	} else {
		if err := c.storeBlock(blockHash, b, source); err != nil {
			log.Errorf("could not store block %v, %v", blockHash, err)
		}
	}

	// if source is BDN, then no authority to update chainstate and indicate no new heads to return
	if source == BSBDN {
		return 0, nil
	}

	return c.updateChainState(height, blockHash, parentHash), nil
}

// ConfirmBlock marks a block as confirmed by a trustworthy source, updating the chain state if possible and returning the number of new canonical hashes added to the head if an update happened.
func (c *Chain) ConfirmBlock(hash ethcommon.Hash) int {
	c.chainLock.Lock()
	defer c.chainLock.Unlock()

	// update metadata
	bm, ok := c.getBlockMetadata(hash)
	if !ok {
		return 0
	}

	bm.confirmed = true
	c.blockHashMetadata.Set(hash.String(), bm)

	header, ok := c.getBlockHeader(bm.height, hash)
	if !ok {
		return 0
	}

	return c.updateChainState(bm.height, hash, ethcommon.BytesToHash(header.Header.GetHeader().GetParentRoot()))
}

// GetNewHeadsForBDN fetches the newest blocks on the chainstate that have not previously been sent to the BDN. In cases of error, as many entries are still returned along with the error. Entries are returned in descending order.
func (c *Chain) GetNewHeadsForBDN(count int) ([]interfaces.ReadOnlySignedBeaconBlock, error) {
	c.chainLock.RLock()
	defer c.chainLock.RUnlock()

	heads := make([]interfaces.ReadOnlySignedBeaconBlock, 0, count)

	for i := 0; i < count; i++ {
		if len(c.chainState) <= i {
			return heads, errors.New("chain state insufficient length")
		}

		head := c.chainState[i]

		// !ok blocks should never be triggered, as any state cleanup should also cleanup the chain state
		bm, ok := c.getBlockMetadata(head.hash)
		if !ok {
			return heads, fmt.Errorf("inconsistent chainstate: no metadata stored for %v", head.hash)
		}

		// blocks have previously been sent to BDN, ok to stop here
		if bm.sentToBDN {
			break
		}

		header, ok := c.getBlockHeader(head.height, head.hash)
		if !ok {
			return heads, fmt.Errorf("inconsistent chainstate: no header stored for %v", head.hash)
		}

		body, ok := c.getBlockBody(head.hash)
		if !ok {
			return heads, fmt.Errorf("inconsistent chainstate: no body stored for %v", head.hash)
		}

		block, err := newSignedBeaconBlock(header.version, header.Header, body)
		if err != nil {
			return heads, fmt.Errorf("inconsistent chainstate: cannot merge head and body for %v: %v", head.hash, err)
		}

		heads = append(heads, block)
	}

	return heads, nil
}

// ValidateBlock determines if block can potentially be added to the chain
func (c *Chain) ValidateBlock(block interfaces.ReadOnlySignedBeaconBlock) error {
	hash, err := block.Block().HashTreeRoot()
	if err != nil {
		return fmt.Errorf("could not get block hash: %v", err)
	}
	blockSlot := block.Block().Slot()

	if c.HasBlock(hash) && c.HasSentToBDN(hash) && c.HasConfirmedBlock(hash) {
		return ErrAlreadySeen
	}

	currentSlot := currentSlot(c.genesisTime)

	maxSlot := currentSlot + types.Slot(c.ignoreSlotCount)
	if blockSlot > maxSlot {
		return fmt.Errorf("too far in future, current slot: %d, max slot: %d", currentSlot, maxSlot)
	}

	minSlot := currentSlot - types.Slot(c.ignoreSlotCount)
	if blockSlot < minSlot {
		return fmt.Errorf("too old, current slot: %d, min slot: %d", currentSlot, minSlot)
	}

	return nil
}

// HasBlock indicates if block has been stored locally
func (c *Chain) HasBlock(hash ethcommon.Hash) bool {
	return c.hasHeader(hash) && c.hasBody(hash)
}

// HasSentToBDN indicates if the block has been sent to the BDN
func (c *Chain) HasSentToBDN(hash ethcommon.Hash) bool {
	bm, ok := c.getBlockMetadata(hash)
	if !ok {
		return false
	}
	return bm.sentToBDN
}

// HasConfirmedBlock indicates if the block has been confirmed by a reliable source
func (c *Chain) HasConfirmedBlock(hash ethcommon.Hash) bool {
	bm, ok := c.getBlockMetadata(hash)
	if !ok {
		return false
	}
	return bm.confirmed
}

// HasConfirmationSentToBDN indicates if block confirmation has been sent to the BDN
func (c *Chain) HasConfirmationSentToBDN(hash ethcommon.Hash) bool {
	c.chainLock.Lock()
	defer c.chainLock.Unlock()

	bm, ok := c.getBlockMetadata(hash)
	if !ok {
		return false
	}

	return bm.cnfMsgSent
}

// MarkSentToBDN marks a block as having been sent to the BDN, so it does not need to be sent again in the future
func (c *Chain) MarkSentToBDN(hash ethcommon.Hash) {
	c.chainLock.Lock()
	defer c.chainLock.Unlock()

	bm, ok := c.getBlockMetadata(hash)
	if !ok {
		return
	}

	bm.sentToBDN = true
	c.blockHashMetadata.Set(hash.String(), bm)
}

// MarkConfirmationSentToBDN marks a block confirmation as having been sent to the BDN, so it does not need to be sent again in the future
func (c *Chain) MarkConfirmationSentToBDN(hash ethcommon.Hash) {
	c.chainLock.Lock()
	defer c.chainLock.Unlock()

	bm, ok := c.getBlockMetadata(hash)
	if !ok {
		return
	}

	bm.cnfMsgSent = true
	c.blockHashMetadata.Set(hash.String(), bm)
}

// GetBodies assembles and returns a set of block bodies
func (c *Chain) GetBodies(hashes []ethcommon.Hash) ([]interfaces.ReadOnlyBeaconBlockBody, error) {
	bodies := make([]interfaces.ReadOnlyBeaconBlockBody, 0, len(hashes))
	for _, hash := range hashes {
		body, ok := c.getBlockBody(hash)
		if !ok {
			return nil, ErrBodyNotFound
		}
		bodies = append(bodies, body)
	}
	return bodies, nil
}

// GetHeaders assembles and returns a set of headers
func (c *Chain) GetHeaders(start eth.HashOrNumber, count int, skip int, reverse bool) ([]*ethpb.SignedBeaconBlockHeader, error) {
	c.chainLock.RLock()
	defer c.chainLock.RUnlock()

	if count < 0 {
		return nil, ErrQueryAmountIsNotValid
	}
	requestedHeaders := make([]*ethpb.SignedBeaconBlockHeader, 0, count)

	var (
		originHash   ethcommon.Hash
		originHeight uint64
	)

	// figure out query scheme, then initialize requested headers with the first entry
	if start.Number > 0 {
		originHeight = start.Number

		if originHeight > c.chainState.head().height {
			return nil, ErrFutureHeaders
		}

		tail := c.chainState.tail()
		if tail != nil && originHeight < tail.height {
			return nil, ErrAncientHeaders
		}

		originHeader, err := c.getHeaderAtHeight(originHeight)
		if err != nil {
			return nil, err
		}

		// originHeader may be nil if a block in the future is requested, return empty headers in that case
		if originHeader != nil {
			requestedHeaders = append(requestedHeaders, originHeader.Header)
		}
	} else if start.Hash != (ethcommon.Hash{}) {
		originHash = start.Hash
		bm, ok := c.getBlockMetadata(originHash)
		originHeight = bm.height

		if !ok {
			return nil, fmt.Errorf("could not retrieve a corresponding height for block: %v", originHash)
		}
		originHeader, ok := c.getBlockHeader(originHeight, originHash)
		if !ok {
			return nil, fmt.Errorf("no header was with slot %v and hash %v", originHeight, originHash)
		}
		requestedHeaders = append(requestedHeaders, originHeader.Header)
	} else {
		return nil, ErrInvalidRequest
	}

	// if only 1 header was requested, return result
	if count == 1 {
		return requestedHeaders, nil
	}

	directionalMultiplier := 1
	increment := skip + 1
	if reverse {
		directionalMultiplier = -1
	}
	increment *= directionalMultiplier

	nextHeight := int(originHeight) + increment
	if len(c.chainState) == 0 {
		return nil, fmt.Errorf("no entries stored at slot: %v", nextHeight)
	}

	// iterate through all requested headers and fetch results
	for height := nextHeight; len(requestedHeaders) < count; height += increment {
		header, err := c.getHeaderAtHeight(uint64(height))
		if err != nil {
			return nil, err
		}

		if header == nil {
			log.Tracef("requested slot %v is beyond best slot: ok", height)
			break
		}

		requestedHeaders = append(requestedHeaders, header.Header)
	}

	return requestedHeaders, nil
}

// BlockAtDepth returns the blockRefChain with depth from the head of the chain
func (c *Chain) BlockAtDepth(chainDepth int) (interfaces.ReadOnlySignedBeaconBlock, error) {
	if len(c.chainState) <= chainDepth {
		return nil, fmt.Errorf("not enough block in the chain state with length %v for depth lookup with depth of %v", len(c.chainState), chainDepth)
	}
	ref := c.chainState[chainDepth]
	header, err := c.getHeaderAtHeight(ref.height)
	if err != nil {
		return nil, err
	}
	body, ok := c.getBlockBody(ref.hash)
	if !ok {
		return nil, fmt.Errorf("cannot get block body for block %v with slot %v in the chain state ", ref.hash, ref.height)
	}
	block, err := newSignedBeaconBlock(header.version, header.Header, body)
	if err != nil {
		return nil, fmt.Errorf("cannot merge head and body for block %v", ref.hash)
	}

	return block, nil
}

// HeadHeight returns head height
func (c *Chain) HeadHeight() uint64 {
	return c.chainState.head().height
}

// should be called with c.chainLock held
func (c *Chain) updateChainState(height uint64, hash ethcommon.Hash, parentHash ethcommon.Hash) int {
	if len(c.chainState) == 0 {
		c.chainState = append(c.chainState, blockRef{
			height: height,
			hash:   hash,
		})
		return 1
	}

	chainHead := c.chainState[0]

	// canonical block, append immediately
	if chainHead.height+1 == height && chainHead.hash == parentHash {
		c.chainState = append([]blockRef{{height, hash}}, c.chainState...)
		return 1
	}

	// non-canonical block in the past, ignore for now
	if height <= chainHead.height {
		return 0
	}

	// better block than current head, try reorganizing with new best block
	missingEntries := make([]blockRef, 0, height-chainHead.height)

	headHeight := height
	headHash := hash

	// build chainstate from previous head to the latest block
	// e.g. suppose we had 10 on our head, and we just received block 14; we try to fill in entries 11-13 and check if that's all ok
	for ; headHeight > chainHead.height; headHeight-- {
		headHeader, ok := c.getBlockHeader(headHeight, headHash)
		if !ok {
			log.Debugf("cannot update chainstate, missing %d blocks", c.minValidChain-len(missingEntries))
			return 0
		}

		missingEntries = append(missingEntries, blockRef{height: headHeight, hash: headHash})
		headHash = ethcommon.BytesToHash(headHeader.Header.GetHeader().GetParentRoot())

		// suppose our head is 10, and we receive block 15-100 (for some reason 11-14 are never received), then we'll switch over the chain to be 15-100 as soon as the valid chain is >= c.minValidChain length
		if len(missingEntries) >= c.minValidChain {
			c.chainState = missingEntries
			return len(missingEntries)
		}
	}

	// chainstate was successfully reconciled (some entries were just missing), nothing else needed
	if headHeight == chainHead.height && headHash == chainHead.hash {
		c.chainState = append(missingEntries, c.chainState...)
		return len(missingEntries)
	}

	// reorganization is required, look back until c.maxReorg
	// e.g. suppose we had 10 on our head, and we just received block 14
	// 		- we filled 11-13, but 10b is 11's parent, so we iterate backward until we find a common ancestor
	//		- for example, suppose 9 doesn't match 10b's parent, but 8 matches 9b's parent, then we stop there
	//		- if this takes too long (> c.maxReorg needed), then we just trim the chainstate to c.maxReorg

	i := 0
	for ; i < len(c.chainState); i++ {
		chainRef := c.chainState[i]
		if headHash == chainRef.hash {
			// common ancestor found, break and recombine chains
			break
		}

		headHeader, ok := c.getBlockHeader(headHeight, headHash)
		if !ok {
			// TODO: log anything? chainstate can't be reconciled
			return 0
		}

		missingEntries = append(missingEntries, blockRef{height: headHeight, hash: headHash})
		headHash = ethcommon.BytesToHash(headHeader.Header.GetHeader().GetParentRoot())
		headHeight--

		// exceeded c.maxReorg, trim the chainstate
		if i+1 >= c.maxReorg {
			c.chainState = missingEntries
			return len(missingEntries)
		}
	}

	c.chainState = append(missingEntries, c.chainState[i:]...)
	return len(missingEntries)
}

// fetches correct header from chain, not store (require lock?)
func (c *Chain) getHeaderAtHeight(height uint64) (*ethBeaconHeader, error) {
	if len(c.chainState) == 0 {
		return nil, fmt.Errorf("%v: no header at slot %v", c.chainState, height)
	}

	head := c.chainState[0]
	requestedIndex := int(head.height - height)

	// requested block in the future, ok to break with no header
	if requestedIndex < 0 {
		return nil, nil
	}

	// requested block too far in the past, fail out
	if requestedIndex >= len(c.chainState) {
		return nil, fmt.Errorf("%v: no header at slot %v", c.chainState, height)
	}

	header, ok := c.getBlockHeader(height, c.chainState[requestedIndex].hash)

	// block in chainstate seems to no longer be in storage, error out
	if !ok {
		return nil, fmt.Errorf("%v: no header at slot %v", c.chainState, height)
	}
	return header, nil
}

func (c *Chain) storeBlock(hash ethcommon.Hash, block interfaces.ReadOnlySignedBeaconBlock, source BlockSource) error {
	header, err := block.Header()
	if err != nil {
		return fmt.Errorf("could not get block header: %v", err)
	}

	c.storeBlockHeader(block.Version(), hash, header, source)
	c.storeBlockBody(hash, block.Block().Body())

	return nil
}

func (c *Chain) getBlockHeader(height uint64, hash ethcommon.Hash) (*ethBeaconHeader, bool) {
	headers, ok := c.getHeadersAtHeight(height)
	if !ok {
		return nil, ok
	}
	for _, header := range headers {
		if bytes.Equal(header.hash.Bytes(), hash.Bytes()) {
			return &header, true
		}
	}
	return nil, false
}

func (c *Chain) storeBlockHeader(version int, hash ethcommon.Hash, header *ethpb.SignedBeaconBlockHeader, source BlockSource) {
	height := uint64(header.GetHeader().Slot)
	c.storeHeaderAtHeight(hash, version, height, header)
	c.storeBlockMetadata(hash, height, source == BSBlockchain, false)
}

func (c *Chain) getBlockMetadata(hash ethcommon.Hash) (blockMetadata, bool) {
	bm, ok := c.blockHashMetadata.Get(hash.String())
	if !ok {
		return blockMetadata{}, ok
	}
	return bm.(blockMetadata), ok
}

func (c *Chain) storeBlockMetadata(hash ethcommon.Hash, height uint64, confirmed bool, cnfMsgSent bool) {
	set := c.blockHashMetadata.SetIfAbsent(hash.String(), blockMetadata{height, false, confirmed, cnfMsgSent})
	if !set {
		bm, _ := c.getBlockMetadata(hash)
		bm.confirmed = bm.confirmed || confirmed
		bm.cnfMsgSent = bm.cnfMsgSent || cnfMsgSent
		c.blockHashMetadata.Set(hash.String(), bm)
	}
}

func (c *Chain) removeBlockMetadata(hash ethcommon.Hash) {
	c.blockHashMetadata.Remove(hash.String())
}

func (c *Chain) hasHeader(hash ethcommon.Hash) bool {
	// always corresponds to a header stored at c.heightToBlockHeaders
	return c.blockHashMetadata.Has(hash.String())
}

func (c *Chain) getHeadersAtHeight(height uint64) ([]ethBeaconHeader, bool) {
	rawHeaders, ok := c.heightToBlockHeaders.Get(strconv.FormatUint(height, 10))
	if !ok {
		return nil, ok
	}

	return rawHeaders.([]ethBeaconHeader), ok
}

func (c *Chain) storeHeaderAtHeight(hash ethcommon.Hash, version int, height uint64, header *ethpb.SignedBeaconBlockHeader) {
	eh := ethBeaconHeader{
		Header:  header,
		hash:    hash,
		version: version,
	}
	c.storeEthHeaderAtHeight(height, eh)
}

// generally avoid calling this function directly
func (c *Chain) storeEthHeaderAtHeight(height uint64, eh ethBeaconHeader) {
	// concurrent calls to this function are ok, only needs to be exclusionary with clean
	c.headerLock.RLock()
	defer c.headerLock.RUnlock()

	heightStr := strconv.FormatUint(height, 10)

	ok := c.heightToBlockHeaders.SetIfAbsent(heightStr, []ethBeaconHeader{eh})
	if !ok {
		rawHeaders, _ := c.heightToBlockHeaders.Get(heightStr)
		ethHeaders := rawHeaders.([]ethBeaconHeader)
		ethHeaders = append(ethHeaders, eh)
		c.heightToBlockHeaders.Set(heightStr, ethHeaders)
	}
}

func (c *Chain) storeBlockBody(hash ethcommon.Hash, body interfaces.ReadOnlyBeaconBlockBody) {
	c.blockHashToBody.Set(hash.String(), body)
}

func (c *Chain) getBlockBody(hash ethcommon.Hash) (interfaces.ReadOnlyBeaconBlockBody, bool) {
	body, ok := c.blockHashToBody.Get(hash.String())
	if !ok {
		return nil, ok
	}

	return body.(interfaces.ReadOnlyBeaconBlockBody), ok
}

func (c *Chain) hasBody(hash ethcommon.Hash) bool {
	return c.blockHashToBody.Has(hash.String())
}

func (c *Chain) removeBlockBody(hash ethcommon.Hash) {
	c.blockHashToBody.Remove(hash.String())
}

// removes all info corresponding to a given block in storage
func (c *Chain) pruneHash(hash ethcommon.Hash) {
	c.removeBlockMetadata(hash)
	c.removeBlockBody(hash)
}

func (c *Chain) clean(maxSize int) (lowestCleaned int, highestCleaned int, numCleaned int) {
	c.headerLock.Lock()
	defer c.headerLock.Unlock()

	c.chainLock.Lock()
	defer c.chainLock.Unlock()

	// Find largest height from headers if chainState is empty
	// This may happened if no connection to node was established but we receiving blocks from BDN
	if len(c.chainState) == 0 {
		var maxHeight int
		for elem := range c.heightToBlockHeaders.IterBuffered() {
			heightStr := elem.Key
			height, err := strconv.Atoi(heightStr)
			if err != nil {
				log.Errorf("failed to convert slot %v from string to integer: %v", heightStr, err)
				continue
			}

			if height > maxHeight {
				maxHeight = height
			}
		}

		lowestCleaned = maxHeight
	} else {
		lowestCleaned = int(c.chainState[0].height)
	}

	numCleaned = 0
	highestCleaned = 0

	// minimum height to not be cleaned
	minHeight := lowestCleaned - maxSize + 1
	numHeadersStored := c.heightToBlockHeaders.Count()

	if numHeadersStored >= maxSize {
		for elem := range c.heightToBlockHeaders.IterBuffered() {
			heightStr := elem.Key
			height, err := strconv.Atoi(heightStr)
			if err != nil {
				log.Errorf("failed to convert slot %v from string to integer: %v", heightStr, err)
				continue
			}
			if height < minHeight {
				headers := elem.Val.([]ethBeaconHeader)
				c.heightToBlockHeaders.Remove(heightStr)
				for _, header := range headers {
					hash := header.hash
					c.pruneHash(hash)

					numCleaned++
					if height < lowestCleaned {
						lowestCleaned = height
					}
					if height > highestCleaned {
						highestCleaned = height
					}
				}
			}
		}

		chainStatePruned := 0
		if len(c.chainState) > maxSize {
			chainStatePruned = len(c.chainState) - maxSize
			c.chainState = c.chainState[:maxSize]
		}

		log.Debugf("cleaned block storage (previous size %v out of max %v): %v block headers from %v to %v, pruning %v elements off of chainstate", numHeadersStored, maxSize, numCleaned, lowestCleaned, highestCleaned, chainStatePruned)
	} else {
		log.Debugf("skipping block storage cleanup, only had %v block headers out of a limit of %v", numHeadersStored, maxSize)
	}
	return
}

func newSignedBeaconBlock(ver int, header *ethpb.SignedBeaconBlockHeader, body interfaces.ReadOnlyBeaconBlockBody) (interfaces.ReadOnlySignedBeaconBlock, error) {
	var err error

	var sb interface{}
	switch ver {
	case version.Phase0:
		sb, err = newSignedBeaconBlockPhase0(header, body)
		if err != nil {
			return nil, err
		}
	case version.Altair:
		sb, err = newSignedBeaconBlockAltair(header, body)
		if err != nil {
			return nil, err
		}
	case version.Bellatrix:
		sb, err = newSignedBeaconBlockBellatrix(header, body)
		if err != nil {
			return nil, err
		}
	case version.Capella:
		sb, err = newSignedBeaconBlockCapella(header, body)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("block version %v is not recognized", ver)
	}

	return blocks.NewSignedBeaconBlock(sb)
}

func newSignedBeaconBlockPhase0(header *ethpb.SignedBeaconBlockHeader, body interfaces.ReadOnlyBeaconBlockBody) (*ethpb.SignedBeaconBlock, error) {
	randaoReveal := body.RandaoReveal()
	graffiti := body.Graffiti()

	return &ethpb.SignedBeaconBlock{
		Block: &ethpb.BeaconBlock{
			Slot:          header.GetHeader().GetSlot(),
			ProposerIndex: header.GetHeader().GetProposerIndex(),
			ParentRoot:    header.GetHeader().GetParentRoot(),
			StateRoot:     header.GetHeader().GetStateRoot(),
			Body: &ethpb.BeaconBlockBody{
				RandaoReveal:      randaoReveal[:],
				Eth1Data:          body.Eth1Data(),
				Graffiti:          graffiti[:],
				ProposerSlashings: body.ProposerSlashings(),
				AttesterSlashings: body.AttesterSlashings(),
				Attestations:      body.Attestations(),
				Deposits:          body.Deposits(),
				VoluntaryExits:    body.VoluntaryExits(),
			},
		},
		Signature: header.GetSignature(),
	}, nil
}

func newSignedBeaconBlockAltair(header *ethpb.SignedBeaconBlockHeader, body interfaces.ReadOnlyBeaconBlockBody) (*ethpb.SignedBeaconBlockAltair, error) {
	hash, err := body.HashTreeRoot()
	if err != nil {
		return nil, errors.New("could not calculate hash")
	}

	syncAggregate, err := body.SyncAggregate()
	if err != nil {
		return nil, fmt.Errorf("could not get block %v sync aggregate: %v", hash, err)
	}

	randaoReveal := body.RandaoReveal()
	graffiti := body.Graffiti()

	return &ethpb.SignedBeaconBlockAltair{
		Block: &ethpb.BeaconBlockAltair{
			Slot:          header.GetHeader().GetSlot(),
			ProposerIndex: header.GetHeader().GetProposerIndex(),
			ParentRoot:    header.GetHeader().GetParentRoot(),
			StateRoot:     header.GetHeader().GetStateRoot(),
			Body: &ethpb.BeaconBlockBodyAltair{
				RandaoReveal:      randaoReveal[:],
				Eth1Data:          body.Eth1Data(),
				Graffiti:          graffiti[:],
				ProposerSlashings: body.ProposerSlashings(),
				AttesterSlashings: body.AttesterSlashings(),
				Attestations:      body.Attestations(),
				Deposits:          body.Deposits(),
				VoluntaryExits:    body.VoluntaryExits(),
				SyncAggregate:     syncAggregate,
			},
		},
		Signature: header.GetSignature(),
	}, nil
}

func newSignedBeaconBlockBellatrix(header *ethpb.SignedBeaconBlockHeader, body interfaces.ReadOnlyBeaconBlockBody) (*ethpb.SignedBeaconBlockBellatrix, error) {
	hash, err := body.HashTreeRoot()
	if err != nil {
		return nil, errors.New("could not calculate hash")
	}

	syncAggregate, err := body.SyncAggregate()
	if err != nil {
		return nil, fmt.Errorf("could not get block %v sync aggregate: %v", hash, err)
	}

	execution, err := body.Execution()
	if err != nil {
		return nil, fmt.Errorf("could not get block %v execution: %v", hash, err)
	}

	bellatrixExecution, err := execution.PbBellatrix()
	if err != nil {
		return nil, fmt.Errorf("could not get block %v execution payload: %v", hash, err)
	}

	randaoReveal := body.RandaoReveal()
	graffiti := body.Graffiti()

	return &ethpb.SignedBeaconBlockBellatrix{
		Block: &ethpb.BeaconBlockBellatrix{
			Slot:          header.GetHeader().GetSlot(),
			ProposerIndex: header.GetHeader().GetProposerIndex(),
			ParentRoot:    header.GetHeader().GetParentRoot(),
			StateRoot:     header.GetHeader().GetStateRoot(),
			Body: &ethpb.BeaconBlockBodyBellatrix{
				RandaoReveal:      randaoReveal[:],
				Eth1Data:          body.Eth1Data(),
				Graffiti:          graffiti[:],
				ProposerSlashings: body.ProposerSlashings(),
				AttesterSlashings: body.AttesterSlashings(),
				Attestations:      body.Attestations(),
				Deposits:          body.Deposits(),
				VoluntaryExits:    body.VoluntaryExits(),
				SyncAggregate:     syncAggregate,
				ExecutionPayload:  bellatrixExecution,
			},
		},
		Signature: header.GetSignature(),
	}, nil
}

func newSignedBeaconBlockCapella(header *ethpb.SignedBeaconBlockHeader, body interfaces.ReadOnlyBeaconBlockBody) (*ethpb.SignedBeaconBlockCapella, error) {
	hash, err := body.HashTreeRoot()
	if err != nil {
		return nil, errors.New("could not calculate hash")
	}

	syncAggregate, err := body.SyncAggregate()
	if err != nil {
		return nil, fmt.Errorf("could not get block %v sync aggregate: %v", hash, err)
	}

	execution, err := body.Execution()
	if err != nil {
		return nil, fmt.Errorf("could not get block %v execution: %v", hash, err)
	}

	capellaExecution, err := execution.PbCapella()
	if err != nil {
		return nil, fmt.Errorf("could not get block %v capella execution: %v", hash, err)
	}

	randaoReveal := body.RandaoReveal()
	graffiti := body.Graffiti()

	return &ethpb.SignedBeaconBlockCapella{
		Block: &ethpb.BeaconBlockCapella{
			Slot:          header.GetHeader().GetSlot(),
			ProposerIndex: header.GetHeader().GetProposerIndex(),
			ParentRoot:    header.GetHeader().GetParentRoot(),
			StateRoot:     header.GetHeader().GetStateRoot(),
			Body: &ethpb.BeaconBlockBodyCapella{
				RandaoReveal:      randaoReveal[:],
				Eth1Data:          body.Eth1Data(),
				Graffiti:          graffiti[:],
				ProposerSlashings: body.ProposerSlashings(),
				AttesterSlashings: body.AttesterSlashings(),
				Attestations:      body.Attestations(),
				Deposits:          body.Deposits(),
				VoluntaryExits:    body.VoluntaryExits(),
				SyncAggregate:     syncAggregate,
				ExecutionPayload:  capellaExecution,
			},
		},
		Signature: header.GetSignature(),
	}, nil
}
