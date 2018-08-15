local args = {...}
if #args < 1 then
    printError("Usage: stream <endpoint>\nExample: stream ws://localhost:8080/stream")
    return
end


local ws

local ok, endpoint = http.websocketAsync(args[1])
local data = {}
local written = 0
local frames = 0
local tickTimer
local tickTime = 0.05
local textBuffer = ""

local function tape(side)
    return {
        side = side,
        peripheral = peripheral.wrap(side),
        playing = false,
        length = 0,
        queued = false
    }
end
local tapes = {tape("right")}

local queue = ""

local function popQueue(len)
    local slice = queue:sub(1, len)
    queue = queue:sub(len + 1)
    return slice
end

local function pushQueue(o)
    queue = queue .. o
end

local function isPlaying()
    for k, v in pairs(tapes) do
        if v.playing == true then
            local position = v.peripheral.getPosition()
            --print(position.."/"..v.length)
            if position >= v.length then
                v.playing = false
                v.queued = false
                if #queue == 0 then
                    v.peripheral.stop()
                    v.peripheral.seek(-v.peripheral.getPosition())
                    v.peripheral.write(string.rep("\00",v.peripheral.getSize()))
                end
                return false
            else
                return true
            end
        end
    end
    return false
end

local function canQueue()
    for k, v in pairs(tapes) do
        if v.queued == false then
            return true
        end
    end
    return false
end

local function split(str, pat)
    local t = {}  -- NOTE: use {n = 0} in Lua-5.0
    local fpat = "(.-)" .. pat
    local last_end = 1
    local s, e, cap = str:find(fpat, 1)
    while s do
       if s ~= 1 or cap ~= "" then
          table.insert(t,cap)
       end
       last_end = e+1
       s, e, cap = str:find(fpat, last_end)
    end
    if last_end <= #str then
       cap = str:sub(last_end)
       table.insert(t, cap)
    end
    return t
 end

term.setCursorBlink(true)

if ok then
    while true do
        local e = {os.pullEventRaw()}
        local event = e[1]

        if event == "websocket_success" then
            print("Connected!")
            ws = e[3]
            tickTimer = os.startTimer(tickTime)
        elseif event == "websocket_failure" then
            printError("Failed to connect.")
        elseif event == "websocket_message" then
            local frame = e[3]
            frames = frames + 1
            local size = #frame
            written = written + size
            --print("Got "..frames.." frames [size "..size.."]")
            pushQueue(frame)
        elseif event == "terminate" then
            ws.close()
            if #textBuffer > 0 then
                write("\n")
            end
            print("Closing socket and seeking drive to beginning...")
            break
        elseif event == "timer" then
            local timer = e[2]

            if timer == tickTimer then
                --print("Tick")
                local queueLen = #queue
                if canQueue() and queueLen > 0 then
                    for k, v in pairs(tapes) do
                        if v.queued == false then
                            local len = queueLen < v.peripheral.getSize() and queueLen or v.peripheral.getSize()
                            if queueLen > 500 then
                                len = len - (len % 500)
                            end
                            local frame = popQueue(len)
                            v.peripheral.seek(-v.peripheral.getPosition())
                            v.peripheral.write(frame)
                            v.peripheral.seek(-v.peripheral.getPosition())
                            v.queued = true
                            v.length = #frame
                            --print("Queueing "..v.length.." bytes on "..v.side)
                            break
                        end
                    end
                end

                if not isPlaying() then
                    for k, v in pairs(tapes) do
                        if v.queued == true then
                            v.peripheral.play()
                            v.playing = true
                            --print("Playing "..v.length.." bytes on "..v.side)
                            break
                        end
                    end
                end
                tickTimer = os.startTimer(tickTime)
            end
        elseif event == "char" then
            textBuffer = textBuffer..e[2]
            write(e[2])
        elseif event == "key" then
            if e[2] == keys.enter then
                local cmds = split(textBuffer, " ")

                if (cmds[1] == "track" or cmds[1] == "stream") and #cmds > 1 then
                    if cmds[2]:sub(1, 14) == "spotify:track:" then
                        cmds[2] = cmds[2]:sub(15)
                    end
                    ws.send(table.concat(cmds, " "))
                end
                textBuffer = ""
                write("\n")
            end
        elseif event == "paste" then
            textBuffer = textBuffer..e[2]
            write(e[2])
        end
    end
end