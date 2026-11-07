// ---------------------------------------------------------------- how new skins are unlocked (Skin slots)
// The start-rule experiment (settings.unlockAtStart) and the read-only report "How the game unlocks
// skins" (/api/skins/report). See docs/dlc-research.md for why new skins are not shipped as DLC.
const unlocksBlock = el('div', { class: 'block', id: 'ulBlock' },
  el('h2', { text: 'How new skins are unlocked' }),
  el('p', { class: 'hint', text: 'New skins and emblems unlock when you finish a campaign mission, the same way the game unlocks its own first skins. The game itself writes that into your save; Skin Studio does not touch it. Unlock now on the Save tools screen does it straight away instead, after backing your save up.' }),
  el('label', { class: 'check' }, el('input', { type: 'checkbox', id: 'ulStart' }),
    el('span', {}, 'Also try to unlock them when the game starts (experimental)',
      el('span', { class: 'hint', text: 'Gives each new skin and emblem a second unlock rule with no conditions, the way the game unlocks its own music player. If the game acts on it, new skins appear without a mission, and the game writes your save itself. If it does not, finishing a mission still unlocks them, so nothing is lost. Not tried in the game yet: the steps to try it are below.' }))),
  el('div', { id: 'ulStartOut' }),
  el('details', { class: 'ul-test' }, el('summary', { text: 'How to try the experiment in the game' }),
    el('ol', {},
      el('li', { text: 'Open Save tools, press Back up now, and turn on Backup at game start.' }),
      el('li', { text: 'Tick the box above. The skin list is rebuilt.' }),
      el('li', { text: 'Make one new test skin. Do not press Unlock now.' }),
      el('li', { text: 'Start the game (offline) and go straight to the hangar, without playing a mission. Is the test skin there?' }),
      el('li', { text: 'Close the game, come back here and press "How the game unlocks skins". It says whether the game acted on the new rule. Copy the report and send it to whoever asked you to try it.' }))),
  el('div', { class: 'row' },
    el('button', { class: 'btn small', id: 'ulReport', type: 'button' }, 'How the game unlocks skins'),
    el('span', { class: 'hint', text: 'Reads the game\'s own lists and your save, and changes nothing.' })),
  el('div', { id: 'ulReportOut' }));
{
  const before = $('#slTablesBlock') || $('#slRemovedBlock');
  if (before) before.before(unlocksBlock); else $('#page-slots').append(unlocksBlock);
}

function unlocksButtons() {
  const busy = !!activeJob;
  $('#ulStart').disabled = busy;
  $('#ulReport').disabled = busy || !(STATE && STATE.checks.paks.ok && STATE.checks.aes.ok);
  $('#ulReport').title = $('#ulReport').disabled && !busy ? 'Needs the game\'s Paks folder and AES key in Settings.' : '';
  if (STATE) $('#ulStart').checked = !!STATE.settings.unlockAtStart;
}

$('#ulStart').addEventListener('change', async ev => {
  const on = ev.target.checked, out = $('#ulStartOut');
  out.replaceChildren();
  try {
    await api('/api/settings', { unlockAtStart: on });
    await refreshState();
  } catch (e) { ev.target.checked = !on; notice(out, 'error', e.message); return; }
  const hasSkins = SLOTS && (SLOTS.mods || []).length > 0;
  if (!hasSkins || !STATE.checks.mods.ok) {
    notice(out, 'info', on ? 'Turned on. New skins and emblems get the extra rule when they are built.' : 'Turned off.');
    return;
  }
  try {
    const res = await runJob('/api/skinlist/rebuild', {});
    const box = el('div', { class: 'notice good', role: 'status' }, el('strong', { text: on ? 'Turned on. ' : 'Turned off. ' }),
      on ? 'The skin list now gives every new skin and emblem the extra rule. Start the game to try it.' : 'The extra rules are out of the skin list again; finishing a mission still unlocks new skins.');
    const saved = savedLine(res); if (saved) box.append(saved);
    out.replaceChildren(box);
    for (const p of (res && res.problems) || []) out.append(el('div', { class: 'notice warn', style: 'margin-top:8px', text: p }));
  } catch (e) { notice(out, 'error', e.message); }
  loadSlots();
});

$('#ulReport').addEventListener('click', async () => {
  const out = $('#ulReportOut');
  out.replaceChildren();
  try {
    const r = await runJob('/api/skins/report', {});
    const groups = el('table', { class: 'ul-groups' },
      el('thead', {}, el('tr', {}, ['DLC id', 'Category', 'Skins', 'In your save', 'No unlock rule'].map(h => el('th', { text: h })))),
      el('tbody', {}, (r.groups || []).map(g => el('tr', {},
        el('td', { text: g.dlcid || '(none)' }), el('td', { text: g.category + (g.skinType ? ' · type' : '') }),
        el('td', { text: String(g.count) }), el('td', { text: String(g.inSave) }), el('td', { text: String(g.noRule) })))));
    const copy = el('button', { class: 'btn small', type: 'button', onclick: async ev => {
      const b = ev.currentTarget;
      try { await navigator.clipboard.writeText(r.text); b.textContent = 'Copied'; } catch (e) { b.textContent = 'Could not copy'; }
      setTimeout(() => { b.textContent = 'Copy report'; }, 2000);
    } }, 'Copy report');
    const box = el('div', { class: 'notice info', role: 'status' },
      el('strong', { text: 'What your game and save say' }),
      el('ul', { style: 'margin:6px 0 8px 18px' }, (r.lines || []).map(l => el('li', { text: l }))),
      r.saveFound ? null : el('p', { class: 'hint', text: 'Your save was not read (' + r.saveProblem + '); the game\'s lists were.' }),
      groups, el('div', { class: 'row', style: 'margin-top:8px' }, copy));
    out.replaceChildren(box);
  } catch (e) { notice(out, 'error', e.message); }
});
