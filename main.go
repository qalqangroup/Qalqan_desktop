package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Qalqan_DS")
	myWindow.Resize(fyne.NewSize(600, 300))
	myWindow.CenterOnScreen()
	myWindow.SetFixedSize(false)
	InitUI(myWindow)
	myWindow.ShowAndRun()
}
