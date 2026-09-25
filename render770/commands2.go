package render770

import (
	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical 770 ids.
const (
	IDSetCamera    = 0x56
	IDTickingState = 0x78
	IDTickingStep  = 0x79
	IDTransfer     = 0x7a
)

// Transfer renders transfer: host, then port.
func Transfer(e attach.Transfer) Packet {
	return Packet{IDTransfer, protocol.AppendVarInt(protocol.AppendString(nil, e.Host), e.Port)}
}

// Camera renders set_camera.
func Camera(e attach.Camera) Packet {
	return Packet{IDSetCamera, protocol.AppendVarInt(nil, e.EID)}
}

// TickingState renders ticking_state: the rate, then frozen.
func TickingState(e attach.TickingState) Packet {
	return Packet{IDTickingState, protocol.AppendBool(protocol.AppendF32(nil, e.Rate), e.Frozen)}
}

// TickingStep renders ticking_step.
func TickingStep(e attach.TickingStep) Packet {
	return Packet{IDTickingStep, protocol.AppendVarInt(nil, e.Steps)}
}
