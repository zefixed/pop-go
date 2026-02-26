package literals

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

type AESObfuscateLiteralsProfile struct{}

func (p *AESObfuscateLiteralsProfile) GenerateKey(seed int64) interface{} {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	r := rand.New(rand.NewSource(seed))

	// Generate 32-byte AES key and 16-byte IV
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

func (p *AESObfuscateLiteralsProfile) DecryptFunction(key interface{}, funcName string) string {
	k := key.(struct{ Key, IV []byte })

	// Format key/IV as hex literals for embedding
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

// Helper to format byte slices as hex literals
func formatBytes(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("0x%02x", v)
	}
	return strings.Join(parts, ", ")
}

func (p *AESObfuscateLiteralsProfile) EncryptString(content string, key interface{}) string {
	k := key.(struct{ Key, IV []byte })

	block, _ := aes.NewCipher(k.Key)
	stream := cipher.NewCTR(block, k.IV)

	data := []byte(content)
	stream.XORKeyStream(data, data)
	return string(data)
}

func (p *AESObfuscateLiteralsProfile) RequiredImports() []string {
	return []string{"crypto/aes", "crypto/cipher"}
}
