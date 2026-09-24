package protocol

import (
	"bytes"
	"io"
	"testing"
)

// BenchmarkEncryptedWrite measures the gateway's cipher on a join-sized
// stream: 16 KiB writes, as chunk packets go out.
func BenchmarkEncryptedWrite(b *testing.B) {
	secret := []byte("0123456789abcdef")
	var sink bytes.Buffer
	ec, _ := NewEncryptedConn(struct {
		io.Reader
		io.Writer
	}{nil, &sink}, secret)
	buf := make([]byte, 16<<10)
	b.SetBytes(int64(len(buf)))
	for i := 0; i < b.N; i++ {
		sink.Reset()
		ec.Write(buf)
	}
}
