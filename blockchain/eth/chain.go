package eth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	cmap "github.com/orcaman/concurrent-map"
	log "github.com/sirupsen/logrus"
	"math/big"
	"strconv"
	"sync"
	"time"
)

const (
	maxReorgLength       = 20
	minValidChainLength  = 10
	defaultMaxSize       = 500
	defaultCleanInterval = 30 * time.Minute
)

// Chain represents and stores blockchain state info in memory
type Chain struct {
	chainLock  sync.RWMutex // lock for updating chainstate
	headerLock sync.RWMutex // lock for block headers/heights

	// if reconciling a fork takes longer than this value, then trim the chain to this length
	maxReorg int

	// if a missing block is preventing updating the chain head, once a valid chain of this length is possible, discard the old chain
	minValidChain int

	heightToBlockHeaders  cmap.ConcurrentMap
	blockHashMetadata     cmap.ConcurrentMap
	blockHashToBody       cmap.ConcurrentMap
	blockHashToDifficulty cmap.ConcurrentMap

	chainState blockRefChain
}

// BlockSource indicates the origin of a block message in the blockchain
type BlockSource string

// enumerate types of BlockSource
const (
	BSBDN        BlockSource = "BDN"
	BSBlockchain             = "Blockchain"
)

// BlockInfo wraps an Ethereum block with its total difficulty.
type BlockInfo struct {
	Block           *ethtypes.Block
	totalDifficulty *big.Int
}

// NewBlockInfo composes a new BlockInfo. nil is considered a valid total difficulty for constructing this struct
func NewBlockInfo(block *ethtypes.Block, totalDifficulty *big.Int) *BlockInfo {
	info := &BlockInfo{
		Block: block,
	}
	info.SetTotalDifficulty(totalDifficulty)
	return info
}

// SetTotalDifficulty sets the total difficulty, filtering nil arguments
func (e *BlockInfo) SetTotalDifficulty(td *big.Int) {
	if td == nil {
		e.totalDifficulty = big.NewInt(0)
	} else {
		e.totalDifficulty = td
	}
}

// TotalDifficulty validates and returns the block's total difficulty. Any value <=0 may be encoded in types.BxBlock, and are considered invalid difficulties that should be treated as "unknown diffiuclty"
func (e BlockInfo) TotalDifficulty() *big.Int {
	if e.totalDifficulty.Int64() <= 0 {
		return nil
	}
	return e.totalDifficulty
}

type blockMetadata struct {
	height    uint64
	sentToBDN bool
	confirmed bool
}

type ethHeader struct {
	*ethtypes.Header
	hash ethcommon.Hash
}

// NewChain returns a new chainstate struct for usage
func NewChain(ctx context.Context) *Chain {
	return newChain(ctx, maxReorgLength, minValidChainLength, defaultCleanInterval, defaultMaxSize)
}

func newChain(ctx context.Context, maxReorg, minValidChain int, cleanInterval time.Duration, maxSize int) *Chain {
	c := &Chain{
		chainLock:             sync.RWMutex{},
		headerLock:            sync.RWMutex{},
		heightToBlockHeaders:  cmap.New(),
		blockHashMetadata:     cmap.New(),
		blockHashToBody:       cmap.New(),
		blockHashToDifficulty: cmap.New(),
		chainState:            make([]blockRef, 0),
		maxReorg:              maxReorg,
		minValidChain:         minValidChain,
	}
	go c.cleanBlockStorage(ctx, cleanInterval, maxSize)
	return c
}

func (c *Chain) cleanBlockStorage(ctx context.Context, cleanInterval time.Duration, maxSize int) {
	ticker := time.NewTicker(cleanInterval)
	for {
		select {
		case <-ticker.C:
			c.clean(maxSize)
		case <-ctx.Done():
			return
		}
	}
}

// AddBlock adds the provided block from the source into storage, updating the chainstate if the block comes from a reliable source. AddBlock returns the number of new canonical hashes added to the head if a reorganization happened. TODO: consider computing difficulty in here?
func (c *Chain) AddBlock(b *BlockInfo, source BlockSource) int {
	c.chainLock.Lock()
	defer c.chainLock.Unlock()

	height := b.Block.NumberU64()
	hash := b.Block.Hash()
	parentHash := b.Block.ParentHash()

	// update metadata if block already stored, otherwise update all block info
	if c.HasBlock(hash) {
		c.storeBlockMetadata(hash, height, source == BSBlockchain)
	} else {
		c.storeBlock(b.Block, b.TotalDifficulty(), source)
	}

	// if source is BDN, then no authority to update chainstate and indicate no new heads to return
	if source == BSBDN {
		return 0
	}

	return c.updateChainState(height, hash, parentHash)
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

	return c.updateChainState(bm.height, hash, header.ParentHash)
}

