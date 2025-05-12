package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type Config struct {
	Homeserver string `json:"homeserver"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	RoomID     string `json:"room_id"`
}

var (
	pendingCandidates []webrtc.ICECandidateInit
	mu                sync.Mutex
)

func ptrString(s string) *string { return &s }
func ptrUint16(i uint16) *uint16 { return &i }

func startVoiceCall() {
	cfg := loadConfig("config.json")
	client := mustLogin(cfg)
	roomID := id.RoomID(cfg.RoomID)

	partyID := string(client.DeviceID)
	callID := fmt.Sprintf("call-%d", time.Now().Unix())

	pc := mustCreatePeerConnection()

	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		ice := c.ToJSON()
		content := map[string]interface{}{
			"call_id":  callID,
			"party_id": partyID,
			"version":  "1",
			"candidates": []interface{}{map[string]interface{}{
				"candidate":     ice.Candidate,
				"sdpMid":        ice.SDPMid,
				"sdpMLineIndex": ice.SDPMLineIndex,
			}},
		}
		if _, err := client.SendMessageEvent(
			context.Background(),
			roomID,
			event.CallCandidates,
			content,
		); err != nil {
			fmt.Println("Error sending m.call.candidates:", err)
		}
	})

	syncer.OnEventType(event.CallAnswer, func(_ context.Context, ev *event.Event) {
		raw := ev.Content.Raw
		if raw["call_id"] != callID {
			return
		}
		ans, _ := raw["answer"].(map[string]interface{})
		sdp, _ := ans["sdp"].(string)

		if err := pc.SetRemoteDescription(webrtc.SessionDescription{
			Type: webrtc.SDPTypeAnswer,
			SDP:  sdp,
		}); err != nil {
			fmt.Println("SetRemoteDescription error:", err)
			return
		}

		mu.Lock()
		for _, init := range pendingCandidates {
			if err := pc.AddICECandidate(init); err != nil {
				fmt.Println("Buffered AddICECandidate error:", err)
			}
		}
		pendingCandidates = nil
		mu.Unlock()

		selectContent := map[string]interface{}{
			"call_id":  callID,
			"party_id": partyID,
			"version":  "1",
		}
		if _, err := client.SendMessageEvent(
			context.Background(),
			roomID,
			event.CallSelectAnswer,
			selectContent,
		); err != nil {
			fmt.Println("Error sending m.call.select_answer:", err)
		}
	})

	syncer.OnEventType(event.CallCandidates, func(_ context.Context, ev *event.Event) {
		raw := ev.Content.Raw
		if raw["call_id"] != callID {
			return
		}
		list, _ := raw["candidates"].([]interface{})
		for _, ci := range list {
			m, _ := ci.(map[string]interface{})
			init := webrtc.ICECandidateInit{
				Candidate:     m["candidate"].(string),
				SDPMid:        ptrString(m["sdpMid"].(string)),
				SDPMLineIndex: ptrUint16(uint16(m["sdpMLineIndex"].(float64))),
			}

			mu.Lock()
			if pc.RemoteDescription() != nil {
				_ = pc.AddICECandidate(init)
			} else {
				pendingCandidates = append(pendingCandidates, init)
			}
			mu.Unlock()
		}
	})

	go func() {
		if err := client.Sync(); err != nil {
			panic(err)
		}
	}()

	_, inviteContent := buildOffer(callID, partyID, pc)
	if _, err := client.SendMessageEvent(
		context.Background(),
		roomID,
		event.CallInvite,
		inviteContent,
	); err != nil {
		panic(fmt.Errorf("sending m.call.invite: %w", err))
	}
	fmt.Println("🔔 Sent m.call.invite — waiting for answer…")

	select {}
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

func mustLogin(cfg Config) *mautrix.Client {
	client, err := mautrix.NewClient(cfg.Homeserver, "", "")
	if err != nil {
		panic(err)
	}
	resp, err := client.Login(context.Background(), &mautrix.ReqLogin{
		Type: "m.login.password",
		Identifier: mautrix.UserIdentifier{
			Type: "m.id.user", User: cfg.Username,
		},
		Password:         cfg.Password,
		StoreCredentials: true,
	})
	if err != nil {
		panic(err)
	}
	client.SetCredentials(resp.UserID, resp.AccessToken)
	fmt.Println("✔ Logged in as", resp.UserID)
	return client
}

func mustCreatePeerConnection() *webrtc.PeerConnection {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		panic(err)
	}
	if _, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		panic(err)
	}
	return pc
}

func buildOffer(
	callID, partyID string,
	pc *webrtc.PeerConnection,
) (webrtc.SessionDescription, map[string]interface{}) {
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		panic(err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		panic(err)
	}
	<-webrtc.GatheringCompletePromise(pc)

	content := map[string]interface{}{
		"call_id":  callID,
		"party_id": partyID,
		"lifetime": 60000,
		"version":  "1",
		"offer": map[string]interface{}{
			"type": offer.Type.String(),
			"sdp":  offer.SDP,
		},
	}
	return offer, content
}

func printRoomMembers(client *mautrix.Client, roomID id.RoomID) error {
	resp, err := client.JoinedMembers(context.Background(), roomID)
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
