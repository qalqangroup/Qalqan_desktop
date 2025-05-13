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
	sampleRate  = 48000
	channels    = 1   // теперь моно
	frameSize   = 960 // 20 ms @48 kHz
	opusBufSize = 4000
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
	cfg := loadConfig("config.json")
	client := mustLogin(cfg)
	myUserID = client.UserID
	myPartyID = uuid.New().String()

	if err := portaudio.Initialize(); err != nil {
		panic(err)
	}
	defer portaudio.Terminate()

	pc := mustCreatePeerConnection()

	enc, err := gopus.NewEncoder(sampleRate, channels, gopus.Voip)
	if err != nil {
		panic(err)
	}

	sendTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: sampleRate,
			Channels:  channels,
		},
		"audio", "matrix-send",
	)
	if err != nil {
		panic(err)
	}
	if _, err := pc.AddTrack(sendTrack); err != nil {
		panic(err)
	}

	// Моно-микрофон: 1 входной канал
	in := make([]int16, frameSize*channels)
	micStream, err := portaudio.OpenDefaultStream(
		channels, // inputChannels
		0,        // outputChannels
		sampleRate,
		frameSize,
		in,
	)
	if err != nil {
		panic(err)
	}
	defer micStream.Close()
	if err := micStream.Start(); err != nil {
		panic(err)
	}

	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if err := micStream.Read(); err != nil {
				fmt.Println("mic read error:", err)
				continue
			}
			opusPacket, err := enc.Encode(in, frameSize, opusBufSize)
			if err != nil {
				fmt.Println("gopus encode error:", err)
				continue
			}
			if err := sendTrack.WriteSample(media.Sample{
				Data:     opusPacket,
				Duration: 20 * time.Millisecond,
			}); err != nil {
				fmt.Println("WriteSample error:", err)
			}
		}
	}()

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
		playStream, err := portaudio.OpenDefaultStream(
			0,        // inputChannels
			channels, // outputChannels
			sampleRate,
			frameSize,
			out,
		)
		if err != nil {
			panic(err)
		}
		defer playStream.Close()
		if err := playStream.Start(); err != nil {
			panic(err)
		}
		fmt.Println("🔊 Remote audio started")

		for {
			packet, _, err := track.ReadRTP()
			if err != nil {
				return
			}
			sb.Push(packet)
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

	// 8) Matrix signaling
	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	// 8a) Outgoing ICE candidates
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		ice := c.ToJSON()
		client.SendMessageEvent(
			context.Background(),
			id.RoomID(cfg.RoomID),
			event.CallCandidates,
			map[string]interface{}{
				"call_id":  currentCallID,
				"party_id": myPartyID,
				"version":  "1",
				"candidates": []interface{}{map[string]interface{}{
					"candidate":     ice.Candidate,
					"sdpMid":        ice.SDPMid,
					"sdpMLineIndex": ice.SDPMLineIndex,
				}},
			},
		)
	})

	// 8b) Auto-answer incoming invite
	syncer.OnEventType(event.CallInvite, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == myUserID || pc.SignalingState() != webrtc.SignalingStateStable {
			return
		}
		raw := ev.Content.Raw
		callID := raw["call_id"].(string)
		offer := raw["offer"].(map[string]interface{})["sdp"].(string)
		remote := raw["party_id"].(string)
		currentCallID = callID

		if err := pc.SetRemoteDescription(
			webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer},
		); err != nil {
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

		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallAnswer, map[string]interface{}{
			"call_id":  callID,
			"party_id": myPartyID,
			"version":  "1",
			"answer":   map[string]interface{}{"type": answer.Type.String(), "sdp": answer.SDP},
		})
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallSelectAnswer, map[string]interface{}{
			"call_id":           callID,
			"party_id":          myPartyID,
			"selected_party_id": remote,
			"version":           "1",
		})
		fmt.Println("✔️ Auto-answered call", callID)
	})

	// 8c) Handle answer to our invite
	syncer.OnEventType(event.CallAnswer, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == myUserID ||
			pc.SignalingState() != webrtc.SignalingStateHaveLocalOffer {
			return
		}
		raw := ev.Content.Raw
		if raw["call_id"].(string) != currentCallID {
			return
		}
		sdp := raw["answer"].(map[string]interface{})["sdp"].(string)
		if err := pc.SetRemoteDescription(
			webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp},
		); err != nil {
			fmt.Println("SetRemoteDescription error:", err)
			return
		}
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallSelectAnswer, map[string]interface{}{
			"call_id":           currentCallID,
			"party_id":          myPartyID,
			"selected_party_id": raw["party_id"].(string),
			"version":           "1",
		})
		fmt.Println("✔️ Call established with", raw["party_id"].(string))
	})

	// 8d) Handle remote ICE candidates
	syncer.OnEventType(event.CallCandidates, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == myUserID {
			return
		}
		raw := ev.Content.Raw
		if raw["call_id"].(string) != currentCallID {
			return
		}
		for _, ci := range raw["candidates"].([]interface{}) {
			m := ci.(map[string]interface{})
			pc.AddICECandidate(webrtc.ICECandidateInit{
				Candidate:     m["candidate"].(string),
				SDPMid:        toPtrString(m["sdpMid"].(string)),
				SDPMLineIndex: toPtrUint16(m["sdpMLineIndex"].(float64)),
			})
		}
	})

	// 9) Start sync loop
	go func() {
		if err := client.Sync(); err != nil {
			panic(err)
		}
	}()

	// 10) Initiate call
	currentCallID = fmt.Sprintf("call-%d", time.Now().Unix())
	offer, invite := buildOffer(currentCallID, pc)
	invite["version"] = "1"
	invite["party_id"] = myPartyID
	client.SendMessageEvent(
		context.Background(),
		id.RoomID(cfg.RoomID),
		event.CallInvite,
		invite,
	)
	fmt.Printf("🔔 Sent m.call.invite — waiting… SDP len=%d\n", len(offer.SDP))

	// block forever
	select {}
}

func buildOffer(callID string, pc *webrtc.PeerConnection) (webrtc.SessionDescription, map[string]interface{}) {
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		panic(err)
	}
	pc.SetLocalDescription(offer)
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
	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		fmt.Println("ICE Connection State:", s)
	})
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Println("Peer Connection State:", s)
	})
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

func toPtrString(s string) *string  { return &s }
func toPtrUint16(f float64) *uint16 { u := uint16(f); return &u }
