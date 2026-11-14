package main

import "testing"

func fakeSkinRow(id int64, material string) SkinRow {
	return SkinRow{Key: skinRowKey(id), Obj: &Obj{Struct: skinRowStruct, Props: []Prop{
		{Name: "SkinID", Val: int32(id)},
		{Name: "DecalMaterialRef", Val: softPathTo(material)},
	}}}
}

// The ADF-X02's files are PP0038_adfx02, but the skin list keeps its skins
// under aircraft 138 (SkinID 138001001 ...), not 38.
func TestSkinAircraftOddNumber(t *testing.T) {
	st := &SkinTable{Rows: []SkinRow{
		fakeSkinRow(skinIDOf(22, 1), "/Game/Vehicles/Aircraft/PP/PP0022_f16c/Materials/001/MI_PP0022_f16c_001_Decal"),
		fakeSkinRow(skinIDOf(138, 1), "/Game/Vehicles/Aircraft/PP/PP0038_adfx02/Materials/001/MI_PP0038_adfx02_001_Decal"),
		fakeSkinRow(skinIDOf(138, 2), "/Game/Vehicles/Aircraft/PP/PP0038_adfx02/Materials/002/MI_PP0038_adfx02_002_Decal"),
	}}
	_, a, _, err := vehicleOfTexture("/Game/Vehicles/Aircraft/PP/PP0038_adfx02/Textures/001/T_PP0038_adfx02_001_C_BaseColor")
	if err != nil || a != 38 {
		t.Fatalf("vehicle number %d, %v", a, err)
	}
	if got := skinAircraft(st, "PP0038_adfx02", a); got != 138 {
		t.Fatalf("PP0038_adfx02 -> %d, want 138", got)
	}
	if got := skinAircraft(st, "PP0022_f16c", 22); got != 22 {
		t.Fatalf("PP0022_f16c -> %d, want 22", got)
	}
	if got := skinAircraft(st, "PP0099_none", 99); got != 99 {
		t.Fatalf("unknown aircraft -> %d, want 99 (unchanged)", got)
	}
	if got := skinAircraft(nil, "PP0038_adfx02", 38); got != 38 {
		t.Fatalf("no table -> %d", got)
	}
}

// Same check against the game's real skin list, when the test files are there.
func TestSkinAircraftGame(t *testing.T) {
	st, _ := loadGameTables(t)
	if got := skinAircraft(st, "PP0038_adfx02", 38); got != 138 || st.Find(skinIDOf(got, 1)) == nil {
		t.Fatalf("PP0038_adfx02 -> %d", got)
	}
}

// A "based on" number typed in the window is made from the vehicle's name (38); for the ADF-X02
// it is moved to the number the skin list uses (138) when the slot is built.
func TestSkinAircraftTypedTemplate(t *testing.T) {
	typed := skinIDOf(38, 2) // what the window sends for PP0038_adfx02, skin 002
	if got := skinIDOf(138, slotOf(typed)); got != 138002001 || aircraftOf(typed) != 38 {
		t.Fatalf("typed %d -> %d", typed, got)
	}
}
