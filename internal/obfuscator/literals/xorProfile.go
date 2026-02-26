package literals

import (
	"fmt"
	"math/rand"
	"time"
)

type XorObfuscateLiteralsProfile struct{}

func (p *XorObfuscateLiteralsProfile) GenerateKey(seed int64) interface{} {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	localRand := rand.New(rand.NewSource(seed))
	return byte(localRand.Intn(256))
}

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

func (p *XorObfuscateLiteralsProfile) EncryptString(content string, key interface{}) string {
	k := key.(byte)
	b := []byte(content)
	for i := range b {
		b[i] ^= k
	}
	return string(b)
}

func (p *XorObfuscateLiteralsProfile) RequiredImports() []string {
	return nil
}
