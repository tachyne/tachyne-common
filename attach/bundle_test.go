package attach

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestFrameQueueExpandsBundles(t *testing.T) {
	var wire bytes.Buffer
	add, _ := json.Marshal(EntityAdd{EID: 5})
	b := Bundle{Frames: []BundleFrame{{Type: MsgEntityAdd, Payload: add}}}
	WriteJSON(&wire, MsgBundle, b)
	WriteJSON(&wire, MsgPing, struct{}{})
	var q FrameQueue
	typ, payload, err := q.Next(&wire)
	if err != nil || typ != MsgBundle || !q.Expand(payload) {
		t.Fatalf("first frame %#x %v", typ, err)
	}
	var got []byte
	for i := 0; i < 3; i++ {
		typ, _, err := q.Next(&wire)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, typ)
	}
	if !bytes.Equal(got, []byte{MsgEntityAdd, MsgBundleEnd, MsgPing}) {
		t.Fatalf("order %x", got)
	}
}
