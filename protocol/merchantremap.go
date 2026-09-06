package protocol

import "bytes"

// remapMerchantOffers rewrites merchant_offers for a translated client: the
// item ids inside each offer's ItemCosts and result Slot, and the component
// ids inside the result (an enchanted book's enchantments component
// renumbers at 774+). Layout, identical 770→26.2 (MerchantOffer.writeToStream):
//
//	varint containerId, varint count, per offer:
//	  ItemCost A  = varint item, varint count, varint componentPredicateCount (+entries)
//	  Slot result
//	  bool hasB [+ ItemCost B]
//	  bool outOfStock, i32 uses, i32 maxUses, i32 xp, i32 specialPrice,
//	  f32 priceMultiplier, i32 demand
//	varint villagerLevel, varint villagerXp, bool showProgress, bool canRestock
//
// Anything the parser does not understand (a cost with component predicates,
// which we never emit) returns the body untouched rather than guessing.
func remapMerchantOffers(version int32, body []byte) []byte {
	r := bytes.NewReader(body)
	remap := func(i int32) int32 { return RemapID(RegItem, version, i) }
	var out []byte
	container, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	out = AppendVarInt(out, container)
	n, err := ReadVarInt(r)
	if err != nil || n < 0 || n > 64 {
		return body
	}
	out = AppendVarInt(out, n)
	copyCost := func() bool {
		item, e1 := ReadVarInt(r)
		count, e2 := ReadVarInt(r)
		preds, e3 := ReadVarInt(r)
		if e1 != nil || e2 != nil || e3 != nil || preds != 0 {
			return false
		}
		out = AppendVarInt(out, remap(item))
		out = AppendVarInt(out, count)
		out = AppendVarInt(out, 0)
		return true
	}
	for i := int32(0); i < n; i++ {
		if !copyCost() {
			return body
		}
		if !copyFullSlot(r, &out, remap, version, false) {
			return body
		}
		hasB, err := r.ReadByte()
		if err != nil {
			return body
		}
		out = append(out, hasB)
		if hasB != 0 && !copyCost() {
			return body
		}
		var fixed [1 + 4*4 + 4 + 4]byte // outOfStock, uses, maxUses, xp, specialPrice, priceMultiplier, demand
		if _, err := r.Read(fixed[:]); err != nil || r.Len() < 0 {
			return body
		}
		out = append(out, fixed[:]...)
	}
	rest := make([]byte, r.Len()) // villagerLevel, villagerXp, showProgress, canRestock — version-stable
	r.Read(rest)
	return append(out, rest...)
}
