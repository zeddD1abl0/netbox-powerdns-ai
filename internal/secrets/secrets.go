// Package secrets seals the secrets that nbpdns stores in its database,
// with envelope encryption (ADR-0041): each value is sealed with its own
// random data key, by AES-256-GCM, and the data key is wrapped by the master
// key, by AES-256-GCM too. Both are ASD-approved. A sealed value names its
// master key, and is bound to its place, such as setting:netbox.token, so
// that it can't be opened anywhere else. Rotating the master key re-wraps
// the data keys, and leaves the values as they are.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// KeySize is a master key's size, and a data key's: 256 bits.
const KeySize = 32

// version is the sealed form's version.
const version = 1

// A MasterKey wraps the data keys. It's database.master_key.
type MasterKey struct {
	key []byte
	id  string
}

// ParseMasterKey reads a master key written in base64, as database.master_key
// holds it: 32 random bytes, such as `openssl rand -base64 32` prints. Its
// error never holds the key.
func ParseMasterKey(s string) (MasterKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return MasterKey{}, errors.New("it isn't base64; make one with openssl rand -base64 32")
	}
	if len(b) != KeySize {
		return MasterKey{}, fmt.Errorf("it's %d bytes, not %d; make one with openssl rand -base64 32", len(b), KeySize)
	}
	return newMasterKey(b), nil
}

// NewMasterKey returns a new random master key.
func NewMasterKey() MasterKey {
	b := make([]byte, KeySize)
	_, _ = rand.Read(b) // crypto/rand never fails
	return newMasterKey(b)
}

func newMasterKey(b []byte) MasterKey {
	sum := sha512.Sum384(append([]byte("nbpdns master key ID\x00"), b...))
	return MasterKey{key: b, id: hex.EncodeToString(sum[:8])}
}

// ID names the key, without giving it away: 16 hex digits of a hash of it.
func (k MasterKey) ID() string { return k.id }

// IsSet reports whether k holds a key.
func (k MasterKey) IsSet() bool { return k.key != nil }

// String returns the key's ID, never the key.
func (k MasterKey) String() string { return "master key " + k.id }

// Base64 returns the key in base64, as database.master_key holds it.
func (k MasterKey) Base64() string { return base64.StdEncoding.EncodeToString(k.key) }

// sealed is a sealed value's stored form, as JSON.
type sealed struct {
	V int `json:"v"`
	// KeyID names the master key that wrapped the data key.
	KeyID string `json:"kid"`
	// DataKey is the wrapped data key, and Value the sealed value, each a
	// nonce followed by the ciphertext, in base64.
	DataKey string `json:"dk"`
	Value   string `json:"ct"`
}

// WrongKeyError is the error for a value sealed under another master key.
type WrongKeyError struct{ Sealed, Have string }

func (e *WrongKeyError) Error() string {
	return fmt.Sprintf("it's sealed with master key %s, but database.master_key is master key %s", e.Sealed, e.Have)
}

// ErrTampered is the error for a sealed value that was changed, or moved
// from its place.
var ErrTampered = errors.New("it doesn't open: it was changed, or moved from where it was sealed")

// Seal seals value for place, such as setting:netbox.token, with a new data
// key that k wraps, and returns its stored form.
func Seal(k MasterKey, place, value string) (string, error) {
	if !k.IsSet() {
		return "", errors.New("there's no master key: set database.master_key")
	}
	dk := make([]byte, KeySize)
	_, _ = rand.Read(dk)
	ct, err := encrypt(dk, []byte(value), aad("value", place))
	if err != nil {
		return "", err
	}
	wrapped, err := encrypt(k.key, dk, aad("data key", place))
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(sealed{V: version, KeyID: k.id, DataKey: wrapped, Value: ct})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Open opens a value that Seal sealed for place. It returns a
// *WrongKeyError if another master key sealed it, and ErrTampered if it was
// changed, or sealed for another place.
func Open(k MasterKey, place, stored string) (string, error) {
	s, dk, err := unwrap(k, place, stored)
	if err != nil {
		return "", err
	}
	value, err := decrypt(dk, s.Value, aad("value", place))
	if err != nil {
		return "", err
	}
	return string(value), nil
}

// Rewrap returns stored, sealed for place, with its data key wrapped by
// next rather than by k, and its value as it was.
func Rewrap(k, next MasterKey, place, stored string) (string, error) {
	s, dk, err := unwrap(k, place, stored)
	if err != nil {
		return "", err
	}
	if s.DataKey, err = encrypt(next.key, dk, aad("data key", place)); err != nil {
		return "", err
	}
	s.KeyID = next.id
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// KeyID returns the ID of the master key that sealed stored.
func KeyID(stored string) (string, error) {
	s, err := parse(stored)
	return s.KeyID, err
}

func parse(stored string) (sealed, error) {
	var s sealed
	if err := json.Unmarshal([]byte(stored), &s); err != nil || s.V != version || s.KeyID == "" {
		return s, errors.New("it isn't a sealed value")
	}
	return s, nil
}

// unwrap returns stored's parts, and its data key.
func unwrap(k MasterKey, place, stored string) (sealed, []byte, error) {
	s, err := parse(stored)
	if err != nil {
		return s, nil, err
	}
	if !k.IsSet() {
		return s, nil, fmt.Errorf("it's sealed with master key %s, and database.master_key isn't set", s.KeyID)
	}
	if s.KeyID != k.id {
		return s, nil, &WrongKeyError{Sealed: s.KeyID, Have: k.id}
	}
	dk, err := decrypt(k.key, s.DataKey, aad("data key", place))
	if err != nil {
		return s, nil, err
	}
	return s, dk, nil
}

// aad binds a ciphertext to what it is and where it belongs.
func aad(what, place string) []byte { return []byte("nbpdns " + what + "\x00" + place) }

// encrypt seals plaintext with key, by AES-256-GCM with a random nonce, and
// returns the nonce and the ciphertext, in base64.
func encrypt(key, plaintext, ad []byte) (string, error) {
	g, err := gcm(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	_, _ = rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, plaintext, ad)), nil
}

func decrypt(key []byte, text string, ad []byte) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, ErrTampered
	}
	g, err := gcm(key)
	if err != nil {
		return nil, err
	}
	if len(b) < g.NonceSize() {
		return nil, ErrTampered
	}
	out, err := g.Open(nil, b[:g.NonceSize()], b[g.NonceSize():], ad)
	if err != nil {
		return nil, ErrTampered
	}
	return out, nil
}

func gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
