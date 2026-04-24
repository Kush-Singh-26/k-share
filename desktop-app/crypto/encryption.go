package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

const (
	ChunkSize = 64 * 1024 // 64KB chunks for streaming
)

// GetEncryptionKey generates the AES key from pairing code using SHA-256
// Replicated from server/internal/bootstrap/bootstrap.go
func GetEncryptionKey(pairingCode string) []byte {
	hash := sha256.Sum256([]byte(pairingCode))
	return hash[:]
}

// EncryptData encrypts small data payloads (for JSON responses, clipboard text, etc.)
// Replicated from server/internal/bootstrap/bootstrap.go
func EncryptData(data []byte, pairingCode string) (string, error) {
	key := GetEncryptionKey(pairingCode)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	io.ReadFull(rand.Reader, nonce)

	timestamp := time.Now().Unix()
	payload := map[string]interface{}{
		"t": timestamp,
		"d": base64.StdEncoding.EncodeToString(data),
	}
	jsonPayload, _ := json.Marshal(payload)
	ciphertext := gcm.Seal(nonce, nonce, jsonPayload, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptData decrypts small data payloads with timestamp validation
// Replicated from server/internal/bootstrap/bootstrap.go
func DecryptData(encryptedStr string, pairingCode string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedStr)
	if err != nil {
		return nil, err
	}
	key := GetEncryptionKey(pairingCode)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, _ := cipher.NewGCM(block)
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	json.Unmarshal(plaintext, &payload)
	if t, ok := payload["t"].(float64); ok {
		if time.Now().Unix()-int64(t) > 600 {
			return nil, fmt.Errorf("message expired")
		}
	}
	dataStr, _ := payload["d"].(string)
	return base64.StdEncoding.DecodeString(dataStr)
}

// EncryptStream encrypts data stream using chunked AES-GCM with custom nonce XOR
// CRITICAL: Uses exact bitwise XOR nonce logic from server for compatibility
// Replicated from server/internal/bootstrap/bootstrap.go
func EncryptStream(dst io.Writer, src io.Reader, pairingCode string) error {
	key := GetEncryptionKey(pairingCode)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	io.ReadFull(rand.Reader, nonce)
	dst.Write(nonce)

	buf := make([]byte, ChunkSize)
	chunkIndex := uint64(0)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			currentNonce := make([]byte, len(nonce))
			copy(currentNonce, nonce)
			// CRITICAL: Custom XOR logic - must match server exactly
			for i := 0; i < 8; i++ {
				currentNonce[i] ^= byte(chunkIndex >> (i * 8))
			}
			encrypted := gcm.Seal(nil, currentNonce, buf[:n], nil)
			// Write chunk size as 4-byte little-endian
			sizeBuf := make([]byte, 4)
			sizeBuf[0], sizeBuf[1], sizeBuf[2], sizeBuf[3] = byte(len(encrypted)), byte(len(encrypted)>>8), byte(len(encrypted)>>16), byte(len(encrypted)>>24)
			dst.Write(sizeBuf)
			dst.Write(encrypted)
			chunkIndex++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// DecryptStream decrypts chunked AES-GCM stream with custom nonce XOR
// Replicated from server/internal/bootstrap/bootstrap.go
func DecryptStream(dst io.Writer, src io.Reader, pairingCode string) error {
	key := GetEncryptionKey(pairingCode)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(src, nonce); err != nil {
		return err
	}
	chunkIndex := uint64(0)
	sizeBuf := make([]byte, 4)
	for {
		_, err := io.ReadFull(src, sizeBuf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Read chunk size (little-endian)
		length := uint32(sizeBuf[0]) | uint32(sizeBuf[1])<<8 | uint32(sizeBuf[2])<<16 | uint32(sizeBuf[3])<<24
		encrypted := make([]byte, length)
		if _, err := io.ReadFull(src, encrypted); err != nil {
			return err
		}
		currentNonce := make([]byte, len(nonce))
		copy(currentNonce, nonce)
		// CRITICAL: Custom XOR logic - must match server exactly
		for i := 0; i < 8; i++ {
			currentNonce[i] ^= byte(chunkIndex >> (i * 8))
		}
		plaintext, err := gcm.Open(nil, currentNonce, encrypted, nil)
		if err != nil {
			return err
		}
		dst.Write(plaintext)
		chunkIndex++
	}
	return nil
}
