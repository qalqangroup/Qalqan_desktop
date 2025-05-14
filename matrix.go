package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/gordonklaus/portaudio"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/pion/webrtc/v3/pkg/media/samplebuilder"
	"layeh.com/gopus"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const (
	sampleRate   = 48000
	channels     = 1
	frameSize    = 960
	opusBufSize  = 4000
	syncRetryGap = time.Second
)

var (
	currentCallID string
	myPartyID     string
	myUserID      id.UserID
)

type Config struct {
	Homeserver string `json:"homeserver"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	RoomID     string `json:"room_id"` // если пусто — создаём DM автоматически
}

func startVoiceCall() {
	// 1) Load config and login
	cfg := loadConfig("config.json")
	client := mustLogin(cfg)
	myUserID = client.UserID
	myPartyID = uuid.New().String()

	// 2) Initialize PortAudio
	if err := portaudio.Initialize(); err != nil {
		panic(err)
	}
	defer portaudio.Terminate()

	// 3) Create WebRTC PeerConnection
	pc := mustCreatePeerConnection()

	// 4) Prepare audio encoder and send track
	enc, err := gopus.NewEncoder(sampleRate, channels, gopus.Voip)
	if err != nil {
		panic(err)
	}
	sendTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: sampleRate, Channels: channels},
		"audio", "matrix-send",
	)
	if err != nil {
		panic(err)
	}
	if _, err := pc.AddTrack(sendTrack); err != nil {
		panic(err)
	}

	// 5) Open microphone stream
	in := make([]int16, frameSize*channels)
	micStream, err := portaudio.OpenDefaultStream(channels, 0, sampleRate, frameSize, in)
	if err != nil {
		panic(err)
	}
	defer micStream.Close()
	if err := micStream.Start(); err != nil {
		panic(err)
	}

	// 6) Read from mic, encode, send over WebRTC
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if err := micStream.Read(); err != nil {
				fmt.Println("mic read error:", err)
				continue
			}
			buf, err := enc.Encode(in, frameSize, opusBufSize)
			if err != nil {
				fmt.Println("gopus encode error:", err)
				continue
			}
			if err := sendTrack.WriteSample(media.Sample{Data: buf, Duration: 20 * time.Millisecond}); err != nil {
				fmt.Println("WriteSample error:", err)
			}
		}
	}()

	// 7) Handle incoming audio
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeAudio {
			return
		}
		sb := samplebuilder.New(10, &codecs.OpusPacket{}, track.Codec().ClockRate)
		dec, err := gopus.NewDecoder(sampleRate, channels)
		if err != nil {
			panic(err)
		}
		out := make([]int16, frameSize*channels)
		playStream, err := portaudio.OpenDefaultStream(0, channels, sampleRate, frameSize, out)
		if err != nil {
			panic(err)
		}
		defer playStream.Close()
		if err := playStream.Start(); err != nil {
			panic(err)
		}
		fmt.Println("🔊 Remote audio started")

		for {
			rtpPkt, _, err := track.ReadRTP()
			if err != nil {
				return
			}
			sb.Push(rtpPkt)
			for {
				s := sb.Pop()
				if s == nil {
					break
				}
				decoded, err := dec.Decode(s.Data, frameSize, false)
				if err != nil {
					fmt.Println("gopus decode error:", err)
					break
				}
				copy(out, decoded)
				playStream.Write()
			}
		}
	})

	// 8) Log WebRTC connection states
	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		fmt.Println("ICE Connection State:", s)
	})
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Println("Peer Connection State:", s)
	})

	// 9) Matrix signaling setup
	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	// 9a) Handle incoming ICE candidates
	syncer.OnEventType(event.CallCandidates, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == myUserID {
			return
		}
		raw := ev.Content.Raw
		callID, ok := raw["call_id"].(string)
		if !ok || callID != currentCallID {
			return
		}
		if arr, ok := raw["candidates"].([]interface{}); ok {
			for _, it := range arr {
				if m, ok := it.(map[string]interface{}); ok {
					cand, _ := m["candidate"].(string)
					mid, _ := m["sdpMid"].(string)
					if idxF, ok := m["sdpMLineIndex"].(float64); ok {
						idx := uint16(idxF)
						ice := webrtc.ICECandidateInit{
							Candidate:     cand,
							SDPMid:        &mid,
							SDPMLineIndex: &idx,
						}
						if err := pc.AddICECandidate(ice); err != nil {
							fmt.Println("AddICECandidate error:", err)
						}
					}
				}
			}
		}
	})

	// 9b) Auto-answer incoming invites
	syncer.OnEventType(event.CallInvite, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == myUserID || pc.SignalingState() != webrtc.SignalingStateStable {
			return
		}
		raw := ev.Content.Raw
		callID, ok1 := raw["call_id"].(string)
		offerMap, ok2 := raw["offer"].(map[string]interface{})
		sdp, ok3 := offerMap["sdp"].(string)
		party, ok4 := raw["party_id"].(string)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			fmt.Println("Malformed call.invite")
			return
		}
		currentCallID = callID

		if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}); err != nil {
			fmt.Println("SetRemoteDescription error:", err)
			return
		}
		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			fmt.Println("CreateAnswer error:", err)
			return
		}
		pc.SetLocalDescription(answer)
		<-webrtc.GatheringCompletePromise(pc)

		// Send call.answer
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallAnswer, map[string]interface{}{
			"call_id":  callID,
			"party_id": myPartyID,
			"version":  "1",
			"answer": map[string]interface{}{
				"type": answer.Type.String(),
				"sdp":  answer.SDP,
			},
		})

		// Send call.select_answer
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallSelectAnswer, map[string]interface{}{
			"call_id":           callID,
			"party_id":          myPartyID,
			"selected_party_id": party,
			"version":           "1",
		})
		fmt.Println("✔️ Auto-answered call", callID)
	})

	// 9c) Handle answers to our outgoing invite
	syncer.OnEventType(event.CallAnswer, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == myUserID || pc.SignalingState() != webrtc.SignalingStateHaveLocalOffer {
			return
		}
		raw := ev.Content.Raw
		callID, ok1 := raw["call_id"].(string)
		ansMap, ok2 := raw["answer"].(map[string]interface{})
		sdp, ok3 := ansMap["sdp"].(string)
		party, ok4 := raw["party_id"].(string)
		if !ok1 || !ok2 || !ok3 || !ok4 || callID != currentCallID {
			return
		}
		if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp}); err != nil {
			fmt.Println("SetRemoteDescription error:", err)
			return
		}
		// Send call.select_answer
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallSelectAnswer, map[string]interface{}{
			"call_id":           callID,
			"party_id":          myPartyID,
			"selected_party_id": party,
			"version":           "1",
		})
		fmt.Println("✔️ Call established with", party)
	})

	// 10) Start sync loop (with retry)
	go func() {
		for {
			if err := client.Sync(); err != nil {
				fmt.Println("Matrix sync error:", err)
				time.Sleep(syncRetryGap)
				continue
			}
			break
		}
	}()

	// 11) Initiate outgoing call
	currentCallID = fmt.Sprintf("call-%d", time.Now().Unix())
	offer, invite := buildOffer(currentCallID, pc)
	invite["version"] = "1"
	invite["party_id"] = myPartyID

	client.SendMessageEvent(context.Background(), id.RoomID(cfg.RoomID), event.CallInvite, invite)
	fmt.Printf("🔔 Sent m.call.invite — waiting… SDP len=%d\n", len(offer.SDP))

	// Block to keep running
	select {}
}

func buildOffer(callID string, pc *webrtc.PeerConnection) (webrtc.SessionDescription, map[string]interface{}) {
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		panic(err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		panic(err)
	}
	<-webrtc.GatheringCompletePromise(pc)
	return offer, map[string]interface{}{
		"call_id":  callID,
		"lifetime": 60000,
		"offer": map[string]interface{}{
			"type": offer.Type.String(),
			"sdp":  offer.SDP,
		},
	}
}

func mustCreatePeerConnection() *webrtc.PeerConnection {
	conf := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	}
	pc, err := webrtc.NewPeerConnection(conf)
	if err != nil {
		panic(err)
	}
	pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	return pc
}

func mustLogin(cfg Config) *mautrix.Client {
	client, err := mautrix.NewClient(cfg.Homeserver, "", "")
	if err != nil {
		panic(err)
	}
	resp, err := client.Login(context.Background(), &mautrix.ReqLogin{
		Type:       "m.login.password",
		Identifier: mautrix.UserIdentifier{Type: "m.id.user", User: cfg.Username},
		Password:   cfg.Password,
	})
	if err != nil {
		panic(err)
	}
	client.SetCredentials(resp.UserID, resp.AccessToken)
	return client
}

func loadConfig(path string) Config {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		panic(err)
	}
	return cfg
}

func printRoomMembers(client *mautrix.Client, roomID id.RoomID) {
	resp, err := client.JoinedMembers(context.Background(), roomID)
	if err != nil {
		fmt.Println("JoinedMembers error:", err)
		return
	}
	fmt.Printf("Members in %s:\n", roomID)
	for uid, m := range resp.Joined {
		name := string(uid)
		if m.DisplayName != "" {
			name = fmt.Sprintf("%s (%s)", m.DisplayName, uid)
		}
		fmt.Println(" •", name)
	}
}
