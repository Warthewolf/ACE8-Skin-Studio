// ---------------------------------------------------------------- save tools
let SAVE = null;
async function loadSave() {
  try { SAVE = await api('/api/save/status'); }
  catch (e) { notice($('#svProblems'), 'error', e.message); return; }
  renderSave();
}

const fmtWhen = t => { const d = new Date(t); return isNaN(d) || d.getFullYear() < 2000 ? '' : d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }); };

function renderSave() {
  const S = SAVE;
  const items = S.items || [];
  const unlocked = items.filter(x => x.unlocked).length;
  const auto = S.autoBackup || {};
  $('#svStats').replaceChildren(
    stat('Save', S.found ? 'found' : 'not found', S.found && !S.problem ? 'good' : 'bad', S.found ? 'Last saved ' + fmtWhen(S.modified) : 'Set its folder in Settings'),
    stat('Game', S.gameRunning ? 'running' : 'closed', S.gameRunning ? 'bad' : 'good', S.gameRunning ? 'Close it to use these tools' : 'Ready'),
    stat('New skins and emblems', items.length ? unlocked + ' of ' + items.length + ' unlocked' : 'none yet', items.length ? (unlocked === items.length ? 'good' : 'warn') : '', items.length ? 'In the game\'s lists now' : 'Build one first'),
    stat('Backup at game start', auto.installed ? 'on' : (auto.dir ? 'off' : 'needs UE4SS'), auto.installed ? 'good' : 'warn', (S.backups || []).length + ((S.backups || []).length === 1 ? ' backup kept' : ' backups kept')));

  const probs = $('#svProblems');
  probs.replaceChildren();
  const warn = (...c) => probs.append(el('div', { class: 'notice warn', style: 'margin-bottom:8px' }, ...c));
  if (S.problem) warn(el('strong', { text: 'The save cannot be used. ' }), S.problem, ' ', el('button', { class: 'btn small', onclick: () => showPage('settings') }, 'Open Settings'));
  if (S.gameRunning) warn(el('strong', { text: 'The game is running. ' }), 'Close it first: the game writes its save when it closes, which would undo any change made now. ', el('button', { class: 'btn small', onclick: loadSave }, 'Check again'));

  // what Unlock now would unlock
  const box = $('#svItems');
  if (!items.length) {
    box.replaceChildren(el('p', { class: 'hint', text: 'No new skins or emblems are in the game\'s lists yet. Make one with New skin slot or New emblem, or import a skin mod on the Skin slots screen.' }));
  } else {
    box.replaceChildren(el('div', { class: 'list', style: 'margin-bottom:12px' }, items.map(it => el('div', { class: 'svrow' },
      el('div', { class: 'what' }, el('b', { text: it.name }),
        el('small', { text: [it.kind === 'skin' ? (it.plane || 'skin') + ' skin ' + pad3(it.number) : 'emblem ' + it.number, it.modTitle, it.worn ? 'worn by ' + it.worn + (it.worn === 1 ? ' aircraft' : ' aircraft') : ''].filter(Boolean).join('  ·  ') })),
      el('span', { class: 'chip ' + (it.unlocked ? 'blue' : 'orange'), text: it.unlocked ? 'unlocked' : 'not unlocked yet' })))));
  }
  const left = items.filter(x => !x.unlocked).length;
  $('#svUnlockHint').textContent = !items.length ? '' : left ? (left === 1 ? '1 to unlock.' : left + ' to unlock.') : 'All of them are unlocked already.';

  // the vanilla button, with a confirmation
  const vrow = $('#svVanillaRow');
  const vNote = [S.worn ? (S.worn === 1 ? '1 aircraft wears a new skin or emblem' : S.worn + ' aircraft wear a new skin or emblem') : '',
    S.leftover ? (S.leftover === 1 ? '1 unlocked one is not installed any more' : S.leftover + ' unlocked ones are not installed any more') : ''].filter(Boolean).join('; ');
  const vNormal = () => vrow.replaceChildren(...[el('button', { class: 'btn', type: 'button', onclick: vConfirm }, 'Prepare my save for the game without mods'),
    vNote ? el('span', { class: 'hint', text: vNote + '.' }) : null].filter(Boolean));
  const vConfirm = () => vrow.replaceChildren(el('span', { class: 'hint', text: 'Take the new skins and emblems out of your save? It is backed up first.' }),
    el('button', { class: 'btn danger', type: 'button', onclick: () => saveJob('/api/save/vanilla', {}, 'Your save is ready for the game without mods.') }, 'Yes, prepare it'),
    el('button', { class: 'btn quiet', type: 'button', onclick: vNormal }, 'Cancel'));
  vNormal();

  // the backup at game start
  const a = $('#svAuto');
  if (auto.installed) {
    a.replaceChildren(el('p', { class: 'hint' }, el('strong', { text: 'Your save is backed up each time the game starts. ' }), 'The last 20 starts are kept, as "when the game started" below. It is SkinStudioSaveBackup, a small UE4SS mod; it only reads the save.'),
      el('div', { class: 'row', style: 'margin-bottom:12px' }, el('button', { class: 'btn small', type: 'button', onclick: () => autoMod(false) }, 'Turn it off')));
  } else if (auto.dir) {
    a.replaceChildren(el('p', { class: 'hint' }, el('strong', { text: 'Back up the save each time the game starts? ' }), 'This adds SkinStudioSaveBackup, a small UE4SS mod that copies the save when the game starts, keeping the last 20 starts. It only reads the save.'),
      el('div', { class: 'row', style: 'margin-bottom:12px' }, el('button', { class: 'btn small', type: 'button', onclick: () => autoMod(true) }, 'Turn it on')));
  } else {
    a.replaceChildren(el('p', { class: 'hint', text: auto.problem || '' }));
  }

  // the list of backups
  const list = $('#svBackups');
  const bks = S.backups || [];
  if (!bks.length) { list.replaceChildren(el('div', { class: 'svrow' }, el('span', { class: 'hint', text: 'No backups yet. One is made before every change, or press Back up now.' }))); }
  else list.replaceChildren(...bks.slice(0, 40).map(backupRow));
  if (bks.length > 40) list.append(el('div', { class: 'svrow' }, el('span', { class: 'hint', text: (bks.length - 40) + ' older backups are in the backups folder.' })));
  saveButtons();
}

