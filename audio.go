package main

import (
	"github.com/oleiade/lane"
	"github.com/gorilla/websocket"
	"github.com/1lann/dissonance/audio"
	"io"
	"github.com/tmpim/juroku/dfpwm"
	"log"
	"github.com/1lann/dissonance/ffmpeg"
)

var (
	audioInstances = map[string]*AudioInstance{}
)

const (
	channels   int = 1
	frameRate  int = 48000
	bufferSize int = 6000
)

// AudioInstance is created for each connected client
type AudioInstance struct {
	connection   *websocket.Conn
	queue        *lane.Queue
	audio        audio.Stream
	clientID     string
	skip         bool
	stop         bool
	trackPlaying bool
}

func (ai *AudioInstance) playTrack(audioFile io.Reader) {
	audio, err := ffmpeg.NewFFMPEGStream(audioFile, false)
	if err != nil {
		log.Fatal("ffmpeg:", err)
		return
	}
	ai.audio = audio

	rd, wr := io.Pipe()
	defer rd.Close()

	go func() {
		dfpwm.EncodeDFPWM(wr, ai.audio)
		wr.Close()
	}()

	buf := make([]byte, bufferSize)
	for {
		n, err := rd.Read(buf[:bufferSize])
		if err == io.EOF || err == io.ErrUnexpectedEOF || ai.stop || ai.skip {
			if a, ok := audioFile.(io.ReadCloser); ok {
				a.Close()
			}
			rd.Close()
			return
		}
		//log.Printf("Read %d bytes from buffer", n)
		ai.connection.WriteMessage(websocket.BinaryMessage, buf[:n])
		//time.Sleep(time.Duration(n / 6) * time.Millisecond)
		//log.Printf("Sleeping %d ms", n / 6)
	}
}

func (ai *AudioInstance) processQueue() {
	if ai.trackPlaying == false {
		for {
			ai.skip = false
			track := ai.queue.Dequeue()
			if ai.stop == true {
				break
			} else if track != nil {
				ai.playTrack(track.(io.Reader))
			}
		}
	}
}

func (ai *AudioInstance) Enqueue(track io.Reader) {
	ai.queue.Enqueue(track)
}

func (ai *AudioInstance) Stop() {
	ai.stop = true
	delete(audioInstances, ai.clientID)
}

func (ai *AudioInstance) Skip() {
	ai.skip = true
}

func CreateAudioInstance(connection *websocket.Conn, clientID string) {
	ai := new (AudioInstance)
	ai.stop = false
	ai.skip = false
	ai.trackPlaying = false
	ai.connection = connection
	audioInstances[clientID] = ai
	ai.clientID = clientID
	ai.queue = lane.NewQueue()

	log.Printf("Begin processing queue for %s...", clientID)

	ai.processQueue()
}