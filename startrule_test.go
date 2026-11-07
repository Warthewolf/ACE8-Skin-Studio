package main

import (
	"strings"
	"testing"
)

// The experimental start rule: an unlock rule with no conditions, written and read back exactly in
// the game's layout, next to each new skin's mission rule; its id is kept while the skin keeps its
// number and never shared with any other rule.
func TestStartRuleWritesAndReadsBack(t *testing.T) {
	_, ut := synthGameLists()
	e, err := ut.NewStartEntry(1400700, 21601001)
	if err != nil {
		t.Fatal(err)
	}
	if conds := objList(e, "ConditionEntryList"); len(conds) != 0 {
		t.Fatalf("start rule has %d conditions", len(conds))
	}
	if unlockedSkin(e) != 21601001 || e.Int("ID") != 1400700 {
		t.Fatalf("start rule unlocks %d with id %d", unlockedSkin(e), e.Int("ID"))
	}
	// the template it was copied from keeps its condition
	if tmpl, _ := ut.templateEntry(); len(objList(tmpl, "ConditionEntryList")) != 1 {
		t.Fatal("the game's rule lost its condition")
	}
	ut.Append(e)
	ut.head = []byte{0, 0, 0, 0}
	nt, data, err := ut.encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := readUnlockTableData(nt.Names, data, builtinSchema)
	if err != nil {
		t.Fatalf("the list with a start rule does not read back exactly: %v", err)
	}
	var got *Obj
	for _, x := range back.Entries() {
		if x.Int("ID") == 1400700 {
			got = x
		}
	}
	if got == nil || len(objList(got, "ConditionEntryList")) != 0 || unlockedSkin(got) != 21601001 {
		t.Fatalf("start rule read back as %+v", got)
	}
}

func TestStartIDs(t *testing.T) {
	st, ut := synthGameLists()
	mods := []InstalledSlotMod{fakeSlotMod("Alpha_abcde", "mission")}
	placed, assign, refs := assignNumbers(st, ut, mods, SkinListState{})
	giveStartIDs(ut, SkinListState{}, placed, assign)
	p := placed[0]
	if p.StartID == 0 || p.StartID == p.UnlockID || ut.UsedIDs()[p.StartID] || assign[0].StartID != p.StartID {
		t.Fatalf("start id %d (mission rule %d, record %d)", p.StartID, p.UnlockID, assign[0].StartID)
	}
	// both rules go into the unlock list, for the same skin
	if err := addSlotRows(builtinSchema, st, ut, placed, refs); err != nil {
		t.Fatal(err)
	}
	if ids := ruleFor(ut, p.SkinID); len(ids) != 2 {
		t.Fatalf("rules for the new skin: %v", ids)
	}
	// rebuilt with the same number: the same ids
	st2, ut2 := synthGameLists()
	old := SkinListState{Assign: assign, Retired: []int64{assign[0].UnlockID, assign[0].StartID}}
	placed2, assign2, _ := assignNumbers(st2, ut2, mods, old)
	giveStartIDs(ut2, old, placed2, assign2)
	if placed2[0].Number != p.Number || placed2[0].StartID != p.StartID || placed2[0].UnlockID != p.UnlockID {
		t.Fatalf("rebuilt: %+v, before %+v", placed2[0], p)
	}
	// its number taken meanwhile (a game update added that skin): it moves, and gets a new start
	// id, never one given out before
	st3, ut3 := synthGameLists()
	st3.Rows = append(st3.Rows, synthSkinRow(skinIDOf(21, p.Number), gameMaterial(p.Number), "AircraftSkin_Name_su57_x"))
	placed3, assign3, _ := assignNumbers(st3, ut3, mods, old)
	giveStartIDs(ut3, old, placed3, assign3)
	q := placed3[0]
	if q.Number == p.Number || q.Problem != "" {
		t.Fatalf("the skin did not move: %+v", q)
	}
	if q.StartID == 0 {
		t.Fatal("moved skin has no start rule")
	}
	for _, id := range []int64{p.StartID, p.UnlockID, q.UnlockID} {
		if q.StartID == id {
			t.Fatalf("moved skin's start id %d reuses %d", q.StartID, id)
		}
	}
	// the experiment's ids count as this program's in "Prepare my save"
	if !ourRuleIDs(SkinListState{Assign: assign})[p.StartID] {
		t.Fatal("start rule not forgotten by Prepare my save")
	}
}

// The report's findings in plain words: DLC skins with no rule, whether the save lists them, and
// whether the game acted on the experimental start rules.
func TestSkinReportLines(t *testing.T) {
	r := &skinReport{SaveFound: true, NoCondition: 1, Groups: []reportGroup{
		{DLCID: "Live", Category: "Campaign", Count: 593},
		{DLCID: "DLC0002", Category: "Campaign", Count: 2, NoRule: 2, InSave: 2},
		{DLCID: "DLC0000", Category: "None", Count: 3, NoRule: 3},
	}, Ours: []reportRule{{Kind: "skin", Name: "Navy", ID: 1400689}, {Kind: "skin", Name: "Navy", ID: 1400690, Start: true, Activated: true}}}
	lines := strings.Join(reportLines(r), "\n")
	for _, want := range []string{"5 DLC skins (DLC0002, DLC0000); 5 of them have no unlock rule", "Your save lists 2 DLC skins", "1 unlock rules with no conditions", "acted on 1 of 1 experimental start rules"} {
		if !strings.Contains(lines, want) {
			t.Errorf("missing %q in:\n%s", want, lines)
		}
	}
	r.Ours[1].Activated = false
	r.Groups[1].InSave = 0
	lines = strings.Join(reportLines(r), "\n")
	if !strings.Contains(lines, "has not acted on any of the 1") || !strings.Contains(lines, "lists no DLC skins") {
		t.Errorf("not acted on:\n%s", lines)
	}
	if txt := reportText(r); !strings.Contains(txt, "DLC0002 / Campaign / false: 2, 0, 2") || !strings.Contains(txt, "1400690: skin, Navy, true") {
		t.Errorf("report text:\n%s", txt)
	}
}
