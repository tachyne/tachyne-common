package protocol

import (
	"bytes"
	"crypto/aes"
	"testing"
)

// The canonical wiki.vg auth-digest test vectors.
func TestAuthDigestVectors(t *testing.T) {
	cases := map[string]string{
		"Notch": "4ed1f46bbe04bc756bcb17c0c7ce3e4632f06a48",
		"jeb_":  "-7c9d5b0044c130109a5d7b5fb5c317c02b4e28c1",
		"simon": "88e16a1019277b15d58faf0541e11910eb756f6",
	}
	for in, want := range cases {
		if got := AuthDigest(in, nil, nil); got != want {
			t.Errorf("AuthDigest(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCFB8RoundTrip(t *testing.T) {
	secret := []byte("0123456789abcdef")
	block, _ := aes.NewCipher(secret)
	enc := newCFB8(block, secret, false)
	dec := newCFB8(block, secret, true)
	msg := []byte("The quick brown fox jumps over the lazy dog — twice, with unicode ✓")
	ct := make([]byte, len(msg))
	enc.XORKeyStream(ct, msg)
	if bytes.Equal(ct, msg) {
		t.Fatal("ciphertext equals plaintext")
	}
	pt := make([]byte, len(ct))
	// Decrypt in awkward chunk sizes to prove statefulness.
	for i := 0; i < len(ct); {
		n := 1 + (i % 7)
		if i+n > len(ct) {
			n = len(ct) - i
		}
		dec.XORKeyStream(pt[i:i+n], ct[i:i+n])
		i += n
	}
	if !bytes.Equal(pt, msg) {
		t.Fatalf("round trip failed: %q", pt)
	}
}
