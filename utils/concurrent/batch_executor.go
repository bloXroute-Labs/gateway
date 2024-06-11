package concurrent

import (
	"hash/maphash"
	"runtime"
	"time"
	"unsafe"

	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

// resultWrapper holds the result or error from executing a request and a channel to signal completion.
type resultWrapper[Result any] struct {
	done      chan struct{}
	result    Result
	err       error
	cleanTime time.Time // time when the result is no longer needed
}

// ExecuteFunc is a generic function type that processes a batch of requests of type K and returns results of type T.
// It is intended to be implemented by the caller of BatchExecutor to define specific execution logic.
type ExecuteFunc[Request comparable, Result any] func(req []Request) ([]Result, error)

// BatchExecutor manages the execution of batches of requests ensuring that each unique request
// is processed only once concurrently. It is useful for scenarios where tasks or requests need to be deduplicated
// and processed efficiently without overlap, such as network calls or database operations.
type BatchExecutor[Request comparable, Result any] struct {
	workChan       chan work[Request, Result]
	storeQueueChan chan work[Request, Result]
	data           *syncmap.SyncMap[Request, *resultWrapper[Result]]
	executeFunc    ExecuteFunc[Request, Result]
	batchSize      int
	flushDuration  time.Duration
	cleanDur       time.Duration
}

// NewStringBatchExecutor creates a new instance of BatchExecutor with string requests.
func NewStringBatchExecutor[Result any](executeFunc ExecuteFunc[string, Result], batchSize int, flushDuration time.Duration, cleanDur time.Duration) *BatchExecutor[string, Result] {
	return NewBatchExecutor(executeFunc, syncmap.StringHasher, batchSize, flushDuration, cleanDur)
}

// NewIntegerBatchExecutor creates a new instance of BatchExecutor with integer requests.
func NewIntegerBatchExecutor[Request ~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr, Result any](processFunc ExecuteFunc[Request, Result], batchSize int, flushDuration time.Duration, ttl time.Duration) *BatchExecutor[Request, Result] {
	return NewBatchExecutor(processFunc, func(seed maphash.Seed, r Request) uint64 {
		// TODO: put it somewhere
		n := uint64(r)
		// Java's Long standard hash function.
		n = n ^ (n >> 32)
		nseed := *(*uint64)(unsafe.Pointer(&seed))
		// 64-bit variation of boost's hash_combine.
		nseed ^= n + 0x9e3779b97f4a7c15 + (nseed << 12) + (nseed >> 4)
		return nseed
	}, batchSize, flushDuration, ttl)
}

// NewBatchExecutor creates a new instance of BatchExecutor with the specified
// execute function that defines how each batch of requests is processed.
func NewBatchExecutor[Request comparable, Result any](executeFunc ExecuteFunc[Request, Result], hasher syncmap.Hasher[Request], batchSize int, flushDuration time.Duration, cleanDur time.Duration) *BatchExecutor[Request, Result] {
	e := &BatchExecutor[Request, Result]{
		workChan:       make(chan work[Request, Result], 100000),
		storeQueueChan: make(chan work[Request, Result], 100000),
		data:           syncmap.NewTypedMapOf[Request, *resultWrapper[Result]](hasher),
		executeFunc:    executeFunc,
		batchSize:      batchSize,
		flushDuration:  flushDuration,
		cleanDur:       cleanDur,
	}

	for i := 0; i < runtime.GOMAXPROCS(0); i++ {
		go e.processRoutine(e.batchSize, e.flushDuration)
	}

	go e.processStores()
	go e.cleanRoutine(cleanDur)

	return e
}

// Execute takes a slice of requests and processes them using the provided ExecuteFunc.
// It ensures that each request in the batch is processed only once, even if requested multiple times concurrently,
// thus improving efficiency and preventing unnecessary work in concurrent environments.
//
// Duplicate requests wait for the result of the initial request's processing. If the processing of any request
// results in an error, that error is returned immediately for all requests in the batch, and the processing halts.
// Notably, if an error occurs, the failed request is removed from the internal cache, allowing for the possibility
// of retrying the request. This mechanism ensures that transient errors do not permanently prevent the reprocessing
// of requests.
//
// Returns a slice of results corresponding to the input requests and any error encountered during processing.
// Users of this method should consider handling errors by potentially retrying failed requests, taking into account
// the nature of the error to avoid unnecessary retries in cases of permanent failures.
func (e *BatchExecutor[Request, Result]) Execute(reqs []Request) ([]Result, []Request, error) {
	reqsToProcess := make([]Request, 0, len(reqs))
	resultsToProcess := make([]*resultWrapper[Result], 0, len(reqs))
	resultWrappers := make([]*resultWrapper[Result], len(reqs))

	addTime := time.Now().Add(e.cleanDur)

	var loaded bool
	for i, req := range reqs {
		resultWrappers[i], loaded = e.data.Load(req)
		if !loaded {
			resultWrappers[i] = &resultWrapper[Result]{
				cleanTime: addTime,
				done:      make(chan struct{}),
			}

			reqsToProcess = append(reqsToProcess, req)
			resultsToProcess = append(resultsToProcess, resultWrappers[i])
		}
	}

	work := work[Request, Result]{request: reqsToProcess, result: resultsToProcess}
	e.storeQueueChan <- work
	e.workChan <- work

	results := make([]Result, len(reqs))
	for i := range resultWrappers {
		<-resultWrappers[i].done
		if resultWrappers[i].err != nil {
			return nil, reqsToProcess, resultWrappers[i].err
		}
		results[i] = resultWrappers[i].result
	}

	return results, reqsToProcess, nil
}

type work[Request comparable, Result any] struct {
	request []Request
	result  []*resultWrapper[Result]
}

func (e *BatchExecutor[Request, Result]) processStores() {
	for work := range e.storeQueueChan {
		for i := range work.request {
			e.data.Store(work.request[i], work.result[i])
		}
	}
}

func (e *BatchExecutor[Request, Result]) processRoutine(batchSize int, flushDuration time.Duration) {
	requests := make([]Request, 0, batchSize*2)
	results := make([]*resultWrapper[Result], 0, batchSize*2)
	ticker := time.NewTicker(flushDuration)

	process := func(requests []Request, results []*resultWrapper[Result]) {
		processedResults, err := e.executeFunc(requests)

		for i, req := range requests {
			if err != nil {
				results[i].err = err
				e.data.Delete(req)
			} else {
				results[i].result = processedResults[i]
			}

			close(results[i].done)
		}
	}

	requestsCount := 0
	for {
		select {
		case <-ticker.C:
			if requestsCount == 0 {
				break
			}

			process(requests, results)

			ticker.Reset(flushDuration)
			requests = make([]Request, 0, len(requests))
			results = make([]*resultWrapper[Result], 0, len(results))
			requestsCount = 0
		case work := <-e.workChan:
			requestsCount += len(work.request)
			requests = append(requests, work.request...)
			results = append(results, work.result...)
			ticker.Reset(flushDuration)

			if requestsCount < batchSize {
				break
			}

			process(requests, results)

			requests = make([]Request, 0, len(requests))
			results = make([]*resultWrapper[Result], 0, len(results))
			requestsCount = 0
		}
	}
}

func (e *BatchExecutor[Request, Result]) Clear() {
	e.data.Clear()
}

func (e *BatchExecutor[Request, Result]) cleanRoutine(cleanInterval time.Duration) {
	ticker := time.NewTicker(cleanInterval)

	for range ticker.C {
		e.data.Range(func(key Request, value *resultWrapper[Result]) bool {
			if time.Now().After(value.cleanTime) {
				e.data.Delete(key)
			}
			return true
		})
	}
}
