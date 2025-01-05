package network

import (
	"context"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/logging"
)

// CreateTracer creates a ConnectionTracer that updates only the essential metrics.
// We omit all the optional debug/log calls to reduce overhead.
func CreateTracer(
	ctx context.Context,
	perspective logging.Perspective,
	connID quic.ConnectionID,
	bwTracer *BandwidthTracer, // pass in our BandwidthTracer
) *logging.ConnectionTracer {
	return &logging.ConnectionTracer{
		UpdatedMetrics: func(rttStats *logging.RTTStats, cwnd, bytesInFlight logging.ByteCount, packetsInFlight int) {
			// Forward to our bandwidth tracer
			bwTracer.UpdateConnectionStats(rttStats, cwnd, bytesInFlight, packetsInFlight)
		},
		// We omit all other callbacks (AcknowledgedPacket, LostPacket, etc.)
		// or leave them as no-ops to minimize overhead.
	}
}

// BandwidthTracer holds just enough metrics for our adaptive chunk sizing.
type BandwidthTracer struct {
	mu            sync.RWMutex
	rtt           *logging.RTTStats
	cwnd          logging.ByteCount
	bytesInFlight logging.ByteCount

	// We'll keep track of our chunk size here (float64 so we can do multiplicative updates).
	currentChunkSize float64

	// Bounds for chunk size
	minChunkSize int
	maxChunkSize int
}

// NewBandwidthTracer creates a tracer with sensible default bounds and initial chunk size.
func NewBandwidthTracer() *BandwidthTracer {
	return &BandwidthTracer{
		currentChunkSize: 1.0 * 1024.0 * 1024.0, // 1 MB initial
		minChunkSize:     64 * 1024,             // 64 KB
		maxChunkSize:     8 * 1024 * 1024,       // 8 MB
	}
}

// UpdateConnectionStats is invoked from the QUIC tracer to update RTT/cwnd/bytesInFlight.
func (t *BandwidthTracer) UpdateConnectionStats(
	rttStats *logging.RTTStats,
	cwnd, bytesInFlight logging.ByteCount,
	packetsInFlight int,
) {
	t.mu.Lock()
	t.rtt = rttStats
	t.cwnd = cwnd
	t.bytesInFlight = bytesInFlight
	t.mu.Unlock()
}

// GetChunkSize performs a lightweight adaptive chunk sizing with minimal overhead.
// Basic approach:
//   - If ratio > 1 => halving chunk size
//   - Else if RTT > 200ms => reduce chunk size 20%
//   - Else if ratio < 0.8 && RTT < 50ms => increase chunk size 10%
//   - Then clamp to min/max
func (t *BandwidthTracer) GetChunkSize() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	// If no stats yet, return current chunk size
	if t.rtt == nil {
		return int(t.currentChunkSize)
	}

	smoothed := t.rtt.SmoothedRTT()
	ratio := float64(t.bytesInFlight) / float64(t.cwnd)

	// 1) Congestion signals => reduce chunk size
	if ratio > 1.0 {
		t.currentChunkSize *= 0.5
	} else if smoothed > 200*time.Millisecond {
		t.currentChunkSize *= 0.8
	}

	// 2) Good signals => increase chunk size
	if ratio < 0.8 && smoothed < 50*time.Millisecond {
		t.currentChunkSize *= 1.10
	}

	// 3) Clamp the result
	if t.currentChunkSize < float64(t.minChunkSize) {
		t.currentChunkSize = float64(t.minChunkSize)
	} else if t.currentChunkSize > float64(t.maxChunkSize) {
		t.currentChunkSize = float64(t.maxChunkSize)
	}

	return int(t.currentChunkSize)
}
