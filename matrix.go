package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Config holds JSON configuration for the Matrix homeserver and user credentials.
type Config struct {
	Homeserver string `json:"homeserver"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	RoomID     string `json:"room_id"`
}

var (
	currentCallID string
	myPartyID     string
	myUserID      id.UserID
)

// startVoiceCall initializes Matrix client, WebRTC peer connection,
// and orchestrates the full VoIP handshake (invite, answer, select_answer, ICE).
func startVoiceCall() {
	// Load configuration and login
	cfg := loadConfig("config.json")
	client := mustLogin(cfg)
	myUserID = client.UserID
	myPartyID = uuid.New().String()

	// Create WebRTC peer connection with STUN
	pc := mustCreatePeerConnection()
	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	// 1) Send local ICE candidates to Matrix room
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		ice := c.ToJSON()
		eventContent := map[string]interface{}{
			"call_id":  currentCallID,
			"party_id": myPartyID,
			"version":  "1",
			"candidates": []interface{}{map[string]interface{}{
				"candidate":     ice.Candidate,
				"sdpMid":        ice.SDPMid,
				"sdpMLineIndex": ice.SDPMLineIndex,
			}},
		}
		client.SendMessageEvent(context.Background(), id.RoomID(cfg.RoomID), event.CallCandidates, eventContent)
	})

	// 2) Auto-answer incoming invite if PC is stable
	syncer.OnEventType(event.CallInvite, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == myUserID || pc.SignalingState() != webrtc.SignalingStateStable {
			return
		}
		raw := ev.Content.Raw
		callID, _ := raw["call_id"].(string)
		offerMap, _ := raw["offer"].(map[string]interface{})
		sdp, _ := offerMap["sdp"].(string)
		remoteParty, _ := raw["party_id"].(string)

		currentCallID = callID

		// Apply remote offer
		if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}); err != nil {
			fmt.Println("SetRemoteDescription error:", err)
			return
		}

		// Create answer
		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			fmt.Println("CreateAnswer error:", err)
			return
		}
		pc.SetLocalDescription(answer)
		<-webrtc.GatheringCompletePromise(pc)

		// Send m.call.answer
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallAnswer, map[string]interface{}{
			"call_id":  callID,
			"party_id": myPartyID,
			"version":  "1",
			"answer": map[string]interface{}{
				"type": answer.Type.String(),
				"sdp":  answer.SDP,
			},
		})

		// Send m.call.select_answer
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallSelectAnswer, map[string]interface{}{
			"call_id":           callID,
			"party_id":          myPartyID,
			"selected_party_id": remoteParty,
			"version":           "1",
		})

		fmt.Println("✔️ Auto-answered call", callID)
	})

	// 3) Handle answers to our invite
	syncer.OnEventType(event.CallAnswer, func(ctx context.Context, ev *event.Event) {
		if ev.Sender == myUserID || pc.SignalingState() != webrtc.SignalingStateHaveLocalOffer {
			return
		}
		raw := ev.Content.Raw
		callID, _ := raw["call_id"].(string)
		if callID != currentCallID {
			return
		}
		ansMap, _ := raw["answer"].(map[string]interface{})
		sdp, _ := ansMap["sdp"].(string)

		// Apply remote answer
		if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp}); err != nil {
			fmt.Println("SetRemoteDescription error:", err)
			return
		}

		// Finalize handshake
		client.SendMessageEvent(ctx, id.RoomID(cfg.RoomID), event.CallSelectAnswer, map[string]interface{}{
			"call_id":           callID,
			"party_id":          myPartyID,
			"selected_party_id": raw["party_id"].(string),
			"version":           "1",
		})

		fmt.Println("✔️ Call established with", raw["party_id"].(string))
	})

	// 4) Handle remote ICE candidates
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

	// Start sync loop
	go func() {
		if err := client.Sync(); err != nil {
			panic(err)
		}
	}()

	// 5) Initiate outgoing call
	currentCallID = fmt.Sprintf("call-%d", time.Now().Unix())
	offer, invite := buildOffer(currentCallID, pc)
	invite["version"] = "1"
	invite["party_id"] = myPartyID
	client.SendMessageEvent(context.Background(), id.RoomID(cfg.RoomID), event.CallInvite, invite)
	fmt.Println("🔔 Sent m.call.invite — waiting for answer… Offer SDP length:", len(offer.SDP))

	// Block forever
	select {}
}

// buildOffer creates and sends the SDP offer
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

// mustCreatePeerConnection configures WebRTC with a public STUN server and logs states
func mustCreatePeerConnection() *webrtc.PeerConnection {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{
			URLs: []string{"stun:stun.l.google.com:19302"},
		}},
	}
	pc, err := webrtc.NewPeerConnection(config)
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

// mustLogin performs Matrix login and returns an authenticated client
func mustLogin(cfg Config) *mautrix.Client {
	client, err := mautrix.NewClient(cfg.Homeserver, "", "")
	if err != nil {
		panic(err)
	}
	req := &mautrix.ReqLogin{Type: "m.login.password", Identifier: mautrix.UserIdentifier{Type: "m.id.user", User: cfg.Username}, Password: cfg.Password}
	resp, err := client.Login(context.Background(), req)
	if err != nil {
		panic(err)
	}
	client.SetCredentials(resp.UserID, resp.AccessToken)
	fmt.Println("✔️ Logged in as", resp.UserID)
	return client
}

// loadConfig reads JSON configuration from a file
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

// toPtrString returns a pointer to the given string
func toPtrString(s string) *string { return &s }

// toPtrUint16 converts a float64 to uint16 pointer
func toPtrUint16(f float64) *uint16 { u := uint16(f); return &u }

func printRoomMembers(client *mautrix.Client, roomID id.RoomID) error {
	resp, err := client.JoinedMembers(
		context.Background(),
		roomID,
	)
	if err != nil {
		return fmt.Errorf("failed to fetch joined members: %w", err)
	}

	fmt.Printf("Members in room %s:\n", roomID)
	for userID, member := range resp.Joined {
		display := string(userID)
		if member.DisplayName != "" {
			display = fmt.Sprintf("%s (%s)", userID, member.DisplayName)
		}
		fmt.Printf(" • %s\n", display)
	}
	return nil
}
