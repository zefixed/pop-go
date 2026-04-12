package literals

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// AESObfuscateLiteralsProfile encrypts string literals using AES-CTR with a
// randomly generated 32-byte key and 16-byte IV. Both values are embedded as
// byte-array literals in the injected decrypt function so that the binary
// carries no additional metadata.
type AESObfuscateLiteralsProfile struct{}

// GenerateKey derives a 32-byte AES key and a 16-byte IV from seed. When seed
// is 0, the current Unix nanosecond timestamp is used instead.
func (p *AESObfuscateLiteralsProfile) GenerateKey(seed int64) interface{} {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	r := rand.New(rand.NewSource(seed))

	key := make([]byte, 32)
	iv := make([]byte, 16)
	for i := range key {
		key[i] = byte(r.Intn(256))
	}
	for i := range iv {
		iv[i] = byte(r.Intn(256))
	}
	return struct{ Key, IV []byte }{key, iv}
}

// DecryptFunction returns the source of a Go function named funcName that
// decrypts a string produced by EncryptString using AES-CTR mode with the
// key and IV embedded as compile-time byte-array literals.
func (p *AESObfuscateLiteralsProfile) DecryptFunction(key interface{}, funcName string) string {
	k := key.(struct{ Key, IV []byte })
	keyStr := formatBytes(k.Key)
	ivStr := formatBytes(k.IV)

	return fmt.Sprintf(`
        func %s(s string) string {
            key := [32]byte{%s}
            iv := [16]byte{%s}
            block, _ := aes.NewCipher(key[:])
            stream := cipher.NewCTR(block, iv[:])
            data := []byte(s)
            stream.XORKeyStream(data, data)
            return string(data)
        }
    `, funcName, keyStr, ivStr)
}

// EncryptString encrypts content with the AES-CTR key and IV from key and
// returns the raw ciphertext bytes as a Go string.
func (p *AESObfuscateLiteralsProfile) EncryptString(content string, key interface{}) string {
	k := key.(struct{ Key, IV []byte })

	block, _ := aes.NewCipher(k.Key)
	stream := cipher.NewCTR(block, k.IV)

	data := []byte(content)
	stream.XORKeyStream(data, data)
	return string(data)
}

// RequiredImports returns the import paths that the AES decrypt function needs.
func (p *AESObfuscateLiteralsProfile) RequiredImports() []string {
	return []string{"crypto/aes", "crypto/cipher"}
}

// formatBytes formats a byte slice as a comma-separated list of 0x-prefixed
// hex literals suitable for embedding in a Go array literal.
func formatBytes(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("0x%02x", v)
	}
	return strings.Join(parts, ", ")
}
