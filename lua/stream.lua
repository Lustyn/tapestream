local args = {...}
local ws

local ok, endpoint = http.websocketAsync("ws://localhost:8080/stream")
local data = {}
local written = 0
local frames = 0
local tickTimer
local tickTime = 0.05

local function tape(side)
    return {
        side = side,
        peripheral = peripheral.wrap(side),
        playing = false,
        length = 0,
        queued = false
    }
end
local tapes = {tape("left"),tape("right")}

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
            if position >= v.length then
                v.peripheral.stop()
                v.playing = false
                v.queued = false
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

if ok then
    while true do
        local e = {os.pullEventRaw()}
        local event = e[1]

        if event == "websocket_success" then
            print("Connected!")
            ws = e[3]
            ws.send("track "..args[1])
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
                            local frame = popQueue(len)
                            v.peripheral.seek(-v.peripheral.getPosition())
                            v.peripheral.write(frame)
                            v.peripheral.seek(-v.peripheral.getPosition())
                            v.queued = true
                            v.length = #frame
                            print("Queueing "..v.length.." bytes on "..v.side)
                            break
                        end
                    end
                end

                if not isPlaying() then
                    for k, v in pairs(tapes) do
                        if v.queued == true then
                            v.peripheral.play()
                            v.playing = true
                            print("Playing "..v.length.." bytes on "..v.side)
                            break
                        end
                    end
                end
                tickTimer = os.startTimer(tickTime)
            end
        end
    end
end