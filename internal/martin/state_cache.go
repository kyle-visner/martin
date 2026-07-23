package martin

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	hostedStateCacheEnvelopeVersion   = 1
	hostedStateMaterializationVersion = 1
)

type hostedStateCache struct {
	dir  string
	path string
	key  [sha256.Size]byte
	aad  []byte
}

type encryptedStateCheckpoint struct {
	Version    int    `json:"version"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

type stateCheckpoint struct {
	MaterializationVersion int   `json:"materialization_version"`
	State                  State `json:"state"`
}

func newHostedStateCache(dir, baseURL, token string) (*hostedStateCache, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create Martin cache base directory: %w", err)
	}
	baseInfo, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("stat Martin cache base directory: %w", err)
	}
	if !baseInfo.IsDir() || baseInfo.Mode()&os.ModeSymlink != 0 {
		return nil, appErr(ErrValidation, "MARTIN_CACHE_DIR must be a real directory, not a symlink")
	}
	if baseInfo.Mode().Perm()&0o022 != 0 {
		return nil, appErr(ErrPermission, "MARTIN_CACHE_DIR must not be writable by group or other users")
	}
	privateDir := filepath.Join(dir, "state")
	if err := os.Mkdir(privateDir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("create Martin cache directory: %w", err)
	}
	info, err := os.Lstat(privateDir)
	if err != nil {
		return nil, fmt.Errorf("stat Martin cache directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, appErr(ErrValidation, "Martin cache state path must be a real directory, not a symlink")
	}
	if err := os.Chmod(privateDir, 0o700); err != nil {
		return nil, fmt.Errorf("secure Martin cache directory: %w", err)
	}
	info, err = os.Lstat(privateDir)
	if err != nil {
		return nil, fmt.Errorf("restat Martin cache directory: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, appErr(ErrPermission, "MARTIN_CACHE_DIR must not be accessible by group or other users")
	}

	key := cacheMAC(token, "martin-hosted-state-key-v1\x00"+baseURL)
	identity := cacheMAC(token, "martin-hosted-state-id-v1\x00"+baseURL)
	name := "hosted-state-v1-" + hex.EncodeToString(identity[:16]) + ".json"
	return &hostedStateCache{
		dir: privateDir, path: filepath.Join(privateDir, name), key: key,
		aad: []byte(fmt.Sprintf("martin-hosted-state-cache:%d:%s", hostedStateCacheEnvelopeVersion, baseURL)),
	}, nil
}

func cacheMAC(token, message string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(message))
	var result [sha256.Size]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func (c *hostedStateCache) Load() (State, bool, error) {
	info, err := os.Lstat(c.path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("stat Martin state checkpoint: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return State{}, false, appErr(ErrPermission, "Martin state checkpoint must be a private regular file")
	}
	raw, err := os.ReadFile(c.path)
	if err != nil {
		return State{}, false, fmt.Errorf("read Martin state checkpoint: %w", err)
	}
	var encrypted encryptedStateCheckpoint
	if err := json.Unmarshal(raw, &encrypted); err != nil || encrypted.Version != hostedStateCacheEnvelopeVersion {
		return c.invalidateCorrupt()
	}
	aead, err := c.aead()
	if err != nil {
		return State{}, false, err
	}
	if len(encrypted.Nonce) != aead.NonceSize() {
		return c.invalidateCorrupt()
	}
	plaintext, err := aead.Open(nil, encrypted.Nonce, encrypted.Ciphertext, c.aad)
	if err != nil {
		return c.invalidateCorrupt()
	}
	var checkpoint stateCheckpoint
	if err := json.Unmarshal(plaintext, &checkpoint); err != nil ||
		checkpoint.MaterializationVersion != hostedStateMaterializationVersion ||
		!validCheckpointState(checkpoint.State) {
		return c.invalidateCorrupt()
	}
	return checkpoint.State, true, nil
}

func (c *hostedStateCache) Save(st State) error {
	if !validCheckpointState(st) {
		return appErr(ErrIntegrity, "refusing to cache an incomplete Martin state")
	}
	plaintext, err := json.Marshal(stateCheckpoint{MaterializationVersion: hostedStateMaterializationVersion, State: st})
	if err != nil {
		return fmt.Errorf("encode Martin state checkpoint: %w", err)
	}
	aead, err := c.aead()
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("create Martin state checkpoint nonce: %w", err)
	}
	raw, err := json.Marshal(encryptedStateCheckpoint{
		Version: hostedStateCacheEnvelopeVersion, Nonce: nonce,
		Ciphertext: aead.Seal(nil, nonce, plaintext, c.aad),
	})
	if err != nil {
		return fmt.Errorf("encode encrypted Martin state checkpoint: %w", err)
	}
	temporary, err := os.CreateTemp(c.dir, ".hosted-state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary Martin state checkpoint: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary Martin state checkpoint: %w", err)
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary Martin state checkpoint: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary Martin state checkpoint: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close Martin state checkpoint: %w", err)
	}
	if err := os.Rename(temporaryPath, c.path); err != nil {
		return fmt.Errorf("replace Martin state checkpoint: %w", err)
	}
	removeTemporary = false
	directory, err := os.Open(c.dir)
	if err != nil {
		return fmt.Errorf("open Martin cache directory for sync: %w", err)
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return fmt.Errorf("sync Martin cache directory: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close Martin cache directory: %w", closeErr)
	}
	return nil
}

func (c *hostedStateCache) Invalidate() error {
	err := os.Remove(c.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("invalidate Martin state checkpoint: %w", err)
	}
	return nil
}

func (c *hostedStateCache) invalidateCorrupt() (State, bool, error) {
	if err := c.Invalidate(); err != nil {
		return State{}, false, err
	}
	return State{}, false, nil
}

func (c *hostedStateCache) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return nil, fmt.Errorf("initialize Martin state checkpoint cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize Martin state checkpoint AEAD: %w", err)
	}
	return aead, nil
}

func validCheckpointState(st State) bool {
	return st.Organizations != nil && st.People != nil && st.Deals != nil && st.Activities != nil &&
		st.Tasks != nil && st.CustomerLinks != nil && st.MagpieRoles != nil && st.MagpieUsers != nil &&
		st.MagpieCustomers != nil && st.MagpieInvoices != nil && st.Imports != nil
}
