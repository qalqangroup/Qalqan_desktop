package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

func startVoiceCall() {
	file, _ := os.Open("config.json")
	defer file.Close()
	var cfg Config
	json.NewDecoder(file).Decode(&cfg)

	client, err := mautrix.NewClient(cfg.Homeserver, "", "")
	if err != nil {
		panic(err)
	}

	resp, err := client.Login(context.Background(), &mautrix.ReqLogin{
		Type: "m.login.password",
		Identifier: mautrix.UserIdentifier{
			Type: "m.id.user",
			User: cfg.Username,
		},
		Password: cfg.Password,
	})
	if err != nil {
		panic(err)
	}
	client.SetCredentials(resp.UserID, resp.AccessToken)
	fmt.Println("Logged in to Matrix")

	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		panic(err)
	}
	_, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	if err != nil {
		panic(err)
	}
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		panic(err)
	}
	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		panic(err)
	}

	<-webrtc.GatheringCompletePromise(peerConnection)

	callID := fmt.Sprintf("call-%d", time.Now().Unix())
	inviteContent := map[string]interface{}{
		"call_id":  callID,
		"lifetime": 60000,
		"version":  1,
		"offer": map[string]interface{}{
			"type": "offer",
			"sdp":  peerConnection.LocalDescription().SDP,
		},
	}

	_, err = client.SendMessageEvent(
		context.Background(),
		id.RoomID(cfg.RoomID),
		event.NewEventType("m.call.invite"),
		inviteContent,
	)

	if err != nil {
		fmt.Printf("Ошибка при отправке m.call.invite: %v\n", err)
		panic(err)
	}
	fmt.Println("Sent m.call.invite")
}
