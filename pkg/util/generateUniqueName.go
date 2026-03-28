package util

import (
	"math/rand"
	"time"
)

// NewRand creates a *rand.Rand seeded with seed.
// If seed is 0 a unique seed derived from the current time is used.
// The returned instance must be reused across all GenerateUniqueName calls
// within the same obfuscation pass — never create a new one per call,
// because on Windows the system timer resolution can be ~15 ms, causing
// multiple rapid time.Now().UnixNano() calls to return the same value and
// therefore produce identical names.
func NewRand(seed int64) *rand.Rand {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return rand.New(rand.NewSource(seed))
}

// GenerateUniqueName returns a random identifier of length [16, 32) using r.
// r must be initialised once (via NewRand) and reused for every call in the
// same pass to guarantee distinct names.
// used is a set of already-allocated names; the function retries until it
// produces a name not present in the set, then records it. This prevents
// collisions between variables that end up in the same scope after CFF hoisting
// or between modules that share the same PRNG with a fixed seed.
func GenerateUniqueName(r *rand.Rand, used map[string]struct{}) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for {
		b := make([]byte, r.Intn(16)+16)
		for i := range b {
			b[i] = alphabet[r.Intn(len(alphabet))]
		}
		name := string(b)
		if _, exists := used[name]; !exists {
			used[name] = struct{}{}
			return name
		}
	}
}
