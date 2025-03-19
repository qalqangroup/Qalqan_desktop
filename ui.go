package main

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func InitUI(w fyne.Window) {

	bgImage := canvas.NewImageFromFile("assets/background.png")
	bgImage.FillMode = canvas.ImageFillStretch

	icon, err := fyne.LoadResourceFromPath("assets/icon.ico")
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		w.SetIcon(icon)
	}

	// Select source of key
	selectSource := widget.NewSelect([]string{"File", "Key"}, nil)
	selectSource.PlaceHolder = "Select source of key"

	// Поле пароля
	passwordEntry := widget.NewEntry()
	passwordEntry.SetPlaceHolder("Enter a password")

	// Кнопка OK
	okButton := widget.NewButton("OK", nil)

	// Радиокнопка Custom Message
	customMessage := widget.NewRadioGroup([]string{""}, nil)

	// Жирный текст Custom Message
	customMessageLabel := canvas.NewText("Custom Message", color.RGBA{38, 26, 172, 255})
	customMessageLabel.TextSize = 18
	customMessageLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Объединяем радиокнопку и текст
	customMessageContainer := container.NewHBox(customMessage, customMessageLabel)

	// Верхняя строка
	topRow := container.NewGridWithColumns(4,
		selectSource,
		passwordEntry,
		okButton,
		customMessageContainer,
	)

	// 🔹 Добавляем отступ перед `Hash of Key`
	spacer := widget.NewLabel(" ") // Отступ (≈20px)

	// Hash of Key
	hashLabel := canvas.NewText("Hash of Key", color.RGBA{38, 26, 172, 255})
	hashLabel.TextSize = 18
	hashLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Поле ввода Hash Value
	hashValue := widget.NewEntry()
	hashValue.Disable()
	hashValue.Resize(fyne.NewSize(400, 40))

	// Упаковываем Hash Label и Entry
	hashContainer := container.NewVBox(
		container.NewCenter(hashLabel),
		container.NewCenter(container.NewGridWrap(fyne.NewSize(400, 40), hashValue)),
	)

	// Радио-кнопки "Mode (for experts)"
	modeExperts := widget.NewRadioGroup([]string{"Mode (for experts)"}, nil)

	// Радио-кнопка "Session keys" и "Mode (for experts)"
	sessionKeys := widget.NewRadioGroup([]string{"Session keys"}, nil)

	sessionModeContainer := container.NewGridWithColumns(2, sessionKeys, modeExperts)

	// Нижняя часть: Кнопки Encrypt/Decrypt
	encryptButton := widget.NewButton("Encrypt a file", nil)
	decryptButton := widget.NewButton("Decrypt a file", nil)

	encryptDecryptRow := container.NewCenter(container.NewHBox(encryptButton, decryptButton))

	// Лог-файл (TextBox)
	logs := widget.NewMultiLineEntry()
	logs.SetPlaceHolder("Logs output...")

	// Кнопка очистки логов
	clearLogsButton := widget.NewButton("Clear logs", func() {
		logs.SetText("")
	})

	mainUI := container.NewVBox(
		widget.NewLabel(" "),
		topRow,
		spacer,
		hashContainer,
		sessionModeContainer,
		encryptDecryptRow,
		logs,
		container.NewCenter(clearLogsButton),
	)

	content := container.NewStack(bgImage, mainUI)

	w.SetContent(content)
}
