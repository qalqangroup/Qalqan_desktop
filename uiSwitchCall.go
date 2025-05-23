package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func switchCall(myApp fyne.App) {
	bgImage := canvas.NewImageFromFile("assets/background.png")
	bgImage.FillMode = canvas.ImageFillStretch

	icon, err := fyne.LoadResourceFromPath("assets/icon.ico")
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		myApp.SetIcon(icon)
	}

	win := myApp.NewWindow(" ")
	win.Resize(fyne.NewSize(350, 350))
	win.CenterOnScreen()
	win.SetFixedSize(true)

	avatar := canvas.NewImageFromFile("assets/avatar.png")
	avatar.FillMode = canvas.ImageFillContain
	avatar.SetMinSize(fyne.NewSize(150, 150))

	iconEndCall, err := fyne.LoadResourceFromPath("assets/endCall.png")
	if err != nil {
		iconEndCall = theme.CancelIcon()
	}

	iconAudioCall, err := fyne.LoadResourceFromPath("assets/call.png")
	if err != nil {
		fmt.Println("Error loading icon:", err)
		iconAudioCall = theme.CancelIcon()
	}

	iconVideoCall, err := fyne.LoadResourceFromPath("assets/videoCall.png")
	if err != nil {
		fmt.Println("Error loading icon:", err)
		iconVideoCall = theme.CancelIcon()
	}

	videoBtn := widget.NewButtonWithIcon("", iconVideoCall, func() {
		// videoCallMatrix()
	})
	cancelBtn := widget.NewButtonWithIcon("", iconEndCall, func() {
		// cancelCall()
		win.Close()
		myApp.Quit()
	})
	callBtn := widget.NewButtonWithIcon("", iconAudioCall, func() {
		win.Hide()
		audioCallingInWindow(myApp, "assets/avatar.png")
	})

	videoBox := container.NewVBox(
		container.NewCenter(videoBtn),
		container.NewCenter(widget.NewLabel("Start Video")),
	)

	cancelBox := container.NewVBox(
		container.NewCenter(cancelBtn),
		container.NewCenter(widget.NewLabel("Cancel")),
	)
	callBox := container.NewVBox(
		container.NewCenter(callBtn),
		container.NewCenter(widget.NewLabel("Start Call")),
	)

	buttons := container.NewHBox(
		layout.NewSpacer(), videoBox,
		layout.NewSpacer(), cancelBox,
		layout.NewSpacer(), callBox,
		layout.NewSpacer(),
	)

	switchWindow := container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(avatar),
		layout.NewSpacer(),
		buttons,
		layout.NewSpacer(),
	)

	content := container.NewStack(bgImage, switchWindow)

	win.SetContent(content)
	win.Show()
}
