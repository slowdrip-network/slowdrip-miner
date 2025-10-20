// internal/wallet/keystore.go
package wallet

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts"
	gethks "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
)

// Keystore holds a single secp256k1 key for signing EVM digests (EIP-191 / EIP-712 / raw 32B).
// It is intentionally minimal: no RPC, no tx assembly — just keys & signatures.
type Keystore struct {
	mu      sync.RWMutex
	priv    *ecdsa.PrivateKey
	addr    common.Address
	chainID *big.Int
}

// NewRandom creates a new random key for the given chainID (nil allowed).
func NewRandom(chainID *big.Int) (*Keystore, error) {
	priv, err := ecdsa.GenerateKey(gethcrypto.S256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("wallet: generate key: %w", err)
	}
	addr := gethcrypto.PubkeyToAddress(priv.PublicKey)
	return &Keystore{priv: priv, addr: addr, chainID: copyBig(chainID)}, nil
}

// FromHex creates a keystore from a 32-byte hex private key (with or without 0x prefix).
func FromHex(hexKey string, chainID *big.Int) (*Keystore, error) {
	hexKey = strings.TrimPrefix(strings.TrimSpace(hexKey), "0x")
	if len(hexKey) != 64 {
		return nil, errors.New("wallet: expected 32-byte hex private key")
	}
	b, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("wallet: bad hex: %w", err)
	}
	priv, err := gethcrypto.ToECDSA(b)
	if err != nil {
		return nil, fmt.Errorf("wallet: to ecdsa: %w", err)
	}
	addr := gethcrypto.PubkeyToAddress(priv.PublicKey)
	return &Keystore{priv: priv, addr: addr, chainID: copyBig(chainID)}, nil
}

// FromKeystoreJSON decrypts a Web3 keystore JSON (V3) blob with password.
func FromKeystoreJSON(jsonBlob []byte, password string, chainID *big.Int) (*Keystore, error) {
	key, err := gethks.DecryptKey(jsonBlob, password)
	if err != nil {
		return nil, fmt.Errorf("wallet: decrypt keystore: %w", err)
	}
	return &Keystore{priv: key.PrivateKey, addr: key.Address, chainID: copyBig(chainID)}, nil
}

// SaveAsKeystore writes the private key as a Web3 JSON keystore file.
// Useful for dev/ops; not required for runtime.
func (w *Keystore) SaveAsKeystore(dir, password string) (string, error) {
	if w == nil || w.priv == nil {
		return "", errors.New("wallet: nil")
	}
	ks := gethks.NewKeyStore(dir, gethks.StandardScryptN, gethks.StandardScryptP)
	acc, err := ks.ImportECDSA(w.priv, password)
	if err != nil {
		return "", fmt.Errorf("wallet: import ecdsa: %w", err)
	}
	// Unlock once to ensure it’s usable (no-op for file import)
	if err := ks.Unlock(acc, password); err != nil {
		return "", fmt.Errorf("wallet: unlock: %w", err)
	}
	// Return the path we expect it to be saved at (best-effort)
	path := filepath.Join(dir, accounts.KeyFilename(acc.Address))
	return path, nil
}

// Address returns the EVM address for this key.
func (w *Keystore) Address() common.Address {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.addr
}

// ChainID returns the configured chain ID (may be nil if unset).
func (w *Keystore) ChainID() *big.Int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return copyBig(w.chainID)
}

// SetChainID sets or updates the chain ID.
func (w *Keystore) SetChainID(id *big.Int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.chainID = copyBig(id)
}

// PrivateKeyHex returns the private key hex WITHOUT 0x, for dev only.
// WARNING: avoid using this in production logs!
func (w *Keystore) PrivateKeyHex() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.priv == nil {
		return ""
	}
	return hex.EncodeToString(gethcrypto.FromECDSA(w.priv))
}

