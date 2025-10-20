// internal/receipts/signer.go
package receipts

import (
	"context"
	cryptoRand "crypto/rand"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"slowdrip-miner/internal/service"
)

// Versioned receipt format (bump if you change layout)
const ReceiptVersion uint8 = 1

// Domain separation tag to avoid cross-protocol replay.
// You can make this configurable if you run testnets.
const DomainTag = "SlowDrip:PoS-Receipt:v1"

// Receipt is the minimal client-acknowledged unit of useful work.
// It intentionally mirrors fields from service.SegmentReceipt.
// All times are encoded as UnixNano for canonical hashing.
type Receipt struct {
	Version  uint8     `json:"v"`
	Path     string    `json:"path"`
	Seq      uint64    `json:"seq"`
	Size     int64     `json:"size"`
	Deadline int64     `json:"deadline_unixnano"`
	Recv     int64     `json:"recv_unixnano"`
	Commit   [32]byte  `json:"commit"`   // integrity commit for the segment/payload (e.g., H(payload/FEC))
	Nonce    uint64    `json:"nonce"`    // anti-replay within a session
	PubKey   []byte    `json:"pubkey"`   // ed25519 public key (ephemeral session key)
	Sig      []byte    `json:"sig"`      // ed25519 signature over digest
}

// SessionSigner holds an ephemeral keypair used to sign receipts
// for a single streaming session (or short time window).
type SessionSigner struct {
	priv      ed25519.PrivateKey
	pub       ed25519.PublicKey
	sessionID string
	started   time.Time
}

// NewSessionSigner creates a fresh ephemeral keypair.
// sessionID is advisory (used only for logging/ops).
func NewSessionSigner(sessionID string) (*SessionSigner, error) {
	pub, priv, err := ed25519.GenerateKey(cryptoRand.Reader)
	if err != nil {
		return nil, err
	}
	return &SessionSigner{
		priv:      priv,
		pub:       pub,
		sessionID: sessionID,
		started:   time.Now(),
	}, nil
}

// PublicKey returns the raw ed25519 public key bytes.
func (s *SessionSigner) PublicKey() []byte { return append([]byte(nil), s.pub...) }

// Close wipes private key material in memory (best-effort).
func (s *SessionSigner) Close() {
	for i := range s.pr iv {
		s.priv[i] = 0
	}
}

// FromSegment converts a SegmentReceipt (miner-side observation) into a signable Receipt.
// The Nonce lets you disambiguate multiple receipts with the same seq in rare replays.
func FromSegment(sr service.SegmentReceipt, nonce uint64) Receipt {
	return Receipt{
		Version:  ReceiptVersion,
		Path:     sr.Path,
		Seq:      sr.Seq,
		Size:     sr.Size,
		Deadline: sr.Deadline.UnixNano(),
		Recv:     sr.Recv.UnixNano(),
		Commit:   sr.Commit,
		Nonce:    nonce,
	}
}

// Sign signs the receipt in-place using the session's ephemeral key.
// It fills PubKey and Sig fields. It does not perform client-side authorization;
// this is a miner-side attestation of "accepted, on-time delivery".
func (s *SessionSigner) Sign(r *Receipt) error {
	if s == nil || s.priv == nil || s.pub == nil {
		return errors.New("signer not initialized")
	}
	d := digest(*r)
	r.PubKey = append(r.PubKey[:0], s.pub...)
	sig := ed25519.Sign(s.priv, d[:])
	r.Sig = append(r.Sig[:0], sig...)
	return nil
}

// Verify checks the signature against the receipt contents and pubkey.
// Returns nil if valid, or an error describing the failure.
func Verify(r Receipt) error {
	if len(r.PubKey) != ed25519.PublicKeySize {
		return errors.New("invalid pubkey length")
	}
	if len(r.Sig) != ed25519.SignatureSize {
		return errors.New("invalid signature length")
	}
	d := digest(r)
	if !ed25519.Verify(ed25519.PublicKey(r.PubKey), d[:], r.Sig) {
		return errors.New("bad signature")
	}
	return nil
}

