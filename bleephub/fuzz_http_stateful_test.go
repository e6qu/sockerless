package bleephub

import "testing"

// FuzzHTTPSequence drives a sequence of 2..8 fuzzed requests against ONE server
// instance, so create→mutate→read chains and cross-request state-machine bugs
// (a create a later read 500s on, an ID reused across resource kinds, a delete
// that corrupts a shared index) are exercised. Every request in the sequence
// must satisfy the same invariants as FuzzHTTPRequest.
//
// A fresh fixture is built per fuzz execution so that a mutating sequence cannot
// permanently poison the shared seed for later inputs (which would turn one real
// bug into a flood of derived failures and destroy corpus determinism).
func FuzzHTTPSequence(f *testing.F) {
	// Build one fixture up front only to validate seeding; each execution gets
	// its own via newFuzzFixture(t).
	_ = newFuzzFixture(f)

	seeds := [][]byte{
		{2, 0, 1, 2, 3},
		{3, 40, 1, 80, 2, 120, 0},
		{4, 10, 0, 20, 1, 30, 2, 40, 3},
		{5, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4},
		{8, 55, 1, 66, 2, 77, 3, 88, 4, 99, 5, 111, 0, 222, 1},
		{2, 200, 0, 4, 1},
		{6, 33, 0, 44, 5, 55, 1, 66, 2, 77, 3, 88, 4},
		{2, 5, 5, 5, 5, 5, 5, 5, 5},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		fx := newFuzzFixture(t)
		r := &fuzzReader{b: data}
		// First byte selects sequence length in [2,8].
		n := 2 + r.pick(7)
		for k := 0; k < n; k++ {
			// Each step consumes a bounded slice of the remaining bytes so the
			// decode stays deterministic and every step gets distinct input.
			chunk := nextChunk(r)
			req := fx.decodeFuzzRequest(chunk)
			serveAndCheck(t, fx.handler, req)
		}
	})
}

// nextChunk deterministically peels a length-prefixed slice off the reader for
// one request in a sequence. When bytes run out it yields empty chunks (which
// decode to a stable default request).
func nextChunk(r *fuzzReader) []byte {
	// Length byte gives 1..24 bytes of payload for this step.
	l := 1 + r.pick(24)
	out := make([]byte, 0, l)
	for i := 0; i < l; i++ {
		out = append(out, byte(r.u8()))
	}
	return out
}