// Close best-effort wipes private key bytes from memory.
func (w *Keystore) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.priv != nil {
		b := gethcrypto.FromECDSA(w.priv)
		for i := range b {
			b[i] = 0
		}
		// Overwrite fields (best-effort; not guaranteed)
		w.priv.D.SetInt64(0)
	}
	w.priv = nil
}

// --------------------------
// Signing helpers
// --------------------------

// SignHash signs a 32-byte digest (Keccak256 hash or domain-separated EIP-712 digest).
// Returns the 65-byte ECDSA signature with recovery id (V) in {27,28}.
func (w *Keystore) SignHash(digest32 []byte) ([]byte, error) {
	if len(digest32) != 32 {
		return nil, errors.New("wallet: SignHash expects 32-byte digest")
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.priv == nil {
		return nil, errors.New("wallet: closed")
	}
	sig, err := gethcrypto.Sign(digest32, w.priv)
	if err != nil {
		return nil, fmt.Errorf("wallet: sign: %w", err)
	}
	// gethcrypto.Sign returns [R || S || V] with V in {0,1}. Convert to {27,28}.
	sig[64] += 27
	return sig, nil
}

// SignEIP191 signs a human-readable message per EIP-191 ("personal_sign").
// digest = keccak256("\x19Ethereum Signed Message:\n" + len(msg) + msg)
func (w *Keystore) SignEIP191(msg []byte) ([]byte, error) {
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(msg))
	digest := gethcrypto.Keccak256([]byte(prefix), msg)
	return w.SignHash(digest)
}

// SignEIP712Digest signs a prebuilt EIP-712 digest (already domain-separated and hashed).
// You are responsible for hashing TypedData per EIP-712 off-chain and passing the 32-byte digest here.
func (w *Keystore) SignEIP712Digest(digest32 []byte) ([]byte, error) {
	return w.SignHash(digest32)
}

// VerifySig checks a signature produced by SignHash/SignEIP191/SignEIP712Digest against the wallet address.
// Accepts sig with V in {27,28} or {0,1}.
func (w *Keystore) VerifySig(digest32, sig []byte) (bool, error) {
	if len(digest32) != 32 {
		return false, errors.New("wallet: VerifySig expects 32-byte digest")
	}
	if len(sig) != 65 {
		return false, errors.New("wallet: signature must be 65 bytes")
	}
	vsig := make([]byte, 65)
	copy(vsig, sig)
	if vsig[64] >= 27 {
		vsig[64] -= 27
	}
	pubkey, err := gethcrypto.SigToPub(digest32, vsig)
	if err != nil {
		return false, fmt.Errorf("wallet: recover pub: %w", err)
	}
	recAddr := gethcrypto.PubkeyToAddress(*pubkey)
	return strings.EqualFold(recAddr.Hex(), w.Address().Hex()), nil
}

// --------------------------
// Utilities
// --------------------------

// LoadHexFromEnv tries to construct a wallet from ENV var (e.g., SLOWDRIP_MINER_KEY).
// If the var is empty and allowGenerate is true, it generates a random key and stores it at writePath (keystore JSON).
func LoadHexFromEnv(envName string, chainID *big.Int, allowGenerate bool, writeKeystorePath string, keystorePass string) (*Keystore, error) {
	if hexKey := strings.TrimSpace(os.Getenv(envName)); hexKey != "" {
		return FromHex(hexKey, chainID)
	}
	if !allowGenerate {
		return nil, fmt.Errorf("wallet: env %s not set and generation disabled", envName)
	}
	w, err := NewRandom(chainID)
	if err != nil {
		return nil, err
	}
	if writeKeystorePath != "" {
		dir := filepath.Dir(writeKeystorePath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("wallet: mkdir: %w", err)
		}
		if _, err := w.SaveAsKeystore(dir, keystorePass); err != nil {
			return nil, fmt.Errorf("wallet: save keystore: %w", err)
		}
	}
	return w, nil
}

// copyBig returns a deep copy of big.Int (or nil).
func copyBig(v *big.Int) *big.Int {
	if v == nil {
		return nil
	}
	out := new(big.Int).Set(v)
	return out
}
