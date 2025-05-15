package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
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

func startCalling() {
	// Load config and login
	cfg := loadConfig("config.json")
	client := mustLogin(cfg)
	myUserID = client.UserID
	myPartyID = uuid.NewString()

	// Init PortAudio
	if err := portaudio.Initialize(); err != nil {
		log.Fatalf("PortAudio init error: %v", err)
	}
	defer portaudio.Terminate()

	// Enumerate audio devices
	hostApis, err := portaudio.HostApis()
	if err != nil {
		log.Fatalf("HostApis error: %v", err)
	}
	for _, host := range hostApis {
		log.Printf("Host API: %s", host.Name)
		for _, dev := range host.Devices {
			log.Printf("  Device: %s | in:%d out:%d", dev.Name, dev.MaxInputChannels, dev.MaxOutputChannels)
		}
	}

	// Select USB mic and speaker
	var inputDev, outputDev *portaudio.DeviceInfo
	for _, host := range hostApis {
		for _, dev := range host.Devices {
			name := strings.ToLower(dev.Name)
			if inputDev == nil && strings.Contains(name, "usb") && dev.MaxInputChannels >= channels {
				inputDev = dev
			}
			if outputDev == nil && strings.Contains(name, "usb") && dev.MaxOutputChannels >= channels {
				outputDev = dev
			}
		}
	}
	if inputDev == nil || outputDev == nil {
		log.Fatalf("Failed to find USB mic or speaker: in=%v out=%v", inputDev, outputDev)
	}

	// PCM channels
	dataCh := make(chan []int16, 50)
	decodeCh := make(chan []int16, 50)

	// Input stream
	inParams := portaudio.LowLatencyParameters(inputDev, nil)
	inParams.Input.Channels = channels
	inParams.SampleRate = sampleRate
	inParams.FramesPerBuffer = frameSize
	micStream, err := portaudio.OpenStream(inParams, func(inBuf []int16) {
		frame := append([]int16(nil), inBuf...)
		select {
		case dataCh <- frame:
		default:
		}
	})
	if err != nil {
		log.Fatalf("OpenStream input error: %v", err)
	}
	defer micStream.Close()
	if err := micStream.Start(); err != nil {
		log.Fatalf("Start input stream error: %v", err)
	}
	log.Println("🎙 Mic stream started on", inputDev.Name)

	// Output stream
	outParams := portaudio.LowLatencyParameters(nil, outputDev)
	outParams.Output.Channels = channels
	outParams.SampleRate = sampleRate
	outParams.FramesPerBuffer = frameSize
	playStream, err := portaudio.OpenStream(outParams, func(outBuf []int16) {
		select {
		case pcm := <-decodeCh:
			copy(outBuf, pcm)
		default:
		}
	})
	if err != nil {
		log.Fatalf("OpenStream output error: %v", err)
	}
	defer playStream.Close()
	if err := playStream.Start(); err != nil {
		log.Fatalf("Start output stream error: %v", err)
	}
	log.Println("🔊 Playback stream started on", outputDev.Name)

	// Create PeerConnection
	pc := mustCreatePeerConnection()

	// Opus encoder and sendTrack
	enc, err := gopus.NewEncoder(sampleRate, channels, gopus.Voip)
	if err != nil {
		log.Fatalf("gopus NewEncoder error: %v", err)
	}
	sendTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: sampleRate, Channels: channels},
		"matrix-send", "audio",
	)
	if err != nil {
		log.Fatalf("NewTrackLocalStaticSample error: %v", err)
	}
	if _, err := pc.AddTrack(sendTrack); err != nil {
		log.Fatalf("AddTrack error: %v", err)
	}

	// Send loop
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			go func() {
				for pcm := range dataCh {
					pkt, err := enc.Encode(pcm, frameSize, opusBufSize)
					if err != nil {
						log.Println("encode error:", err)
						continue
					}
					log.Printf("sending %d bytes of Opus", len(pkt))
					if err := sendTrack.WriteSample(media.Sample{Data: pkt, Duration: 20 * time.Millisecond}); err != nil {
						log.Println("WriteSample error:", err)
					}
				}
			}()
		}
	})

	// Receive loop
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeAudio {
			return
		}
		sb := samplebuilder.New(10, &codecs.OpusPacket{}, track.Codec().ClockRate)
		dec, err := gopus.NewDecoder(sampleRate, channels)
		if err != nil {
			log.Fatalf("gopus NewDecoder error: %v", err)
		}
		go func() {
			for {
				rtpPkt, _, err := track.ReadRTP()
				if err != nil {
					return
				}
				sb.Push(rtpPkt)
				for s := sb.Pop(); s != nil; s = sb.Pop() {
					pcm, err := dec.Decode(s.Data, frameSize, false)
					if err != nil {
						log.Println("decode error:", err)
						break
					}
					select {
					case decodeCh <- pcm:
					default:
					}
				}
			}
		}()
	})

	// Signaling
	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) { log.Println("ICE state:", s) })
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		ice := c.ToJSON()
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

	syncer := client.Syncer.(*mautrix.DefaultSyncer)
	// Remote ICE
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
				m := it.(map[string]interface{})
				cand := m["candidate"].(string)
				mid := m["sdpMid"].(string)
				idx := uint16(m["sdpMLineIndex"].(float64))
				_ = pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: cand, SDPMid: &mid, SDPMLineIndex: &idx})
			}
		}
	})

	// Handle invite
	syncer.OnEventType(event.CallInvite, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == myUserID || pc.SignalingState() != webrtc.SignalingStateStable {
			return
		}
		raw := ev.Content.Raw
		cid, ok1 := raw["call_id"].(string)
		off, ok2 := raw["offer"].(map[string]interface{})
		sdp, ok3 := off["sdp"].(string)
		party, ok4 := raw["party_id"].(string)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return
		}
		currentCallID = cid
		pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp})
		ans, _ := pc.CreateAnswer(nil)
		pc.SetLocalDescription(ans)
		<-webrtc.GatheringCompletePromise(pc)
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallAnswer, map[string]interface{}{
			"call_id":  currentCallID,
			"party_id": myPartyID,
			"version":  "1",
			"answer":   map[string]interface{}{"type": ans.Type.String(), "sdp": ans.SDP},
		})
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallSelectAnswer, map[string]interface{}{
			"call_id": currentCallID, "party_id": myPartyID, "selected_party_id": party, "version": "1",
		})
	})

	// Handle answer to our invite
	syncer.OnEventType(event.CallAnswer, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == myUserID || pc.SignalingState() != webrtc.SignalingStateHaveLocalOffer {
			return
		}
		raw := ev.Content.Raw
		cid, ok1 := raw["call_id"].(string)
		amap, ok2 := raw["answer"].(map[string]interface{})
		sdp, ok3 := amap["sdp"].(string)
		party, ok4 := raw["party_id"].(string)
		if !ok1 || !ok2 || !ok3 || !ok4 || cid != currentCallID {
			return
		}
		pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp})
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallSelectAnswer, map[string]interface{}{
			"call_id": currentCallID, "party_id": myPartyID, "selected_party_id": party, "version": "1",
		})
	})

	// Start sync
	go func() {
		for {
			if err := client.Sync(); err != nil {
				log.Println("Sync error:", err)
				time.Sleep(syncRetryGap)
			}
		}
	}()

	// Call initiation
	currentCallID = fmt.Sprintf("call-%d", time.Now().Unix())
	offer, invite := buildOffer(currentCallID, pc)
	invite["version"] = "1"
	invite["party_id"] = myPartyID
	client.SendMessageEvent(context.Background(), id.RoomID(cfg.RoomID), event.CallInvite, invite)
	log.Printf("Sent invite, SDP len=%d", len(offer.SDP))

	select {} // block forever
}

