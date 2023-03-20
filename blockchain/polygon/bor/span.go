package bor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	ordermap "github.com/wk8/go-ordered-map/v2"

	"github.com/cenkalti/backoff/v4"
	"github.com/pkg/errors"
	"go.uber.org/atomic"

	log "github.com/bloXroute-Labs/gateway/v2/logger"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/polygon/bor/valset"
)

const (
	endpointLatestSpanFmt = "%s/bor/latest-span"
	endpointSpanIDFmt     = "%s/bor/span/%d"

	// SpanSize size of Heimdall span
	SpanSize = 100 * SprintSizeHeimdall
)

var (
	errCorruptedSpanMap    = errors.New("span map corrupted or uninitialized")
	errBadSpanResp         = errors.New("bad span response")
	errBadHeimdallEndpoint = errors.New("bad heimdall endpoint")
)

// SpanInfo basic info about span
type SpanInfo struct {
	SelectedProducers []*valset.Validator `json:"selected_producers"`

	StartBlock uint64 `json:"start_block"`
	EndBlock   uint64 `json:"end_block"`
	SpanID     uint64 `json:"span_id"`

	defaultDifficulty uint64
}

// Difficulty returns default span difficulty.
func (i *SpanInfo) Difficulty() uint64 {
	if i.defaultDifficulty == 0 {
		i.defaultDifficulty = uint64(len(i.SelectedProducers))
	}

	return i.defaultDifficulty
}

type spanResponse struct {
	Result *SpanInfo `json:"result"`
}

// Spanner interface for processing span requests.
type Spanner interface {
	Run() error
	GetCurrentSpan() (*SpanInfo, error)
	GetLatestSpan() (*SpanInfo, error)
	GetSpanByID(spanID uint64) (*SpanInfo, error)
	GetSpanForHeight(height uint64) (*SpanInfo, error)
	GetSpanNotificationCh() <-chan struct{}
}

// HeimdallSpanner basic client for processing heimdall spans.
type HeimdallSpanner struct {
	ctx context.Context

	mx *sync.RWMutex

	booting *atomic.Bool
	started *atomic.Bool

	httpClient *http.Client

	spanMap *ordermap.OrderedMap[uint64, *SpanInfo]

	spanUpdateCh chan *SpanInfo
	spanNotifyCh chan struct{}

	endpoint string
}

// NewHeimdallSpanner creates a new HeimdallSpanner.
func NewHeimdallSpanner(ctx context.Context, endpoint string) *HeimdallSpanner {
	return &HeimdallSpanner{
		ctx: ctx,

		mx: new(sync.RWMutex),

		booting: new(atomic.Bool),
		started: new(atomic.Bool),

		httpClient: new(http.Client),

		spanMap: ordermap.New[uint64, *SpanInfo](),

		spanUpdateCh: make(chan *SpanInfo, 2),
		spanNotifyCh: make(chan struct{}, 1),

		endpoint: strings.TrimSuffix(endpoint, "/"),
	}
}

// GetSpanNotificationCh returns notification channel.
func (h *HeimdallSpanner) GetSpanNotificationCh() <-chan struct{} { return h.spanNotifyCh }

func (h *HeimdallSpanner) bootstrap() error {
	err := h.ctx.Err()
	if err != nil {
		return err
	}

	latestSpan, err := h.GetLatestSpan()
	if err != nil {
		return err
	}

	currentSpan, err := h.GetSpanByID(latestSpan.SpanID - 1)
	if err != nil {
		return err
	}

	h.spanMap.Store(currentSpan.SpanID, currentSpan)
	h.spanMap.Store(latestSpan.SpanID, latestSpan)

	return nil
}

// Run bootstrap initial state and start goroutine for processing of changes.
func (h *HeimdallSpanner) Run() error {
	if h.started.Load() || h.booting.Load() {
		return nil
	}

	h.booting.Store(true)
	defer h.booting.Store(false)

	if err := h.bootstrap(); err != nil {
		return errors.WithMessage(err, "failed to bootstrap spanner")
	}

	h.started.Store(true)

	go func() {
		backOff := backoff.WithContext(Retry(), h.ctx)

		for {
			select {
			case <-h.ctx.Done():
				h.started.Store(false)

				return
			case spanInfo := <-h.spanUpdateCh:
				backOff.Reset()
				if err := backoff.RetryNotify(
					func() error { return h.updateSpanMap(spanInfo) },
					backOff,
					func(err error, duration time.Duration) {
						log.Tracef("failed to update span: %v, retry in %s", err, duration.String())
					},
				); err != nil {
					log.Warnf("failed to update span: %v", err)
				}
			}
		}
	}()

	return nil
}

