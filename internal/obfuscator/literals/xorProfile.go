package literals

import (
	"fmt"
	"math/rand"
	"time"
)

// XorObfuscateLiteralsProfile encrypts string literals by XOR-ing every byte
// with a single random key byte. This is the "easy" profile — fast and
// sufficient for light obfuscation where AES overhead is undesirable.
type XorObfuscateLiteralsProfile struct{}

// GenerateKey derives a single XOR key byte from seed. When seed is 0, the
// current Unix nanosecond timestamp is used instead.
func (p *XorObfuscateLiteralsProfile) GenerateKey(seed int64) interface{} {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	localRand := rand.New(rand.NewSource(seed))
	return byte(localRand.Intn(256))
}

// DecryptFunction returns the source of a Go function named funcName that
// XOR-decrypts a string produced by EncryptString using the embedded key byte.
func (p *XorObfuscateLiteralsProfile) DecryptFunction(key interface{}, funcName string) string {
	k := key.(byte)
	return fmt.Sprintf(`
        func %s(s string) string {
            b := []byte(s)
            for i := range b {
                b[i] ^= %d
            }
            return string(b)
        }
    `, funcName, k)
}

// EncryptString XOR-encrypts every byte of content with the key byte from key.
func (p *XorObfuscateLiteralsProfile) EncryptString(content string, key interface{}) string {
	k := key.(byte)
	b := []byte(content)
	for i := range b {
		b[i] ^= k
	}
	return string(b)
}

// RequiredImports returns nil because the XOR decrypt function needs no
// additional imports.
func (p *XorObfuscateLiteralsProfile) RequiredImports() []string {
	return nil
}
