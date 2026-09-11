package render770

// attributes.go renders update_attributes.

import (
	"strings"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// IDUpdateAttributes is canonical-770 update_attributes.
const IDUpdateAttributes = 0x7c

// UpdateAttributes renders an entity's attribute snapshots: canonical
// registry ids (the translation chain remaps them per client), the base,
// and each modifier as identifier + amount + operation. Names the registry
// does not know are left out rather than guessed at.
func UpdateAttributes(e attach.EntityAttributes) Packet {
	b := protocol.AppendVarInt(nil, e.EID)
	var entries [][]byte
	for _, a := range e.Attrs {
		id, ok := protocol.AttributeID(a.Name)
		if !ok {
			continue
		}
		entry := protocol.AppendVarInt(nil, id)
		entry = protocol.AppendF64(entry, a.Base)
		entry = protocol.AppendVarInt(entry, int32(len(a.Modifiers)))
		for _, m := range a.Modifiers {
			entry = protocol.AppendString(entry, modifierIdentifier(m.ID))
			entry = protocol.AppendF64(entry, m.Amount)
			entry = protocol.AppendVarInt(entry, m.Op)
		}
		entries = append(entries, entry)
	}
	b = protocol.AppendVarInt(b, int32(len(entries)))
	for _, entry := range entries {
		b = append(b, entry...)
	}
	return Packet{IDUpdateAttributes, b}
}

// modifierIdentifier makes a modifier source a valid identifier: lower
// case, the allowed character set, and a namespace (tachyne's when the
// source has none) — the client rejects anything else.
func modifierIdentifier(src string) string {
	var sb strings.Builder
	for _, c := range strings.ToLower(src) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_', c == '-', c == '.', c == '/', c == ':':
			sb.WriteRune(c)
		default:
			sb.WriteByte('_')
		}
	}
	id := sb.String()
	if id == "" {
		id = "modifier"
	}
	if !strings.Contains(id, ":") {
		id = "tachyne:" + id
	}
	return id
}
