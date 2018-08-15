package main

import (
	"github.com/gorilla/websocket"
	"github.com/paked/configure"
	"os"
	"log"
	"net/http"
	"html/template"
	"github.com/nu7hatch/gouuid"
	"io"
	"github.com/justync7/librespot-golang/src/librespot/core"
	"strings"
	"github.com/justync7/librespot-golang/src/librespot"
	"io/ioutil"
	"github.com/justync7/librespot-golang/src/librespot/utils"
	"github.com/justync7/librespot-golang/src/Spotify"
)

const (
	devicename = "librespot"
)

var (
	conf     = configure.New()
	addr     = conf.String("address", "localhost:8080", "http address")
	spotifyEnabled = conf.Bool("spotify", false, "spotify toggle")
	username = conf.String("username", "", "spotify username")
	password = conf.String("password", "", "spotify password")
	blob     = conf.String("blob", "blob.bin", "spotify blob path")
	upgrader = websocket.Upgrader{}
	spotify  *core.Session
)

func main() {
	conf.Use(configure.NewFlag())
	conf.Use(configure.NewEnvironment())
	if _, err := os.Stat("config.json"); err == nil {
		conf.Use(configure.NewJSONFromFile("config.json"))
	}
	conf.Parse()

	if *spotifyEnabled {
		if *username == "" {
			log.Println("No Spotify username was provided.")
			os.Exit(1)
		}

		if !exists(*blob) && *password == "" {
			log.Println("No Spotify password provided and auth blob does not exist.")
			os.Exit(1)
		}

		if *username != "" && *password != "" {
			// Authenticate using a regular login and password, and store it in the blob file.
			session, err := librespot.Login(*username, *password, devicename)

			if err != nil {
				log.Println("Error opening Spotify session: ", err)
				os.Exit(1)
			}

			err = ioutil.WriteFile(*blob, session.ReusableAuthBlob(), 0600)

			if err != nil {
				log.Printf("Could not store authentication blob in %s: %s\n", *blob, err)
				os.Exit(1)
			}
		} else if *blob != "" && *username != "" {
			// Authenticate reusing an existing blob
			blobBytes, err := ioutil.ReadFile(*blob)

			if err != nil {
				log.Printf("Unable to read auth blob from %s: %s\n", *blob, err)
				os.Exit(1)
			}

			spotify, err = librespot.LoginSaved(*username, blobBytes, devicename)
		}
	} else {
		log.Println("Spotify features disabled")
	}

	log.SetFlags(0)
	http.HandleFunc("/stream", stream)
	http.HandleFunc("/", root)
	log.Printf("Listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func stream(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	u, err := uuid.NewV4()
	if err != nil {
		log.Fatal("uuid:", err)
		return
	}
	clientID := u.String()
	log.Printf("%s connected to %s from %s", clientID, r.Host, c.RemoteAddr().String())

	go CreateAudioInstance(c, clientID)

	for {
		mt, message, err := c.ReadMessage()
		if err == io.EOF || err == io.ErrUnexpectedEOF || websocket.IsCloseError(err, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
			audioInstances[clientID].Stop()
			log.Printf("%s disconnected", clientID)
			break
		} else if err != nil {
			log.Println("read:", err)
			break
		}

		if mt == websocket.TextMessage {
			str := string(message)
			args := strings.Split(str, " ")
			log.Printf("%s: %s", clientID, str)

			if len(args) > 1 && args[0] == "track" && *spotifyEnabled {
				log.Printf("Streaming track %s for %s", args[1], clientID)
				track, err := spotify.Mercury().GetTrack(utils.Base62ToHex(args[1]))
				if err != nil {
					log.Fatal("track:", err)
				} else {
					var chosen *Spotify.AudioFile

					for {
						var afs []*Spotify.AudioFile

						if len(track.GetFile()) > 0 {
							afs = track.GetFile()
						} else if len(track.GetAlternative()) > 0 {
							t, err := spotify.Mercury().GetTrack(utils.Base62ToHex(utils.ConvertTo62(track.GetAlternative()[0].GetGid())))
							if err != nil {
								log.Println("Failed to get track")
								return
							}
							track = t

							afs = t.GetFile()
						} else {
							log.Println("Track had no suitable audio files")
							return
						}

						for _, file := range afs {
							if(file.GetFormat() == Spotify.AudioFile_OGG_VORBIS_320) {
								chosen = file
							} else if file.GetFormat() == Spotify.AudioFile_OGG_VORBIS_160 && chosen == nil {
								chosen = file
							}
						}

						if chosen == nil {
							log.Println("Track had no suitable audio files")
							return
						} else {
							break
						}
					}

					log.Printf("Playing track %s - %s\n", track.GetArtist()[0].GetName(), track.GetName())

					audioFile, err := spotify.Player().LoadTrack(chosen, track.GetGid())
					if err != nil {
						log.Fatal("audiofile:", err)
					} else {
						audioInstances[clientID].Enqueue(audioFile)
					}
				}
			} else if len(args) > 1 && args[0] == "stream" {
				resp, err := http.Get(args[1])
				if err != nil {
					log.Fatal(err)
				} else if resp.StatusCode == 200 {
					audioInstances[clientID].Enqueue(resp.Body)
				}
			}
/*			err = c.WriteMessage(mt, message)
			if err != nil {
				log.Println("write:", err)
				break
			}*/
		}
	}

	c.Close()
}

var rootTemplate = template.Must(template.New("").Parse("{{.}}"))

func root(w http.ResponseWriter, r *http.Request) {
	rootTemplate.Execute(w, "ws://"+r.Host+"/stream")
}

func exists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	} else {
		return false
	}
}