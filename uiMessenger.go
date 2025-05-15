package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// startMessenger запускает UI для обмена сообщениями через Matrix
func startMessenger(myApp fyne.App) {
	ctx := context.Background()

	// Загрузка конфигурации и авторизация
	cfg := loadConfig("config.json")
	client := mustLogin(cfg)

	// Устанавливаем присутствие online
	req := mautrix.ReqPresence{Presence: "online", StatusMsg: ""}
	if err := client.SetPresence(ctx, req); err != nil {
		log.Println("Failed to set presence:", err)
	}

	// Создаём окно приложения
	win := myApp.NewWindow("QalqanDS")
	win.Resize(fyne.NewSize(600, 400))

	// Получаем список присоединённых комнат
	jr, err := client.JoinedRooms(ctx)
	if err != nil {
		dialog.ShowError(fmt.Errorf("JoinedRooms error: %w", err), win)
		return
	}

	// Структура для хранения ID и имени
	type Room struct {
		ID   id.RoomID
		Name string
	}
	var rooms []Room
	for _, rid := range jr.JoinedRooms {
		// Получаем имя комнаты
		var nameEvt event.RoomNameEventContent
		err := client.StateEvent(ctx, rid, event.StateRoomName, "", &nameEvt)
		name := string(rid)
		if err == nil && nameEvt.Name != "" {
			name = nameEvt.Name
		}
		rooms = append(rooms, Room{ID: rid, Name: name})
	}

	// UI: список комнат
	roomsList := widget.NewList(
		func() int { return len(rooms) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(rooms[i].Name)
		},
	)

	// UI: чат и ввод сообщений
	chatBox := widget.NewMultiLineEntry()
	chatBox.Disable()
	msgEntry := widget.NewEntry()
	msgEntry.SetPlaceHolder("Type message...")
	sendBtn := widget.NewButton("Send", nil)

	// Компоновка панели ввода
	controls := container.NewBorder(
		nil, nil, nil, sendBtn,
		msgEntry,
	)
	// Основной контейнер чата
	chatContainer := container.NewBorder(nil,
		controls,
		nil, nil,
		chatBox,
	)
	// Разделитель между списком и чатом
	split := container.NewHSplit(roomsList, chatContainer)
	split.SetOffset(0.25)

	iconCall, err := fyne.LoadResourceFromPath("assets/call.png")
	if err != nil {
		fmt.Println("Ошибка загрузки иконки:", err)
		iconCall = theme.CancelIcon()
	}

	// Создаём кнопку аудиозвонка в правом верхнем углу
	audioBtn := widget.NewButtonWithIcon("", iconCall, func() {
		infoCalling(myApp, "Tessay", "avatar.png")
		audioCall()
	})

	// Верхняя панель: пространство + кнопка звонка
	topBar := container.NewHBox(
		layout.NewSpacer(),
		audioBtn,
	)

	// Финальный layout: topBar сверху, split под ним
	final := container.NewBorder(topBar, nil, nil, nil, split)
	win.SetContent(final)

	// Состояние выбранной комнаты
	var mu sync.Mutex
	var selectedRoom id.RoomID

	// Настройка Syncer для новых сообщений
	syncer := client.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEventType(event.EventMessage, func(cctx context.Context, ev *event.Event) {
		mu.Lock()
		curr := selectedRoom
		mu.Unlock()
		if ev.RoomID != curr || ev.Type != event.EventMessage {
			return
		}
		chatBox.SetText(chatBox.Text + string(ev.Sender) + ": " + ev.Content.AsMessage().Body + "\n")
	})

	// Фоновый цикл синхронизации
	go func() {
		for {
			if err := client.Sync(); err != nil {
				log.Println("Sync error:", err)
			}
			time.Sleep(5 * time.Second)
		}
	}()

	// Обработка выбора комнаты
	roomsList.OnSelected = func(i widget.ListItemID) {
		mu.Lock()
		selectedRoom = rooms[i].ID
		mu.Unlock()

		// Загрузка последних сообщений
		resp, err := client.Messages(ctx, selectedRoom, "", "", mautrix.DirectionBackward, nil, 50)
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		// Обновление чата
		content := ""
		for idx := len(resp.Chunk) - 1; idx >= 0; idx-- {
			ev := resp.Chunk[idx]
			if ev.Type == event.EventMessage {
				content += fmt.Sprintf("%s: %s\n", ev.Sender, ev.Content.AsMessage().Body)
			}
		}
		chatBox.SetText(content)

		// Настройка отправки сообщений
		sendBtn.OnTapped = func() {
			if text := msgEntry.Text; text != "" {
				if _, err := client.SendText(ctx, selectedRoom, text); err != nil {
					dialog.ShowError(err, win)
				} else {
					msgEntry.SetText("")
				}
			}
		}
	}
	win.Show()
}
