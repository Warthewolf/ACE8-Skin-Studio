package main

// "How the game unlocks skins": a read-only report of the game's own skin list and unlock lists, and
// of what the save holds for them. It settles, on the player's own computer, the questions in
// docs/dlc-research.md: how the game marks DLC skins (DLCID, SkinCategory, SkinType), which skins
// have no unlock rule (they are shown some other way: owned DLC), which condition types the game's
// rules use, whether owned DLC skins are written into the save's unlocked list, and whether the game
// acted on the unlock rules of this program's skins (including the experimental rules with no
// conditions). Nothing is written: not the game's files, not the save.

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
)

type reportGroup struct {
	DLCID      string         `json:"dlcid"`
	Category   string         `json:"category"`
	SkinType   bool           `json:"skinType"`
	Count      int            `json:"count"`
	Examples   []int64        `json:"examples"`
	NoRule     int            `json:"noRule"` // rows with no unlock rule at all
	InSave     int            `json:"inSave"` // rows whose id is in the save's unlocked list
	Conditions map[string]int `json:"conditions"`
}

type reportRule struct {
	Kind      string `json:"kind"` // skin or emblem
	Name      string `json:"name"`
	ID        int64  `json:"id"`
	Start     bool   `json:"start"`     // the experimental rule with no conditions
	InSave    bool   `json:"inSave"`    // the save has a record of the rule
	Activated bool   `json:"activated"` // and the game marked it done
	Unlocked  bool   `json:"unlocked"`  // the skin or emblem is in the save's unlocked list
}

type skinReport struct {
	Groups       []reportGroup       `json:"groups"`
	ConditionUse map[string]int      `json:"conditionUse"` // skin and emblem unlock rules together
	ConditionEg  map[string][]string `json:"conditionEg"`  // parameter examples per condition type
	NoCondition  int                 `json:"noCondition"`  // the game's own rules with no conditions
	SaveFound    bool                `json:"saveFound"`
	SaveProblem  string              `json:"saveProblem,omitempty"`
	SaveNames    []string            `json:"saveNames"` // save property names that look like ownership
	Ours         []reportRule        `json:"ours"`      // this program's rules and what the save says
	Lines        []string            `json:"lines"`     // the findings in plain words
	Text         string              `json:"text"`      // everything, to copy into a message
}

var reOwnershipName = regexp.MustCompile(`(?i)dlc|package|purchase|entitle|owned|store|product`)

// enumText gives an enum property's name: the zero value (absent) is the enum's first value, and a
// value the layout does not name comes back as a number.
func enumText(o *Obj, name, zero string) string {
	v, _ := o.Get(name)
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return zero
	}
	return fmt.Sprintf("#%v", v)
}

func objList(o *Obj, name string) []*Obj {
	v, _ := o.Get(name)
	arr, _ := v.([]any)
	var out []*Obj
	for _, x := range arr {
		if e, ok := x.(*Obj); ok {
			out = append(out, e)
		}
	}
	return out
}

// ruleConditions names an unlock rule's condition types ("" for a rule with none).
func ruleConditions(e *Obj) string {
	var ts []string
	for _, c := range objList(e, "ConditionEntryList") {
		ts = append(ts, enumText(c, "ConditionType", "Unassigned"))
	}
	return strings.Join(ts, "+")
}