func (h *HeimdallSpanner) updateSpanMap(spanInfo *SpanInfo) error {
	h.mx.RLock()
	_, exists := h.spanMap.Get(spanInfo.SpanID)
	h.mx.RUnlock()

	if exists {
		return nil
	}

	h.mx.Lock()
	defer h.mx.Unlock()

	// cleanup of span map
	for pair := h.spanMap.Oldest(); pair != nil; pair = pair.Next() {
		if pair.Key+2 <= spanInfo.SpanID {
			h.spanMap.Delete(pair.Key)
		}
	}

	defer func() {
		select {
		default:
		case h.spanNotifyCh <- struct{}{}:
		}
	}()

	// adding of new value
	h.spanMap.Store(spanInfo.SpanID, spanInfo)

	prevNewest := h.spanMap.Oldest()
	if prevNewest == nil || prevNewest.Key <= spanInfo.SpanID {
		return nil
	}

	if err := h.spanMap.MoveAfter(prevNewest.Key, spanInfo.SpanID); err != nil {
		return errors.WithMessage(err, "failed to sort spans")
	}

	return nil
}

// GetCurrentSpan returns current cpan.
func (h *HeimdallSpanner) GetCurrentSpan() (*SpanInfo, error) {
	if h.spanMap.Len() != 2 {
		return nil, errCorruptedSpanMap
	}

	return h.spanMap.Oldest().Value, nil
}

// GetLatestSpan returns latest cpan.
func (h *HeimdallSpanner) GetLatestSpan() (*SpanInfo, error) {
	err := h.ctx.Err()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(h.ctx, http.MethodGet, fmt.Sprintf(endpointLatestSpanFmt, h.endpoint), nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create request for fetching of latest span")
	}

	return h.doSpanRequest(req)
}

// GetSpanByID returns cpan by ID.
func (h *HeimdallSpanner) GetSpanByID(spanID uint64) (*SpanInfo, error) {
	err := h.ctx.Err()
	if err != nil {
		return nil, err
	}

	h.mx.RLock()
	spanInfo, exists := h.spanMap.Get(spanID)
	h.mx.RUnlock()

	if exists {
		spanCopy := *spanInfo

		return &spanCopy, nil
	}

	req, err := http.NewRequestWithContext(h.ctx, http.MethodGet, fmt.Sprintf(endpointSpanIDFmt, h.endpoint, spanID), nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create request for fetching of span by id")
	}

	return h.doSpanRequest(req)
}

func (h *HeimdallSpanner) doSpanRequest(req *http.Request) (*SpanInfo, error) {
	if h.endpoint == "" {
		return nil, errBadHeimdallEndpoint
	}

	backOff := backoff.WithContext(Retry(), h.ctx)

	resp, err := backoff.RetryNotifyWithData(
		func() (*http.Response, error) { return h.httpClient.Do(req) },
		backOff,
		func(err error, duration time.Duration) {
			log.Tracef("failed to fetch span: %v, retry in %s", err, duration.String())
		},
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to fetch span")
	}

	defer func() { _ = resp.Body.Close() }()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read response body")
	}

	spanResp := new(spanResponse)

	if err = json.Unmarshal(bytes, spanResp); err != nil {
		return nil, errors.WithMessage(err, "failed to unmarshal response body")
	}

	if spanResp == nil || spanResp.Result == nil {
		return nil, errors.WithMessage(errBadSpanResp, string(bytes))
	}

	// make difficulty cached
	spanResp.Result.Difficulty()

	currentSpan, err := h.GetCurrentSpan()
	if currentSpan == nil || err != nil || currentSpan.SpanID+2 <= spanResp.Result.SpanID {
		spanInfoCopy := *spanResp.Result

		h.spanUpdateCh <- &spanInfoCopy
	}

	return spanResp.Result, nil
}

// GetSpanForHeight returns span for height.
func (h *HeimdallSpanner) GetSpanForHeight(height uint64) (*SpanInfo, error) {
	return h.GetSpanByID(GetSpanIDByHeight(height))
}

// GetSpanIDByHeight returns span id for height.
func GetSpanIDByHeight(height uint64) uint64 {
	if height < 256 {
		return 1
	}

	return ((height - 256) / SpanSize) + 1
}

// SpanStart helper which returns the closest span start for provided blockHeight.
func SpanStart(height uint64) uint64 {
	return ((GetSpanIDByHeight(height) - 1) * SpanSize) + 256
}

// IsSpanStart helper which indicates if provided blockHeight is start of span.
func IsSpanStart(height uint64) bool {
	return (height-256)%SpanSize == 0
}
