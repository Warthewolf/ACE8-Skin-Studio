# New skins as DLC instead of a save unlock: research

**Question.** Today a new skin becomes available in one of two ways: the game unlocks it when a campaign mission ends (an unlock rule Skin Studio adds to `DA_Unlock_Skin`), or the player presses **Unlock now**, which edits `Campaign.sav`. Could new skins instead be presented the way the game presents DLC skins, so they appear without the save being edited?

**Short answer.** No, not in a way that helps. In Ace Combat 8 a skin's DLC id is an **ownership check that can hide a skin**, not a way to make one appear. Two things make the DLC route a dead end:

1. A made-up DLC id would most likely hide the skin for everyone, because no one owns that DLC.
2. A real DLC id would show the skin only to players who bought that DLC. Making it appear for anyone else would mean faking DLC ownership. That is a DLC bypass: Nexus Mods takes such mods down, and Skin Studio will not do it.

Two findings point elsewhere:

- **The save will hold mod skins whatever unlocks them.** As soon as a player puts a skin on an aircraft, the game itself writes that skin's number into the save. This is why the safety net is backups, plus "Prepare my save for the game without mods" before the mods come out.
- **There is a more promising route than DLC**, and it is described below. It needs a test in the game.

## What the game's own data shows

The game's skin list and unlock lists were read from the tables that AC8 save-editor projects publish from the game files ([RivaTesu/ac8-save-editor](https://github.com/RivaTesu/ac8-save-editor) assets), and checked against what Skin Studio's code knows (`schema.json`, `skintable.go`, `savetools.go`).

