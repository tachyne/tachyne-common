package protocol

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"io"
)

// Packet is a single decoded, uncompressed packet frame.
type Packet struct {
	ID   int32  // packet ID
	Data []byte // payload following the ID
}

// Body returns a reader over the packet payload.
func (p *Packet) Body() *bytes.Reader { return bytes.NewReader(p.Data) }

// ReadPacket reads one uncompressed frame: VarInt length, then that many bytes
// of (VarInt packet ID + payload). Compression/encryption are layered on later.
func ReadPacket(r *bufio.Reader) (*Packet, error) {
	length, err := ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if length < 0 {
		return nil, io.ErrUnexpectedEOF
	}
	frame := make([]byte, length)
	if _, err := io.ReadFull(r, frame); err != nil {
		return nil, err
	}
	br := bytes.NewReader(frame)
	id, err := ReadVarInt(br)
	if err != nil {
		return nil, err
	}
	data := make([]byte, br.Len())
	if _, err := io.ReadFull(br, data); err != nil {
		return nil, err
	}
	return &Packet{ID: id, Data: data}, nil
}

// WritePacket writes one uncompressed frame for the given ID and payload.
func WritePacket(w io.Writer, id int32, data []byte) error {
	body := AppendVarInt(nil, id)
	body = append(body, data...)

	frame := AppendVarInt(nil, int32(len(body)))
	frame = append(frame, body...)

	_, err := w.Write(frame)
	return err
}

// WriteCompressed writes one frame in the post-Set-Compression format:
//
//	VarInt  packet length (of everything after it)
//	VarInt  data length — uncompressed size of (id+data), or 0 if not compressed
//	bytes   zlib(id+data) when compressed, else raw id+data
//
// Bodies (id+data) at least `threshold` bytes are zlib-compressed; smaller ones
// are sent raw with a 0 data-length marker, so we never spend CPU deflating tiny
// packets (which would often grow them).
func WriteCompressed(w io.Writer, id int32, data []byte, threshold int) error {
	body := AppendVarInt(nil, id)
	body = append(body, data...)

	var packet []byte
	if len(body) >= threshold {
		var buf bytes.Buffer
		// BestSpeed: chunk/light data is highly redundant, so level 1 still gets
		// a huge ratio while spending far less CPU than the default level.
		zw, _ := zlib.NewWriterLevel(&buf, zlib.BestSpeed)
		if _, err := zw.Write(body); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		packet = AppendVarInt(nil, int32(len(body))) // data length = uncompressed size
		packet = append(packet, buf.Bytes()...)
	} else {
		packet = AppendVarInt(nil, 0) // 0 = payload is uncompressed
		packet = append(packet, body...)
	}

	frame := AppendVarInt(nil, int32(len(packet)))
	frame = append(frame, packet...)
	_, err := w.Write(frame)
	return err
}

// ReadCompressed reads one frame in the post-Set-Compression format (see
// WriteCompressed). A data length of 0 means the payload is uncompressed.
func ReadCompressed(r *bufio.Reader) (*Packet, error) {
	packetLen, err := ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if packetLen < 0 {
		return nil, io.ErrUnexpectedEOF
	}
	raw := make([]byte, packetLen)
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, err
	}
	pr := bytes.NewReader(raw)

	dataLen, err := ReadVarInt(pr)
	if err != nil {
		return nil, err
	}

	var body []byte
	if dataLen == 0 {
		body = make([]byte, pr.Len())
		if _, err := io.ReadFull(pr, body); err != nil {
			return nil, err
		}
	} else {
		zr, err := zlib.NewReader(pr)
		if err != nil {
			return nil, err
		}
		body = make([]byte, dataLen)
		if _, err := io.ReadFull(zr, body); err != nil {
			zr.Close()
			return nil, err
		}
		zr.Close()
	}

	bb := bytes.NewReader(body)
	id, err := ReadVarInt(bb)
	if err != nil {
		return nil, err
	}
	payload := make([]byte, bb.Len())
	if _, err := io.ReadFull(bb, payload); err != nil {
		return nil, err
	}
	return &Packet{ID: id, Data: payload}, nil
}
