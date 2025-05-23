package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func main() {
	myApp := app.NewWithID("QalqanDS")
	myWindow := myApp.NewWindow("QalqanDS")
	myWindow.Resize(fyne.NewSize(570, 300))
	myWindow.CenterOnScreen()
	myWindow.SetFixedSize(false)
	InitUI(myApp, myWindow)
	myWindow.ShowAndRun()
}

/*
------------------------------------------------------------
					Cryptography tasks:
1. Why are the last 16 bytes used when decrypting?
2. Add a camera and video encryption;
3. Add support russian and kazakh languages.
------------------------------------------------------------
					UX/UI tasks:
1. Add settings button on the top in the uiMessenger.go;
2. Implement video call via WEBRTC.
------------------------------------------------------------
*/