func doSkinReport(j *Job, s Settings) (*skinReport, error) {
	j.SetStep("Reading the game's skin list", 0.1)
	st, ut, err := loadBaseTables(j, s)
	if err != nil {
		return nil, err
	}
	rep := &skinReport{ConditionUse: map[string]int{}, ConditionEg: map[string][]string{}, Ours: []reportRule{}}
	ours := map[int64]bool{}
	state := loadListState(s)
	for _, a := range state.Assign {
		ours[a.UnlockID], ours[a.StartID] = true, true
	}
	for _, a := range state.Emblems {
		ours[a.UnlockID], ours[a.StartID] = true, true
	}
	delete(ours, 0)
	ruled := map[int64][]string{} // skin id -> condition types of the game's rules for it
	count := func(u *UnlockTable, action string, into map[int64][]string) {
		for _, e := range u.Entries() {
			if ours[e.Int("ID")] {
				continue // the game's own rules only
			}
			ct := ruleConditions(e)
			if ct == "" {
				rep.NoCondition++
				ct = "(none)"
			}
			rep.ConditionUse[ct]++
			for _, c := range objList(e, "ConditionEntryList") {
				t := enumText(c, "ConditionType", "Unassigned")
				if eg := rep.ConditionEg[t]; len(eg) < 4 && !containsString(eg, c.Str("Parameters")) {
					rep.ConditionEg[t] = append(eg, c.Str("Parameters"))
				}
			}
			if into != nil {
				if id := unlockedBy(e, action); id != 0 {
					into[id] = append(into[id], ct)
				}
			}
		}
	}
	count(ut, "UnlockSkin", ruled)
	j.SetStep("Reading the game's emblem list", 0.5)
	if _, eu, err := loadEmblemTables(j, s); err == nil {
		eu.action = "UnlockEmblem"
		count(eu, "UnlockEmblem", nil)
	} else {
		j.Log("The emblem list could not be read (%v); the report covers skins only.", err)
	}

	// the save, read only
	j.SetStep("Reading your save", 0.8)
	unlocked := map[int64]bool{}
	activated := map[int64]bool{}
	recorded := map[int64]bool{}
	if p, err := s.campaignPath(); err != nil {
		rep.SaveProblem = err.Error()
	} else if raw, err := os.ReadFile(p); err != nil {
		rep.SaveProblem = err.Error()
	} else if f, err := parseSave(raw); err != nil {
		rep.SaveProblem = err.Error()
	} else {
		rep.SaveFound = true
		for _, l := range []string{saveSkinList, saveEmblemList} {
			ids, _, _ := saveIDs(f, l)
			for _, id := range ids {
				unlocked[int64(id)] = true
			}
		}
		names := map[string]bool{}
		for _, sf := range []*saveFile{f} {
			sf.walk(func(p *gvProp) {
				if reOwnershipName.MatchString(p.Name) {
					names[p.Name] = true
				}
			})
		}
		for n := range names {
			rep.SaveNames = append(rep.SaveNames, n)
		}
		sort.Strings(rep.SaveNames)
		if r := f.prop(saveUnlockRules); r != nil {
			for _, e := range r.Elems {
				if v, ok := e.find("ID").int32Val(); ok {
					id := int64(uint32(v))
					recorded[id] = true
					activated[id] = e.find("bIsActivated").boolVal()
				}
			}
		}
	}

	groups := map[[3]string]*reportGroup{}
	for _, r := range st.Rows {
		id := r.SkinID()
		if isModSkin(id, nil) {
			continue // the game's own rows only
		}
		v, _ := r.Obj.Get("SkinType")
		skinType, _ := v.(bool)
		k := [3]string{r.Obj.Str("DLCID"), enumText(r.Obj, "SkinCategory", "None"), fmt.Sprint(skinType)}
		g := groups[k]
		if g == nil {
			g = &reportGroup{DLCID: k[0], Category: k[1], SkinType: skinType, Conditions: map[string]int{}}
			groups[k] = g
		}
		g.Count++
		if len(g.Examples) < 6 {
			g.Examples = append(g.Examples, id)
		}
		if unlocked[id] {
			g.InSave++
		}
		if len(ruled[id]) == 0 {
			g.NoRule++
		}
		for _, ct := range ruled[id] {
			g.Conditions[ct]++
		}
	}
	for _, g := range groups {
		rep.Groups = append(rep.Groups, *g)
	}
	sort.Slice(rep.Groups, func(a, b int) bool { return rep.Groups[a].Count > rep.Groups[b].Count })

	// this program's skins and emblems: did the game act on their rules?
	for _, it := range installedSaveItems(s) {
		for _, rid := range []struct {
			id    int64
			start bool
		}{{it.UnlockID, false}, {startIDOf(state, it), true}} {
			if rid.id == 0 {
				continue
			}
			rep.Ours = append(rep.Ours, reportRule{Kind: it.Kind, Name: it.Name, ID: rid.id, Start: rid.start,
				InSave: recorded[rid.id], Activated: activated[rid.id], Unlocked: unlocked[it.ID]})
		}
	}
	rep.Lines = reportLines(rep)
	rep.Text = reportText(rep)
	return rep, nil
}

