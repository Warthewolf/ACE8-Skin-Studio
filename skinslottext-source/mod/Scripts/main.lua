-- SkinSlotText 1.2 (UE4SS Lua mod for ACE COMBAT 8, experimental)
-- Loads SkinSlotText.dll from this folder. The DLL gives the game the names and descriptions of new skin slots,
-- read from texts\*.ini (and SkinSlotText.ini) next to it, and passes every other text on unchanged.
-- Its own report is SkinSlotText.log next to the DLL.
--
-- It gets along with RoGText.dll (the OTSS / laser mods): it lets RoGText hook the game's text lookup first and then
-- goes in front of it, so both mods' texts work. This file only loads the DLL and gives it a moment each second, on
-- the game thread, to decide; after about a minute it stops.

local TAG = "[SkinSlotText] "
local function log(s) print(TAG .. tostring(s) .. "\n") end

local dirs = {}
do
    local ok, info = pcall(function() return debug.getinfo(1, "S") end)
    if ok and info and type(info.source) == "string" then
        local d = info.source:match("^@(.*[\\/])")
        if d then dirs[#dirs + 1] = d end
    end
end
dirs[#dirs + 1] = "ue4ss\\Mods\\SkinSlotText\\Scripts\\"
dirs[#dirs + 1] = "Mods\\SkinSlotText\\Scripts\\"

if type(package) ~= "table" or type(package.loadlib) ~= "function" then
    log("this UE4SS build cannot load DLLs (no package.loadlib) - new skin slots keep their raw names")
    return
end

local start, tick, errs = nil, nil, {}
for _, d in ipairs(dirs) do
    local f, err = package.loadlib(d .. "SkinSlotText.dll", "luaopen_skinslottext")
    if f then
        start = f
        tick = package.loadlib(d .. "SkinSlotText.dll", "skinslottext_tick")
        log("SkinSlotText.dll loaded from " .. d .. " - its own report is SkinSlotText.log in the same folder")
        break
    end
    errs[#errs + 1] = d .. ": " .. tostring(err)
end
if not start or not tick then
    log("cannot load SkinSlotText.dll (" .. table.concat(errs, "; ") .. ")")
    return
end

local function onGameThread(fn)
    if type(ExecuteInGameThread) == "function" then ExecuteInGameThread(fn) else fn() end
end

onGameThread(function()
    local ok, e = pcall(start)
    if not ok then log("SkinSlotText.dll failed to start: " .. tostring(e)) end
end)

if type(LoopAsync) ~= "function" then
    log("this UE4SS build has no LoopAsync - the texts cannot be switched on")
    return
end
local n = 0
LoopAsync(1000, function()
    n = n + 1
    onGameThread(function() pcall(tick) end)
    return n >= 75          -- the DLL has decided within about 60 s; stop asking
end)
log("1.2 loaded")
