package main

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// infoCalling отображает окно вызова с аватаром, именем и таймером вызова
func infoCalling(myApp fyne.App, calleeName, avatarPath string) {
	// Создаём окно
	win := myApp.NewWindow("Calling " + calleeName)
	win.Resize(fyne.NewSize(200, 300))

	// Загружаем аватар
	avatarRes, err := fyne.LoadResourceFromPath(avatarPath)
	var avatarImg *canvas.Image
	if err != nil {
		fmt.Println("Ошибка загрузки аватара:", err)
		// Используем иконку пользователя по умолчанию
		avatarImg = canvas.NewImageFromResource(theme.AccountIcon())
	} else {
		avatarImg = canvas.NewImageFromResource(avatarRes)
	}
	avatarImg.FillMode = canvas.ImageFillContain
	avatarImg.SetMinSize(fyne.NewSize(100, 100))

	// Метка с именем абонента
	nameLabel := widget.NewLabelWithStyle(calleeName, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	// Метка таймера
	timerLabel := widget.NewLabel("00:00")

	// Запуск таймера
	start := time.Now()
	go func() {
		ticker := time.NewTicker(time.Second)
		for range ticker.C {
			elapsed := time.Since(start)
			min := int(elapsed.Minutes())
			sec := int(elapsed.Seconds()) % 60
			timeText := fmt.Sprintf("%02d:%02d", min, sec)
			// Обновляем метку в главном потоке
			fyne.CurrentApp().Driver().AllWindows()[0].Content().Refresh()
			timerLabel.SetText(timeText)
		}
	}()

	// Кнопка завершения вызова
	iconEndCall, err := fyne.LoadResourceFromPath("assets/endCall.png")
	if err != nil {
		iconEndCall = theme.CancelIcon()
	}
	endCallBtn := widget.NewButtonWithIcon("End Call", iconEndCall, func() {
		win.Close()
	})

	// Компоновка элементов по вертикали
	content := container.NewVBox(
		container.NewCenter(avatarImg),
		nameLabel,
		container.NewCenter(timerLabel),
		container.NewCenter(endCallBtn),
	)

	win.SetContent(content)
	win.Show()
}
