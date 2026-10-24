-- SkinStudioSaveBackup 1.0 (UE4SS Lua mod for ACE COMBAT 8), made by ACE8 Skin Studio.
--
-- Each time the game starts, this copies the campaign save (Campaign.sav) and the settings save
-- (System.sav) into the folder SkinStudioSaveBackups\auto next to the game's SaveGames folder:
--   %LOCALAPPDATA%\BANDAI NAMCO Entertainment\ACE COMBAT 8\Saved\SkinStudioSaveBackups\auto
-- It keeps the last 20 starts (01 to 20, then 01 again). ACE8 Skin Studio can put any of them
-- back: Save tools > Restore a backup. It only reads the save; it never changes it.
-- If ACE8 Skin Studio was told the save is somewhere else, it writes that into paths.txt here.

local TAG = "[SkinStudioSaveBackup] "
local KEEP = 20
local function log(s) print(TAG .. tostring(s) .. "\n") end

local function readAll(p)
    local f = io.open(p, "rb")
    if not f then return nil end
    local d = f:read("*a")
    f:close()
    return d
end

local function writeAll(p, d)
    local f, err = io.open(p, "wb")
    if not f then return false, err end
    local ok, err2 = f:write(d)
    f:close()
    if not ok then return false, err2 end
    return true
end

local function slash(p)
    if p:sub(-1) ~= "\\" and p:sub(-1) ~= "/" then return p .. "\\" end
    return p
end

local function run()
    if type(io) ~= "table" or type(io.open) ~= "function" then
        log("this UE4SS build cannot read files; no backup made")
        return
    end
    local from, to
    local here
    do
        local ok, info = pcall(function() return debug.getinfo(1, "S") end)
        if ok and info and type(info.source) == "string" then here = info.source:match("^@?(.*[\\/])") end
    end
    local custom = here and readAll(here .. "paths.txt")
    if custom then
        local a, b = custom:match("([^\r\n]+)\r?\n([^\r\n]+)")
        if a and b then from, to = slash(a), slash(b) end
    end
    if not from then
        local base = os.getenv("LOCALAPPDATA")
        if not base or base == "" then
            log("LOCALAPPDATA is not set; no backup made")
            return
        end
        local saved = slash(base) .. "BANDAI NAMCO Entertainment\\ACE COMBAT 8\\Saved\\"
        from, to = saved .. "SaveGames\\", saved .. "SkinStudioSaveBackups\\auto\\"
    end

    local n = tonumber((readAll(to .. "next.txt") or ""):match("%d+") or "") or 1
    if n < 1 or n > KEEP then n = 1 end
    local slot = string.format("%02d", n)
    local made = 0
    for _, name in ipairs({ "Campaign.sav", "System.sav" }) do
        local d = readAll(from .. name)
        if d and #d > 0 then
            local ok, err = writeAll(to .. slot .. " " .. name, d)
            if not ok then
                log("could not write the backup (" .. tostring(err) .. "). Open ACE8 Skin Studio's Save tools once to make the folder " .. to)
                return
            end
            made = made + 1
        end
    end
    if made == 0 then
        log("no save found in " .. from .. " (a new game?); no backup made")
        return
    end
    writeAll(to .. slot .. ".txt", os.date("%Y-%m-%d %H:%M:%S") .. "\n")
    writeAll(to .. "next.txt", tostring(n % KEEP + 1) .. "\n")
    log("save backed up as " .. slot .. " in " .. to)
end

local ok, err = pcall(run)
if not ok then log("the backup failed: " .. tostring(err)) end