function backupRow(b) {
  const row = el('div', { class: 'svrow' });
  const normal = () => row.replaceChildren(...[
    el('div', { class: 'what' }, el('b', { text: fmtWhen(b.when) || b.id }), el('small', { text: [b.reason ? (b.auto ? b.reason : 'Made ' + b.reason) : '', b.files.join(' and '), fmtSize(b.bytes)].filter(Boolean).join('  ·  ') })),
    b.files.includes('Campaign.sav') ? el('button', { class: 'btn small', type: 'button', onclick: confirm }, 'Put this back') : null].filter(Boolean));
  const confirm = () => {
    const withSystem = $('#svWithSystem').checked && b.files.includes('System.sav');
    row.replaceChildren(el('div', { class: 'what' }, el('b', { text: 'Put back the save from ' + (fmtWhen(b.when) || b.id) + '?' }),
      el('small', { text: (withSystem ? 'Campaign.sav and System.sav are' : 'Campaign.sav is') + ' replaced by this backup. The save as it is now is backed up first.' })),
      el('button', { class: 'btn small danger', type: 'button', onclick: () => saveJob('/api/save/restore', { id: b.id, withSystem }, 'The backup is back in place.') }, 'Yes, put it back'),
      el('button', { class: 'btn small', type: 'button', onclick: normal }, 'Cancel'));
  };
  normal();
  return row;
}

function saveButtons() {
  const S = SAVE;
  const blocked = !!activeJob || !S || !S.found || !!S.problem || !!S.gameRunning;
  $('#svUnlock').disabled = blocked || !(S && (S.items || []).some(x => !x.unlocked));
  $$('#svVanillaRow button, #svBackups button').forEach(b => { b.disabled = blocked; });
  $('#svBackupNow').disabled = !!activeJob || !S || !S.found;
}

async function saveJob(path, body, okText) {
  const out = $('#svResult'); out.replaceChildren();
  try {
    const res = await runJob(path, body);
    const box = el('div', { class: 'notice good', role: 'status' });
    if (!res.changed) box.append(el('strong', { text: 'Nothing to change: ' }), 'the save already had it that way.');
    else {
      box.append(el('strong', { text: okText }));
      const ul = el('ul', { style: 'margin:6px 0 0 18px' }, (res.lines || []).map(l => el('li', { text: l })));
      box.append(ul);
      if (res.backup && res.backup.id) box.append(el('p', { class: 'hint', text: 'The save as it was before is backed up (' + fmtWhen(res.backup.when) + '), in the list below.' }));
      if (res.cloud) box.append(el('p', { class: 'hint', text: 'Steam Cloud keeps this save. If Steam says the cloud files are in conflict the next time you start the game, keep the files on this computer, not the ones in the cloud.' }));
    }
    out.replaceChildren(box);
  } catch (e) { notice(out, 'error', e.message); }
  out.scrollIntoView({ block: 'nearest' });
  await loadSave();
}

async function autoMod(on) {
  const out = $('#svResult'); out.replaceChildren();
  try { SAVE = await api('/api/save/automod', { on }); renderSave(); notice(out, 'good', on ? 'Your save is now backed up each time the game starts.' : 'The backup at game start is off. Skin Studio still backs the save up before every change it makes.'); }
  catch (e) { notice(out, 'error', e.message); }
}

$('#svUnlock').addEventListener('click', () => saveJob('/api/save/unlock', {}, 'Unlocked. They show as new in the hangar the next time the game starts.'));
$('#svRefresh').addEventListener('click', () => { $('#svResult').replaceChildren(); loadSave(); });
$('#svBackupNow').addEventListener('click', async () => {
  const out = $('#svResult');
  try { const b = await api('/api/save/backup', {}); notice(out, 'good', el('strong', { text: 'Backed up. ' }), fmtWhen(b.when) + ', ' + b.files.join(' and ') + '.'); }
  catch (e) { notice(out, 'error', e.message); }
  loadSave();
});
$('#svOpen').addEventListener('click', () => api('/api/open', { path: SAVE && SAVE.backupsDir }).catch(() => notice($('#svResult'), 'warn', 'There are no backups yet. Press Back up now to make one.')));
