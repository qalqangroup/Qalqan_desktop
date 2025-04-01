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
	InitUI(myWindow)
	myWindow.ShowAndRun()
}
