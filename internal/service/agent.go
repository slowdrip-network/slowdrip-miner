// internal/service/agent.go
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// SegmentReceipt is a minimal placeholder for a per-segment "useful work" unit.
// In the real PoS pipeline, the client (viewer) signs acceptance over something like:
// (seg_id, size, deadline, recv_time, seg_commit). Here we just model it.
type SegmentReceipt struct {
	Path     string        // stream path (e.g., live/stream)
	Seq      uint64        // monotonically increasing segment/frame index (caller-assigned)
	Size     int64         // bytes delivered for this segment
	Deadline time.Time     // delivery deadline
	Recv     time.Time     // when we actually delivered
	Commit   [32]byte      // integrity commit placeholder (e.g., H(payload or FEC))
	Meta     time.Duration // optional extra: observed jitter or render margin
}

// streamStats aggregates basic QoS stats per path.
type streamStats struct {
	Accepted int64  // on-time segments
	Late     int64  // late segments
	Bytes    int64  // accepted bytes (on-time only)
	LastSeq  uint64 // highest seen seq (for sanity/logging)
	// Rolling hash of accepted receipts as a cheap "root" placeholder
	rolling [32]byte
	// Buffer some recent accepted receipts to seed future Merkle construction if needed
	recentCommits [][32]byte
}

// Agent is the PoS stub that collects receipts and periodically logs aggregates.
type Agent struct {
	log zerolog.Logger

	mu        sync.Mutex
	perPath   map[string]*streamStats
	lastFlush time.Time

	// config knobs (could be loaded from miner.yaml later)
	flushInterval time.Duration
	maxRecent     int // number of recent commits to keep per path
}

// defaultAgent provides a simple singleton for early wiring.
// You can also construct your own via New and pass it around explicitly.
var defaultAgent *Agent
var once sync.Once

// New creates a Service Agent with sane defaults.
func New(log zerolog.Logger) *Agent {
	return &Agent{
		log:           log.With().Str("module", "service").Logger(),
		perPath:       make(map[string]*streamStats),
		flushInterval: 10 * time.Second,
		maxRecent:     64,
	}
}

// Start runs a default singleton agent with a periodic flush loop.
// Safe to call in a goroutine: go service.Start(ctx, log)
func Start(ctx context.Context, log zerolog.Logger) {
	once.Do(func() {
		defaultAgent = New(log)
		defaultAgent.lastFlush = time.Now()
	})
	defaultAgent.log.Info().Msg("service agent: started (stub)")
	t := time.NewTicker(defaultAgent.flushInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			defaultAgent.log.Info().Msg("service agent: stopping")
			return
		case <-t.C:
			defaultAgent.flush(ctx)
		}
	}
}

// AddReceipt records a single segment receipt (call from your watcher or player callbacks).
// It classifies on-time vs late using r.Recv <= r.Deadline.
func AddReceipt(r SegmentReceipt) {
	if defaultAgent == nil {
		// In early bring-up, Start() may not be running; drop safely.
		return
	}
	defaultAgent.add(r)
}

// ---- internals ----

func (a *Agent) add(r SegmentReceipt) {
	onTime := !r.Recv.After(r.Deadline)

	a.mu.Lock()
	defer a.mu.Unlock()

	st := a.perPath[r.Path]
	if st == nil {
		st = &streamStats{}
		a.perPath[r.Path] = st
	}

	if r.Seq > st.LastSeq {
		st.LastSeq = r.Seq
	}

	// Update counters
	if onTime {
		st.Accepted++
		st.Bytes += r.Size

		// Update rolling hash as a cheap aggregate "root" placeholder.
		// rolling = H(rolling || Commit || Seq || Size || Deadline || Recv)
		h := sha256.New()
		h.Write(st.rolling[:])
		h.Write(r.Commit[:])

		var seqBuf [8]byte
		putU64(seqBuf[:], r.Seq)
		h.Write(seqBuf[:])

		var sizeBuf [8]byte
		putU64(sizeBuf[:], uint64(r.Size))
		h.Write(sizeBuf[:])

		putU64(seqBuf[:], uint64(r.Deadline.UnixNano()))
		h.Write(seqBuf[:])

		putU64(seqBuf[:], uint64(r.Recv.UnixNano()))
		h.Write(seqBuf[:])

		copy(st.rolling[:], h.Sum(nil))

		// Keep a small window of commits for future Merkle batching
		st.recentCommits = append(st.recentCommits, r.Commit)
		if len(st.recentCommits) > a.maxRecent {
			st.recentCommits = st.recentCommits[len(st.recentCommits)-a.maxRecent:]
		}
	} else {
		st.Late++
	}
}

// flush logs a compact snapshot of per-path stats and a global hash "anchor".
func (a *Agent) flush(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.perPath) == 0 {
		a.log.Debug().Msg("service: no paths yet")
		return
	}

	// Deterministic ordering for a global anchor
	keys := make([]string, 0, len(a.perPath))
	for k := range a.perPath {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	global := sha256.New()
	for _, p := range keys {
		st := a.perPath[p]
		// Path-specific anchor
		pathAnchor := hex.EncodeToString(st.rolling[:])
		a.log.Info().
			Str("path", p).
			Uint64("last_seq", st.LastSeq).
			Int64("accepted", st.Accepted).
			Int64("late", st.Late).
			Int64("bytes", st.Bytes).
			Str("anchor", pathAnchor).
			Msg("service: qos window")

		// Mix into global anchor
		global.Write([]byte(p))
		global.Write(st.rolling[:])
	}

	ga := hex.EncodeToString(global.Sum(nil))
	a.log.Info().Str("global_anchor", ga).Msg("service: aggregate anchor (placeholder)")

	a.lastFlush = time.Now()
}

// putU64 encodes v into b in big-endian; len(b) must be >= 8
func putU64(b []byte, v uint64) {
	_ = b[:8]
	b[0] = byte(v >> 56)
	b[1] = byte(v >> 48)
	b[2] = byte(v >> 40)
	b[3] = byte(v >> 32)
	b[4] = byte(v >> 24)
	b[5] = byte(v >> 16)
	b[6] = byte(v >> 8)
	b[7] = byte(v)
}
