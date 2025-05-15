package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"maunium.net/go/mautrix/id"
)

func startMessenger(myApp fyne.App) {

	cfg := loadConfig("config.json")
	client := mustLogin(cfg)
	roomID := id.RoomID(cfg.RoomID)

	telegramWindow := myApp.NewWindow("QalqanDS")
	telegramWindow.Resize(fyne.NewSize(600, 400))

	sampleChats := []struct {
		Name    string
		Message string
		Time    string
	}{
		{"Мама", "где ты?...", "13:02"},
	}

	var chatItems []fyne.CanvasObject
	for _, chat := range sampleChats {
		item := container.NewBorder(
			nil, nil,
			widget.NewIcon(theme.AccountIcon()),
			widget.NewLabel(chat.Time),
			container.NewVBox(
				widget.NewLabelWithStyle(chat.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				widget.NewLabel(chat.Message),
			),
		)
		chatItems = append(chatItems, item)
	}
	chatList := container.NewVBox(chatItems...)
	chatSidebar := container.NewVBox(widget.NewLabelWithStyle("Chats", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}), chatList)
	chatSidebarContainer := container.NewVScroll(chatSidebar)
	chatSidebarContainer.SetMinSize(fyne.NewSize(250, 700))

	chatTitle := container.NewVBox(
		widget.NewLabelWithStyle("Star 3.0", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("242 members, 112 online"),
	)

	audioCallBtn := widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
		printRoomMembers(client, roomID)
		startCalling()
	})
	videoCallBtn := widget.NewButtonWithIcon("", theme.MediaVideoIcon(), func() {
	})

	callButtons := container.NewHBox(audioCallBtn, videoCallBtn)
	chatHeader := container.NewBorder(nil, nil, nil, callButtons, chatTitle)

	msgs := container.NewVBox()

	msgScroll := container.NewVScroll(msgs)

	input := widget.NewEntry()
	input.SetPlaceHolder("Write a message...")

	sendBtn := widget.NewButton("Send", func() {
		if input.Text != "" {
			msgs.Add(messageBubble("You", input.Text, false))
			input.SetText("")
			msgScroll.ScrollToBottom()
		}
	})

	inputBar := container.NewBorder(nil, nil, widget.NewIcon(theme.ConfirmIcon()), sendBtn, input)

	bg := canvas.NewImageFromFile("assets/background.png")
	bg.FillMode = canvas.ImageFillStretch

	chatRight := container.NewBorder(chatHeader, inputBar, nil, nil, container.NewMax(bg, msgScroll))

	mainSplit := container.NewHSplit(chatSidebarContainer, chatRight)
	mainSplit.Offset = 0.25

	telegramWindow.SetContent(mainSplit)
	telegramWindow.Show()
}

func messageBubble(sender string, text string, isDark bool) *fyne.Container {
	name := widget.NewLabelWithStyle(sender, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	body := widget.NewLabel(text)

	var bubble fyne.CanvasObject
	if isDark {
		rect := canvas.NewRectangle(theme.DisabledColor())
		bubble = container.NewMax(rect, container.NewVBox(name, body))
	} else {
		rect := canvas.NewRectangle(theme.InputBackgroundColor())
		bubble = container.NewMax(rect, container.NewVBox(name, body))
	}
	return container.NewVBox(bubble)
}
