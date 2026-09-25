package attach

import (
	"encoding/json"
	"io"
)

// MsgBundle (w→gw): frames that belong together — an entity's spawn and the
// state that rides it (metadata, equipment, attributes, passengers). A Java
// gateway sends them between two bundle_delimiter packets, so the client
// applies them in one go and never draws a frame of a mob without its
// metadata (ClientboundBundlePacket, as ServerEntity.sendPairingData uses).
const MsgBundle = 0x85

// MsgBundleOpen / MsgBundleClose (w→gw) bracket frames the world sends
// separately but that belong in one bundle — a spawn and its pairing data.
// The world sends both on its reliable path, so an open is always closed.
const (
	MsgBundleOpen  = 0x88
	MsgBundleClose = 0x89
)

// BundleMark is the (empty) payload of both markers.
type BundleMark struct{}

// MsgBundleEnd is the queue's marker for the end of a bundle's frames. It is
// never sent over the wire.
const MsgBundleEnd = 0xff

// Bundle is the frames of one bundle, in order.
type Bundle struct {
	Frames []BundleFrame `json:"frames"`
}

// BundleFrame is one frame inside a bundle.
type BundleFrame struct {
	Type    byte            `json:"t"`
	Payload json.RawMessage `json:"p"`
}

// FrameQueue reads frames, expanding bundles in place: after a MsgBundle is
// returned, its frames come next, then a MsgBundleEnd marker.
type FrameQueue struct {
	queued []BundleFrame
}

// Next is ReadFrame with the queue in front of it.
func (q *FrameQueue) Next(r io.Reader) (byte, []byte, error) {
	if len(q.queued) > 0 {
		f := q.queued[0]
		q.queued = q.queued[1:]
		return f.Type, f.Payload, nil
	}
	return ReadFrame(r)
}

// Expand queues a bundle's frames and its end marker; ok false if the
// payload is not a bundle (nothing is queued).
func (q *FrameQueue) Expand(payload []byte) bool {
	var b Bundle
	if json.Unmarshal(payload, &b) != nil {
		return false
	}
	q.queued = append(q.queued, b.Frames...)
	q.queued = append(q.queued, BundleFrame{Type: MsgBundleEnd})
	return true
}