// GetNewHeadsForBDN fetches the newest blocks on the chainstate that have not previously been sent to the BDN. In cases of error, as many entries are still returned along with the error. Entries are returned in descending order.
func (c *Chain) GetNewHeadsForBDN(count int) ([]*BlockInfo, error) {
	c.chainLock.RLock()
	defer c.chainLock.RUnlock()

	heads := make([]*BlockInfo, 0, count)

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

		block := ethtypes.NewBlockWithHeader(header).WithBody(body.Transactions, body.Uncles)
		// ok for difficulty to not be found since not always available
		td, _ := c.getBlockDifficulty(head.hash)

		heads = append(heads, NewBlockInfo(block, td))
	}

	return heads, nil
}

// ValidateBlock determines if block can potentially be added to the chain
func (c *Chain) ValidateBlock(hash ethcommon.Hash, height int64) error {
	if c.HasBlock(hash) && c.HasSentToBDN(hash) && c.HasConfirmedBlock(hash) {
		return ErrAlreadySeen
	}

	chainHead := c.chainState.head()
	maxBlockNumber := chainHead.height + maxFutureBlockNumber
	if chainHead.height != 0 && height > int64(maxBlockNumber) {
		return fmt.Errorf("too far in future (best height: %v, max allowed height: %v)", chainHead.height, maxBlockNumber)
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

// InitializeDifficulty stores an initial difficulty if needed to start calculating total difficulties. Only 1 difficulty is stored, since these difficulties are never GC'ed.
func (c *Chain) InitializeDifficulty(hash ethcommon.Hash, td *big.Int) {
	c.storeBlockDifficulty(hash, td)
	c.storeEthHeaderAtHeight(0, ethHeader{hash: hash})
	c.storeBlockMetadata(hash, 0, true)
}

// SetTotalDifficulty computes, sets, and stores the difficulty for a provided block
func (c *Chain) SetTotalDifficulty(info *BlockInfo) error {
	totalDifficulty := info.TotalDifficulty()
	if totalDifficulty != nil {
		return nil
	}

	parentHash := info.Block.ParentHash()
	parentDifficulty, ok := c.getBlockDifficulty(parentHash)
	if ok {
		info.SetTotalDifficulty(new(big.Int).Add(parentDifficulty, info.Block.Difficulty()))
		c.storeBlockDifficulty(info.Block.Hash(), info.TotalDifficulty())
		return nil
	}
	return errors.New("could not calculate difficulty")
}

// GetBodies assembles and returns a set of block bodies
func (c *Chain) GetBodies(hashes []ethcommon.Hash) ([]*ethtypes.Body, error) {
	bodies := make([]*ethtypes.Body, 0, len(hashes))
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
func (c *Chain) GetHeaders(start eth.HashOrNumber, count int, skip int, reverse bool) ([]*ethtypes.Header, error) {
	c.chainLock.RLock()
	defer c.chainLock.RUnlock()

	requestedHeaders := make([]*ethtypes.Header, 0, count)

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
			requestedHeaders = append(requestedHeaders, originHeader)
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
			return nil, fmt.Errorf("no header was with height %v and hash %v", originHeight, originHash)
		}
		requestedHeaders = append(requestedHeaders, originHeader)
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
		return nil, fmt.Errorf("no entries stored at height: %v", nextHeight)
	}

	// iterate through all requested headers and fetch results
	for height := nextHeight; len(requestedHeaders) < count; height += increment {
		header, err := c.getHeaderAtHeight(uint64(height))
		if err != nil {
			return nil, err
		}

		if header == nil {
			log.Tracef("requested height %v is beyond best height: ok", height)
			break
		}

		requestedHeaders = append(requestedHeaders, header)
	}

	return requestedHeaders, nil
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
			// TODO: log anything? chainstate can't be reconciled (maybe should just prune to head)
			return 0
		}

		missingEntries = append(missingEntries, blockRef{height: headHeight, hash: headHash})
		headHash = headHeader.ParentHash

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
		headHash = headHeader.ParentHash
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
func (c *Chain) getHeaderAtHeight(height uint64) (*ethtypes.Header, error) {
	if len(c.chainState) == 0 {
		return nil, fmt.Errorf("%v: no header at height %v", c.chainState, height)
	}

	head := c.chainState[0]
	requestedIndex := int(head.height - height)

	// requested block in the future, ok to break with no header
	if requestedIndex < 0 {
		return nil, nil
	}

	// requested block too far in the past, fail out
	if requestedIndex >= len(c.chainState) {
		return nil, fmt.Errorf("%v: no header at height %v", c.chainState, height)
	}

	header, ok := c.getBlockHeader(height, c.chainState[requestedIndex].hash)

	// block in chainstate seems to no longer be in storage, error out
	if !ok {
		return nil, fmt.Errorf("%v: no header at height %v", c.chainState, height)
	}
	return header, nil
}

func (c *Chain) storeBlock(block *ethtypes.Block, difficulty *big.Int, source BlockSource) {
	c.storeBlockHeader(block.Header(), source)
	c.storeBlockBody(block.Hash(), block.Body())
	c.storeBlockDifficulty(block.Hash(), difficulty)
}

func (c *Chain) getBlockHeader(height uint64, hash ethcommon.Hash) (*ethtypes.Header, bool) {
	headers, ok := c.getHeadersAtHeight(height)
	if !ok {
		return nil, ok
	}
	for _, header := range headers {
		if bytes.Equal(header.Hash().Bytes(), hash.Bytes()) {
			return header, true
		}
	}
	return nil, false
}

func (c *Chain) storeBlockHeader(header *ethtypes.Header, source BlockSource) {
	blockHash := header.Hash()
	height := header.Number.Uint64()
	c.storeHeaderAtHeight(height, header)
	c.storeBlockMetadata(blockHash, height, source == BSBlockchain)
}

func (c *Chain) getBlockMetadata(hash ethcommon.Hash) (blockMetadata, bool) {
	bm, ok := c.blockHashMetadata.Get(hash.String())
	if !ok {
		return blockMetadata{}, ok
	}
	return bm.(blockMetadata), ok
}

func (c *Chain) storeBlockMetadata(hash ethcommon.Hash, height uint64, confirmed bool) {
	set := c.blockHashMetadata.SetIfAbsent(hash.String(), blockMetadata{height, false, confirmed})
	if !set {
		bm, _ := c.getBlockMetadata(hash)
		bm.confirmed = bm.confirmed || confirmed
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

func (c *Chain) getHeadersAtHeight(height uint64) ([]*ethtypes.Header, bool) {
	rawHeaders, ok := c.heightToBlockHeaders.Get(strconv.FormatUint(height, 10))
	if !ok {
		return nil, ok
	}

	ethHeaders := rawHeaders.([]ethHeader)
	headers := make([]*ethtypes.Header, 0, len(ethHeaders))

	for _, eh := range ethHeaders {
		headers = append(headers, eh.Header)
	}
	return headers, ok
}

func (c *Chain) storeHeaderAtHeight(height uint64, header *ethtypes.Header) {
	eh := ethHeader{
		Header: header,
		hash:   header.Hash(),
	}
	c.storeEthHeaderAtHeight(height, eh)
}

// generally avoid calling this function directly
func (c *Chain) storeEthHeaderAtHeight(height uint64, eh ethHeader) {
	// concurrent calls to this function are ok, only needs to be exclusionary with clean
	c.headerLock.RLock()
	defer c.headerLock.RUnlock()

	heightStr := strconv.FormatUint(height, 10)

	ok := c.heightToBlockHeaders.SetIfAbsent(heightStr, []ethHeader{eh})
	if !ok {
		rawHeaders, _ := c.heightToBlockHeaders.Get(heightStr)
		ethHeaders := rawHeaders.([]ethHeader)
		ethHeaders = append(ethHeaders, eh)
		c.heightToBlockHeaders.Set(heightStr, ethHeaders)
	}
}

func (c *Chain) storeBlockDifficulty(hash ethcommon.Hash, difficulty *big.Int) {
	if difficulty != nil {
		c.blockHashToDifficulty.Set(hash.String(), difficulty)
	}
}

func (c *Chain) getBlockDifficulty(hash ethcommon.Hash) (*big.Int, bool) {
	difficulty, ok := c.blockHashToDifficulty.Get(hash.String())
	if !ok {
		return nil, ok
	}
	return difficulty.(*big.Int), ok
}

func (c *Chain) removeBlockDifficulty(hash ethcommon.Hash) {
	c.blockHashToDifficulty.Remove(hash.String())
}

func (c *Chain) storeBlockBody(hash ethcommon.Hash, body *ethtypes.Body) {
	c.blockHashToBody.Set(hash.String(), body)
}

func (c *Chain) getBlockBody(hash ethcommon.Hash) (*ethtypes.Body, bool) {
	body, ok := c.blockHashToBody.Get(hash.String())
	if !ok {
		return nil, ok
	}
	return body.(*ethtypes.Body), ok
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
	c.removeBlockDifficulty(hash)
}

func (c *Chain) clean(maxSize int) (lowestCleaned int, highestCleaned int, numCleaned int) {
	c.headerLock.Lock()
	defer c.headerLock.Unlock()

	c.chainLock.Lock()
	defer c.chainLock.Unlock()

	if len(c.chainState) == 0 {
		return
	}

	head := c.chainState[0]

	numCleaned = 0
	lowestCleaned = int(head.height)
	highestCleaned = 0

	// minimum height to not be cleaned
	minHeight := lowestCleaned - maxSize + 1
	numHeadersStored := c.heightToBlockHeaders.Count()

	if numHeadersStored >= maxSize {
		for elem := range c.heightToBlockHeaders.IterBuffered() {
			heightStr := elem.Key
			height, err := strconv.Atoi(heightStr)
			if err != nil {
				log.Errorf("failed to convert height %v from string to integer: %v", heightStr, err)
				continue
			}
			if height < minHeight {
				headers := elem.Val.([]ethHeader)
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