func mustCreatePeerConnection() *webrtc.PeerConnection {
	conf := webrtc.Configuration{ICETransportPolicy: webrtc.ICETransportPolicyAll, ICEServers: []webrtc.ICEServer{
		{URLs: []string{"stun:stun.l.google.com:19302"}},
		{URLs: []string{"turn:webqalqan.com:3478"}, Username: "turnuser", Credential: "turnpass"},
	}}
	pc, err := webrtc.NewPeerConnection(conf)
	if err != nil {
		log.Fatalf("NewPeerConnection error: %v", err)
	}
	return pc
}

func mustLogin(cfg Config) *mautrix.Client {
	client, err := mautrix.NewClient(cfg.Homeserver, "", "")
	if err != nil {
		log.Fatalf("mautrix NewClient error: %v", err)
	}
	resp, err := client.Login(context.Background(), &mautrix.ReqLogin{Type: "m.login.password", Identifier: mautrix.UserIdentifier{Type: "m.id.user", User: cfg.Username}, Password: cfg.Password})
	if err != nil {
		log.Fatalf("Login error: %v", err)
	}
	client.SetCredentials(resp.UserID, resp.AccessToken)
	return client
}

func loadConfig(path string) Config {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("Open config error: %v", err)
	}
	defer f.Close()
	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		log.Fatalf("Decode config error: %v", err)
	}
	return cfg
}

func buildOffer(callID string, pc *webrtc.PeerConnection) (webrtc.SessionDescription, map[string]interface{}) {
	off, err := pc.CreateOffer(nil)
	if err != nil {
		log.Fatalf("CreateOffer error: %v", err)
	}
	if err := pc.SetLocalDescription(off); err != nil {
		log.Fatalf("SetLocalDescription error: %v", err)
	}
	<-webrtc.GatheringCompletePromise(pc)
	invite := map[string]interface{}{"call_id": callID, "lifetime": 60000, "offer": map[string]interface{}{"type": off.Type.String(), "sdp": off.SDP}}
	return off, invite
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