// startIDOf is the experimental start rule of an installed skin or emblem (0 when it has none).
func startIDOf(st SkinListState, it saveItem) int64 {
	if it.Kind == "emblem" {
		for _, a := range st.Emblems {
			if a.Mod == it.Mod && a.Emblem == it.Key {
				return a.StartID
			}
		}
		return 0
	}
	for _, a := range st.Assign {
		if a.Mod == it.Mod && a.Slot == it.Key {
			return a.StartID
		}
	}
	return 0
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// reportLines says what the report means, in plain words.
func reportLines(r *skinReport) []string {
	var out []string
	dlc, dlcInSave, dlcNoRule := 0, 0, 0
	var dlcIDs []string
	for _, g := range r.Groups {
		if g.DLCID != "Live" && g.DLCID != "" {
			dlc += g.Count
			dlcInSave += g.InSave
			dlcNoRule += g.NoRule
			if !containsString(dlcIDs, g.DLCID) {
				dlcIDs = append(dlcIDs, g.DLCID)
			}
		}
	}
	if dlc == 0 {
		out = append(out, "The game's skin list has no DLC skins (every skin is marked Live, the base game).")
	} else {
		out = append(out, fmt.Sprintf("The game's skin list has %d DLC skins (%s); %d of them have no unlock rule, so the game shows them by checking which DLC you own.", dlc, strings.Join(dlcIDs, ", "), dlcNoRule))
		if r.SaveFound {
			if dlcInSave > 0 {
				out = append(out, fmt.Sprintf("Your save lists %d DLC skins as unlocked: the game writes owned DLC skins into the save too.", dlcInSave))
			} else {
				out = append(out, "Your save lists no DLC skins as unlocked: either you do not own a skin DLC, or the game shows owned DLC skins without writing them into the save.")
			}
		}
	}
	if r.NoCondition > 0 {
		out = append(out, "The game itself has "+countOf(r.NoCondition, "unlock rule")+" with no conditions.")
	}
	starts, acted := 0, 0
	for _, o := range r.Ours {
		if o.Start {
			starts++
			if o.Activated {
				acted++
			}
		}
	}
	switch {
	case starts == 0:
	case !r.SaveFound:
		out = append(out, "The experimental start rules cannot be checked: the save was not found.")
	case acted > 0:
		out = append(out, fmt.Sprintf("The game acted on %d of the %s: new skins can be unlocked at game start, by the game itself.", acted, countOf(starts, "experimental start rule")))
	default:
		out = append(out, fmt.Sprintf("The game has not acted on the %s yet. If the game has been started since they were added, it does not act on rules with no conditions for skins.", countOf(starts, "experimental start rule")))
	}
	return out
}

func reportText(r *skinReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: how the game unlocks skins\n\n", appName, appVersion)
	for _, l := range r.Lines {
		b.WriteString("- " + l + "\n")
	}
	b.WriteString("\nThe game's skin list (DLCID / SkinCategory / SkinType: rows, in your save, with no rule, rule conditions, examples)\n")
	for _, g := range r.Groups {
		var cs []string
		for c, n := range g.Conditions {
			cs = append(cs, fmt.Sprintf("%s %d", c, n))
		}
		sort.Strings(cs)
		fmt.Fprintf(&b, "  %s / %s / %v: %d, %d, %d, [%s], e.g. %v\n", g.DLCID, g.Category, g.SkinType, g.Count, g.InSave, g.NoRule, strings.Join(cs, ", "), g.Examples)
	}
	b.WriteString("\nCondition types of the game's unlock rules (skins and emblems)\n")
	var ks []string
	for k := range r.ConditionUse {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		fmt.Fprintf(&b, "  %s: %d", k, r.ConditionUse[k])
		for _, t := range strings.Split(k, "+") {
			if eg := r.ConditionEg[t]; len(eg) > 0 {
				fmt.Fprintf(&b, "  e.g. %q", eg)
				break
			}
		}
		b.WriteString("\n")
	}
	if r.SaveFound {
		fmt.Fprintf(&b, "\nSave fields that look like ownership: %s\n", strings.Join(append([]string{}, r.SaveNames...), ", "))
	} else {
		fmt.Fprintf(&b, "\nSave not read: %s\n", r.SaveProblem)
	}
	if len(r.Ours) > 0 {
		b.WriteString("\nRules of the new skins and emblems (rule id: kind, name, start rule, in save, done, unlocked)\n")
		for _, o := range r.Ours {
			fmt.Fprintf(&b, "  %d: %s, %s, %v, %v, %v, %v\n", o.ID, o.Kind, o.Name, o.Start, o.InSave, o.Activated, o.Unlocked)
		}
	}
	return b.String()
}

func skinReportRoutes(api func(string, http.HandlerFunc)) {
	api("/api/skins/report", func(w http.ResponseWriter, r *http.Request) {
		s := getSettings()
		startJob(w, "How the game unlocks skins", []string{s.AESKey}, func(j *Job) (any, error) { return doSkinReport(j, s) })
	})
}
