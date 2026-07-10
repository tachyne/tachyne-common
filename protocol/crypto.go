package protocol

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"io"
	"strings"
)

// Minecraft's wire encryption: AES-128 CFB-8 with the shared secret as both
// key and IV, plus the protocol's peculiar "Minecraft SHA-1" server digest
// (two's-complement signed hex). Pure stdlib: the CFB-8 mode (one byte per
// feedback step — crypto/cipher only ships CFB-128) is hand-rolled on the
// AES block primitive.

// cfb8 implements one direction of CFB-8 over any 16-byte block cipher.
type cfb8 struct {
	block   cipher.Block
	iv      [16]byte
	scratch [16]byte
	decrypt bool
}

func newCFB8(block cipher.Block, iv []byte, decrypt bool) *cfb8 {
	c := &cfb8{block: block, decrypt: decrypt}
	copy(c.iv[:], iv)
	return c
}

func (c *cfb8) XORKeyStream(dst, src []byte) {
	for i := range src {
		c.block.Encrypt(c.scratch[:], c.iv[:])
		out := src[i] ^ c.scratch[0]
		dst[i] = out
		fed := out
		if c.decrypt {
			fed = src[i] // ciphertext feeds the register on decrypt
		}
		copy(c.iv[:15], c.iv[1:])
		c.iv[15] = fed
	}
}

// EncryptedConn wraps a stream with Minecraft's AES/CFB8 in both directions.
type EncryptedConn struct {
	r   io.Reader
	w   io.Writer
	dec *cfb8
	enc *cfb8
}

// NewEncryptedConn builds the cipher pair from the shared secret (key == IV).
func NewEncryptedConn(rw io.ReadWriter, secret []byte) (*EncryptedConn, error) {
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, err
	}
	return &EncryptedConn{
		r:   rw,
		w:   rw,
		dec: newCFB8(block, secret, true),
		enc: newCFB8(block, secret, false),
	}, nil
}

func (e *EncryptedConn) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if n > 0 {
		e.dec.XORKeyStream(p[:n], p[:n])
	}
	return n, err
}

func (e *EncryptedConn) Write(p []byte) (int, error) {
	buf := make([]byte, len(p))
	e.enc.XORKeyStream(buf, p)
	return e.w.Write(buf)
}

// AuthDigest computes Minecraft's "server hash" for session auth:
// sha1(serverID + sharedSecret + publicKeyDER) rendered as SIGNED
// two's-complement hex (negative hashes get a minus sign, magnitude flipped).
func AuthDigest(serverID string, sharedSecret, publicKey []byte) string {
	h := sha1.New()
	h.Write([]byte(serverID))
	h.Write(sharedSecret)
	h.Write(publicKey)
	sum := h.Sum(nil)

	negative := sum[0]&0x80 != 0
	if negative { // two's complement negate: invert all, add one
		carry := true
		for i := len(sum) - 1; i >= 0; i-- {
			sum[i] = ^sum[i]
			if carry {
				sum[i]++
				carry = sum[i] == 0
			}
		}
	}
	const hexDigits = "0123456789abcdef"
	var b strings.Builder
	for _, v := range sum {
		b.WriteByte(hexDigits[v>>4])
		b.WriteByte(hexDigits[v&0xf])
	}
	s := strings.TrimLeft(b.String(), "0")
	if s == "" {
		s = "0"
	}
	if negative {
		return "-" + s
	}
	return s
}
