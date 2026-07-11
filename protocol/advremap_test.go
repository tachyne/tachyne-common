package protocol

import "testing"

// TestAdvancementsID774Route pins advancementsID774 to the generated protomap
// route: update_advancements is 0x7b at canonical 770 and must arrive at the
// 774→775 step (where the icon layout rewriter is keyed) renumbered by the
// intermediate steps. If a regenerated protomap moves the packet, this fails
// instead of the rewriter silently never firing.
func TestAdvancementsID774Route(t *testing.T) {
	id := int32(canonUpdateAdvancements)
	for v := int32(771); v <= 774; v++ {
		if nid, ok := protoSteps[v].cbUp[StatePlay][id]; ok {
			id = nid
		}
	}
	if id != advancementsID774 {
		t.Fatalf("update_advancements reaches the 775 step as 0x%x; advancementsID774 = 0x%x", id, advancementsID774)
	}
	// and the rewriter is actually registered there
	if stepBody[775].cbUp[StatePlay][advancementsID774] == nil {
		t.Fatal("no 775 icon rewriter registered for update_advancements")
	}
}