| Finding | Detail |
|---|---|
| `DLCID` has three values | `Live` on 878 of 883 rows (the base game), `DLC0000` on 3 (the F‑14A aircraft DLC), `DLC0002` on 2 (the Customizable Cosmetic Set, also in the Deluxe Edition: F‑4E Mobius 25th, F/A‑18C Team Zvezda). |
| DLC skins have **no unlock rule** | Every base-game campaign skin has a rule in `DA_Unlock_Skin` (586 rules). The DLC skins have none. Guides report they become usable once the skin menu itself is unlocked, if you own the DLC. So the game checks ownership at run time instead of keeping a per-skin unlock. |
| Writing DLC items into the save does nothing | AC8 save editors report that DLC unlocks written into `Campaign.sav` "do not work" ([Ovek99/AC8SaveEditor](https://github.com/Ovek99/AC8SaveEditor)). This fits an ownership check made at run time. |
| `PurchasePackage` and `Always` are unused | No shipped rule in `DA_Unlock_Skin`, `_Emblem`, `_Achievement`, `_Feature` or `_AircraftTree` uses them. That matches Skin Studio's own result in 2.0.3: an `Always` rule unlocked nothing. |
| `SkinCategory` `All`/`Owned` are unused | Rows are only `Campaign` (593) or `None` (290, mostly online skins). |
| **The game unlocks something with no condition at all** | `DA_Unlock_Feature` rule 1800010 unlocks the music player with an **empty condition list**. It does not use `Always`. |
| AC7 did this differently | AC7 skin-slot tools set `DLCID = "Patch"` (AC7's "base game") on new rows, and needed no save edits ([since_SkinSlots](https://github.com/sincerity21/since_SkinSlots)). AC8's equivalent of "Patch" is `Live`, which Skin Studio already sets. AC8 also gates campaign skins with unlock rules, which AC7's table approach did not need. |

## The options, weighed

| Option | Shows new skins without a save edit by Skin Studio? | Safety | Verdict |
|---|---|---|---|
| **Mission rule** (today's default) | Yes. The **game** unlocks them and writes its own save with its own code when a mission ends. Skin Studio does not touch the save. | Proven in game since 2.0.3. | **Keep as the default.** |
| **Unlock now** (today's shortcut) | No. Skin Studio edits `Campaign.sav`, after a backup and a read-back check. | Safe so far thanks to the checks, but it is the riskiest path: a direct save edit. | Keep as an option, not a requirement. |
| **DLC id on the row** | Only for players who own that DLC. | A made-up id hides the skin. A real id ties free fan content to a paid DLC. Faking ownership is a DLC bypass. | **Rejected.** |
| **`SkinCategory` All/Owned, `SkinType`** | Unknown; the game never uses these values on a row. | Likely hides the skin, or ties it to online inventory. | Rejected. |
| **`PurchasePackage` rule** | Only if it names a package the player owns. Format unknown. | Same objection as DLC. | Rejected. |
| **Rule with no conditions** (the music-player pattern) | Possibly. If the game treats "no conditions" as met, **the game** unlocks the skin by itself without a mission, and Skin Studio never touches the save. | Data only: no hook and no ownership trick. If it does not fire, nothing breaks. Kept next to the mission rule, the skin still unlocks after a mission either way. | **Most promising. Offered as an opt-in experiment.** |
| UE4SS hook on the unlock or ownership check | Yes. | Needs reverse engineering and the anti-cheat client off. It breaks with game updates, and an ownership hook is DLC-bypass territory. | Not now. |

## What Skin Studio does with this

1. **Backups, always.** Before every change to the skin list (building, importing, changing or removing a skin or emblem, or rebuilding the list), the save is backed up. This is skipped only when the newest backup already holds exactly the same save. The last 20 of these are kept. Before every save edit (Unlock now, Prepare my save, restoring) both save files are backed up as before. The game-start backup mod copies the save each time the game starts.
2. **The mission rule stays the default.** The game writes its own save.
3. **The experiment** (Skin slots screen, off by default): each new skin and emblem also gets a second unlock rule with **no conditions**, the pattern the game uses for its music player. If the game acts on it, new skins appear without a mission and without Skin Studio touching the save. If it does not, the mission rule unlocks them as before.
4. **A read-only report** (Skin slots screen, "How the game unlocks skins") reads the game's own lists and your save, and shows:
   - the DLC ids, categories and skin types in use;
   - which skins have no unlock rule;
   - which condition types the game uses;
   - whether DLC skins you own are in your save's unlocked list.

   This settles the open questions on your own computer, and its text can be pasted into a message.

## Testing the experiment in the game

Use a save you do not mind losing, and keep the game **offline**.

1. On Save tools, press **Back up now**, and make sure **Backup at game start** is on.
2. On Skin slots, tick **Also try to unlock new skins at game start (experimental)**. The skin list is rebuilt.
3. Make one new test skin for an aircraft you own. Do **not** press Unlock now.
4. Start the game and go straight to the hangar, without playing a mission. Is the test skin there?
   - **Yes:** the game acted on the rule. Quit the game and open Save tools: the skin shows as unlocked, written by the game.
   - **No:** finish any campaign mission. The skin should then appear through the mission rule, which shows nothing else broke.
5. Send the result together with **Copy report** from "How the game unlocks skins".

If it works, the experiment can become the default and **Unlock now** can be left for emblems or kept only as a fallback.

## Open questions

- Does the game write owned DLC skins (3006001, 7006001) into `UnlockedSkinIdList`, or show them on the fly? The report answers this for a Deluxe or Cosmetic Set owner.
- What does the game do with a save that wears a skin whose row is gone (mods removed without "Prepare my save")? It is undocumented; save editors warn that the game may wipe saves it does not like. Until this is known, keep using "Prepare my save" and keep backups.
- Does `DLCID` on a mod row with an id the game does not know hide the row, show it, or cause an error? It is untested. It matters only for the rejected options above.

## Sources

- RivaTesu, AC8 save editor and its exported game tables: https://github.com/RivaTesu/ac8-save-editor
- Ovek99, AC8 save editor (notes on DLC unlocks and edited saves): https://github.com/Ovek99/AC8SaveEditor
- sasili7, AC8 Skin Manager (UE4SS material swapping, offline only): https://github.com/sasili7/AC8SkinManager
- sincerity21, AC7 Skin Re-Slotter (`DLCID = "Patch"`): https://github.com/sincerity21/since_SkinSlots
- Steam DLC ownership checks: https://partner.steamgames.com/doc/store/application/dlc and https://partner.steamgames.com/doc/api/ISteamApps
- Ace Combat 8 Deluxe Edition and DLC on Steam: https://store.steampowered.com/app/2288340 and https://store.steampowered.com/app/4249300
- Skin unlocking in AC8 (guide): https://allthings.how/ace-combat-8-how-to-unlock-and-change-aircraft-skins/
