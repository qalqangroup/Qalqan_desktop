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
	frameSize    = 960 // 20ms @48kHz
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
	RoomID     string `json:"room_id"`
}

func startVoiceCall() {
	// 1) Load config & login
	cfg := loadConfig("config.json")
	client := mustLogin(cfg)
	myUserID = client.UserID
	myPartyID = uuid.New().String()

	// 2) Init PortAudio
	if err := portaudio.Initialize(); err != nil {
		panic(err)
	}
	defer portaudio.Terminate()

	// 3) Create PeerConnection (STUN+TURN)
	pc := mustCreatePeerConnection()

	// 4) Prepare Opus encoder & sendTrack
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

	// 5) Open mic stream (blocking read)
	micBuf := make([]int16, frameSize*channels)
	micStream, err := portaudio.OpenDefaultStream(channels, 0, sampleRate, frameSize, micBuf)
	if err != nil {
		panic(err)
	}
	defer micStream.Close()
	if err := micStream.Start(); err != nil {
		panic(err)
	}

	// 6) Only after connected → start sending
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Println("PeerConnection state:", state)
		if state == webrtc.PeerConnectionStateConnected {
			go func() {
				for {
					// blocking read exactly frameSize samples
					if err := micStream.Read(); err != nil {
						fmt.Println("mic read error (overflow?):", err)
						continue
					}
					// encode + send
					pkt, err := enc.Encode(micBuf, frameSize, opusBufSize)
					if err != nil {
						fmt.Println("gopus encode error:", err)
						continue
					}
					if err := sendTrack.WriteSample(media.Sample{Data: pkt, Duration: 20 * time.Millisecond}); err != nil {
						fmt.Println("WriteSample error:", err)
					}
				}
			}()
		}
	})

	// 7) Receive & play remote audio
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeAudio {
			return
		}
		sb := samplebuilder.New(10, &codecs.OpusPacket{}, track.Codec().ClockRate)
		dec, err := gopus.NewDecoder(sampleRate, channels)
		if err != nil {
			panic(err)
		}
		outBuf := make([]int16, frameSize*channels)
		playStream, err := portaudio.OpenDefaultStream(0, channels, sampleRate, frameSize, outBuf)
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
				copy(outBuf, decoded)
				playStream.Write()
			}
		}
	})

	// 8) Debug ICE state
	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		fmt.Println("ICE state:", s)
	})

	// 9) Matrix signaling
	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	// 9a) Outgoing ICE candidates
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		ice := c.ToJSON()
		fmt.Println("→ ICE candidate:", ice.Candidate)
		client.SendMessageEvent(context.Background(), id.RoomID(cfg.RoomID), event.CallCandidates, map[string]interface{}{
			"call_id":  currentCallID,
			"party_id": myPartyID,
			"version":  "1",
			"candidates": []interface{}{map[string]interface{}{
				"candidate":     ice.Candidate,
				"sdpMid":        ice.SDPMid,
				"sdpMLineIndex": ice.SDPMLineIndex,
			}},
		})
	})

	// 9b) Incoming ICE candidates
	syncer.OnEventType(event.CallCandidates, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == myUserID {
			return
		}
		raw := ev.Content.Raw
		if idVal, ok := raw["call_id"].(string); !ok || idVal != currentCallID {
			return
		}
		if arr, ok := raw["candidates"].([]interface{}); ok {
			for _, it := range arr {
				if m, ok := it.(map[string]interface{}); ok {
					cand, _ := m["candidate"].(string)
					mid, _ := m["sdpMid"].(string)
					if idxF, ok := m["sdpMLineIndex"].(float64); ok {
						idx := uint16(idxF)
						pc.AddICECandidate(webrtc.ICECandidateInit{
							Candidate:     cand,
							SDPMid:        &mid,
							SDPMLineIndex: &idx,
						})
					}
				}
			}
		}
	})

	// 9c) Auto-answer incoming invites
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
			fmt.Println("Malformed invite")
			return
		}
		currentCallID = callID
		pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp})
		answer, _ := pc.CreateAnswer(nil)
		pc.SetLocalDescription(answer)
		<-webrtc.GatheringCompletePromise(pc)
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallAnswer, map[string]interface{}{
			"call_id":  callID,
			"party_id": myPartyID,
			"version":  "1",
			"answer": map[string]interface{}{
				"type": answer.Type.String(),
				"sdp":  answer.SDP,
			},
		})
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallSelectAnswer, map[string]interface{}{
			"call_id":           callID,
			"party_id":          myPartyID,
			"selected_party_id": party,
			"version":           "1",
		})
		fmt.Println("Answered call", callID)
	})

	// 9d) Handle answer to our invite
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
		pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp})
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallSelectAnswer, map[string]interface{}{
			"call_id":           callID,
			"party_id":          myPartyID,
			"selected_party_id": party,
			"version":           "1",
		})
		fmt.Println("Established call with", party)
	})

	// 10) Run sync forever
	go func() {
		for {
			if err := client.Sync(); err != nil {
				fmt.Println("Sync error:", err)
				time.Sleep(syncRetryGap)
			}
		}
	}()

	// 11) Initiate call
	currentCallID = fmt.Sprintf("call-%d", time.Now().Unix())
	offer, invite := buildOffer(currentCallID, pc)
	invite["version"] = "1"
	invite["party_id"] = myPartyID
	client.SendMessageEvent(context.Background(), id.RoomID(cfg.RoomID), event.CallInvite, invite)
	fmt.Printf("Sent invite, SDP len=%d\n", len(offer.SDP))

	// block forever
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
		ICETransportPolicy: webrtc.ICETransportPolicyAll,
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{
				URLs:       []string{"turn:webqalqan.com:3478"},
				Username:   "turnuser",
				Credential: "turnpass",
			},
		},
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
