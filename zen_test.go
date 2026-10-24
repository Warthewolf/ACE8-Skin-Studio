package main

import (
	"testing"
)

const slotTestUtoc = "/mnt/user-data/uploads/AceComba8EXPORTS/SkinSlotTest/packed/SkinSlotTest_P.utoc"

func TestZenReadSkinList(t *testing.T) {
	needFile(t, slotTestUtoc)
	needFile(t, slotTestUtoc[:len(slotTestUtoc)-5]+".ucas")
	st, ut, err := readZenSkinTables(slotTestUtoc, builtinSchema)
	if err != nil {
		t.Fatal(err)
	}
	if st == nil || ut == nil {
		t.Fatal("lists not found")
	}
	var news []int64
	for _, r := range st.Rows {
		if id := r.SkinID(); id > 0 && slotOf(id) >= 600 {
			news = append(news, id)
		}
	}
	t.Logf("%d rows, new: %v; %d unlock entries", len(st.Rows), news, len(ut.Entries()))
	if len(news) < 2 {
		t.Fatalf("expected the test slots, got %v", news)
	}
}
