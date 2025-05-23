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

func audioCallingInWindow(myApp fyne.App, avatarPath string) {
	win := myApp.NewWindow("Calling")
	win.Resize(fyne.NewSize(250, 250))
	win.CenterOnScreen()

	avatarRes, err := fyne.LoadResourceFromPath(avatarPath)
	var avatarImg *canvas.Image
	if err != nil {
		fmt.Println("Ошибка загрузки аватара:", err)
		avatarImg = canvas.NewImageFromResource(theme.AccountIcon())
	} else {
		avatarImg = canvas.NewImageFromResource(avatarRes)
	}
	avatarImg.FillMode = canvas.ImageFillContain
	avatarImg.SetMinSize(fyne.NewSize(150, 150))

	nameLabel := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	timerLabel := widget.NewLabel("00:00")

	start := time.Now()
	go func() {
		ticker := time.NewTicker(time.Second)
		for range ticker.C {
			elapsed := time.Since(start)
			min := int(elapsed.Minutes())
			sec := int(elapsed.Seconds()) % 60
			timeText := fmt.Sprintf("%02d:%02d", min, sec)
			fyne.CurrentApp().Driver().AllWindows()[0].Content().Refresh()
			timerLabel.SetText(timeText)
		}
	}()

	iconEndCall, err := fyne.LoadResourceFromPath("assets/endCall.png")
	if err != nil {
		iconEndCall = theme.CancelIcon()
	}
	endCallBtn := widget.NewButtonWithIcon("End Call", iconEndCall, func() {
		win.Close()
	})

	content := container.NewVBox(
		container.NewCenter(avatarImg),
		nameLabel,
		container.NewCenter(timerLabel),
		container.NewCenter(endCallBtn),
	)

	win.SetContent(content)
	win.Show()
	audioCallMatrix()
}