// digest computes a canonical domain-separated hash of the receipt fields.
// Layout: H( DomainTag || v || L(path)||path || seq || size || deadline || recv || commit || nonce )
func digest(r Receipt) [32]byte {
	h := sha256.New()

	// Domain separation
	h.Write([]byte(DomainTag))

	// Version
	h.Write([]byte{r.Version})

	// Path with length prefix (uint16)
	pathBytes := []byte(r.Path)
	var lb [2]byte
	binary.BigEndian.PutUint16(lb[:], uint16(len(pathBytes)))
	h.Write(lb[:])
	h.Write(pathBytes)

	// seq (u64), size (i64), deadline (i64), recv (i64), nonce (u64)
	var b8 [8]byte
	putU64(b8[:], r.Seq)
	h.Write(b8[:])

	putI64(b8[:], r.Size)
	h.Write(b8[:])

	putI64(b8[:], r.Deadline)
	h.Write(b8[:])

	putI64(b8[:], r.Recv)
	h.Write(b8[:])

	// commit
	h.Write(r.Commit[:])

	putU64(b8[:], r.Nonce)
	h.Write(b8[:])

	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

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

func putI64(b []byte, v int64) {
	putU64(b, uint64(v))
}

// ------------------------
// Batching / Anchoring
// ------------------------

// MerkleLeaf computes the leaf hash used in batch anchors.
// Layout: H( "leaf" || digest(receipt) )
func MerkleLeaf(r Receipt) [32]byte {
	d := digest(r)
	h := sha256.New()
	h.Write([]byte("leaf"))
	h.Write(d[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// MerkleRoot builds a simple binary Merkle root from leaves.
// If receipts is empty, returns zero hash.
func MerkleRoot(receipts []Receipt) [32]byte {
	if len(receipts) == 0 {
		return [32]byte{}
	}
	leaves := make([][32]byte, len(receipts))
	for i := range receipts {
		leaves[i] = MerkleLeaf(receipts[i])
	}
	return merkleize(leaves)
}

func merkleize(nodes [][32]byte) [32]byte {
	if len(nodes) == 1 {
		return nodes[0]
	}
	// If odd, duplicate last (Bitcoin-style)
	if len(nodes)%2 == 1 {
		nodes = append(nodes, nodes[len(nodes)-1])
	}
	next := make([][32]byte, 0, len(nodes)/2)
	for i := 0; i < len(nodes); i += 2 {
		h := sha256.New()
		h.Write(nodes[i][:])
		h.Write(nodes[i+1][:])
		var out [32]byte
		copy(out[:], h.Sum(nil))
		next = append(next, out)
	}
	return merkleize(next)
}

// ------------------------
// Helper utilities
// ------------------------

// BuildAndSign takes a SegmentReceipt and returns a signed Receipt.
func BuildAndSign(s *SessionSigner, sr service.SegmentReceipt, nonce uint64) (Receipt, error) {
	r := FromSegment(sr, nonce)
	return signAndReturn(s, r)
}

func signAndReturn(s *SessionSigner, r Receipt) (Receipt, error) {
	if err := s.Sign(&r); err != nil {
		return Receipt{}, err
	}
	return r, nil
}

// AggregateAnchor creates a deterministic "anchor" over a set of receipts.
// Sorts by (Path, Seq) to avoid order ambiguity, then returns hex(root).
func AggregateAnchor(rs []Receipt) string {
	if len(rs) == 0 {
		return ""
	}
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].Path == rs[j].Path {
			return rs[i].Seq < rs[j].Seq
		}
		return rs[i].Path < rs[j].Path
	})
	root := MerkleRoot(rs)
	return hex.EncodeToString(root[:])
}

// ------------------------
// Example wiring (optional)
// ------------------------

// Pump is a demonstration of how you might continuously sign receipts coming
// from a channel. You can call this from your service agent in a goroutine.
func Pump(ctx context.Context, s *SessionSigner, in <-chan service.SegmentReceipt, out chan<- Receipt) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sr := <-in:
			r := FromSegment(sr, uint64(time.Now().UnixNano()))
			if err := s.Sign(&r); err != nil {
				return err
			}
			out <- r
		}
	}
}
