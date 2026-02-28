package util

import (
	"math/rand"
	"time"
)

func GenerateUniqueName(seed int64) string {
	alphabet := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	localRand := rand.New(rand.NewSource(seed))

	// Generate random func name of length [16, 32)
	b := make([]byte, localRand.Intn(16)+16)
	for i := range b {
		b[i] = alphabet[localRand.Intn(len(alphabet))]
	}
	return string(b)
}
