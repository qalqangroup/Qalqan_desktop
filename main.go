package main

import (
	"QalqanDS/qalqan"
	"bytes"
	"crypto/rand"
	"fmt"
	"image/color"
	"io"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

type CustomButton struct {
	widget.BaseWidget
	Text    string
	Color   color.RGBA
	OnClick func()
	bgRect  *canvas.Rectangle
	label   *canvas.Text
}

func NewCustomButton(text string, btnColor color.RGBA, onClick func()) *CustomButton {
	button := &CustomButton{Text: text, Color: btnColor, OnClick: onClick}
	button.ExtendBaseWidget(button)
	return button
}

func (b *CustomButton) CreateRenderer() fyne.WidgetRenderer {
	b.bgRect = canvas.NewRectangle(b.Color)
	b.bgRect.SetMinSize(fyne.NewSize(90, 40))
	b.bgRect.CornerRadius = 6

	b.label = canvas.NewText(b.Text, color.White)
	b.label.Alignment = fyne.TextAlignCenter

	container := container.NewStack(b.bgRect, container.NewCenter(b.label))

	return widget.NewSimpleRenderer(container)
}

func (b *CustomButton) Tapped(*fyne.PointEvent) {
	if b.OnClick != nil {
		go b.animateClick()
		b.OnClick()
	}
}

func (b *CustomButton) animateClick() {
	originalColor := b.Color
	darkerColor := color.RGBA{
		R: originalColor.R / 2,
		G: originalColor.G / 2,
		B: originalColor.B / 2,
		A: originalColor.A,
	}

	b.bgRect.FillColor = darkerColor
	canvas.Refresh(b.bgRect)
	time.Sleep(150 * time.Millisecond)

	b.bgRect.FillColor = originalColor
	canvas.Refresh(b.bgRect)
}

func (b *CustomButton) MouseIn(*desktop.MouseEvent) {
	b.bgRect.FillColor = color.RGBA{
		R: min(255, b.Color.R+20),
		G: min(255, b.Color.G+20),
		B: min(255, b.Color.B+20),
		A: b.Color.A,
	}
	canvas.Refresh(b.bgRect)
}

func min(a, b uint8) uint8 {
	if a < b {
		return a
	}
	return b
}

func (b *CustomButton) MouseOut() {
	b.bgRect.FillColor = b.Color
	canvas.Refresh(b.bgRect)
}

func (b *CustomButton) TappedSecondary(*fyne.PointEvent) {}

func colorButton(text string, btnColor color.RGBA, onClick func()) *fyne.Container {
	button := NewCustomButton(text, btnColor, onClick)
	return container.NewCenter(button)
}

func useAndDeleteSessionKey() []uint8 {
	if len(session_keys) == 0 || len(session_keys[0]) == 0 {
		fmt.Println("No session keys available")
		return nil
	}
	key := session_keys[0][0][:qalqan.DEFAULT_KEY_LEN]
	rkey := make([]uint8, qalqan.EXPKLEN)
	qalqan.Kexp(key, qalqan.DEFAULT_KEY_LEN, qalqan.BLOCKLEN, rkey)
	for i := 0; i < qalqan.DEFAULT_KEY_LEN; i++ {
		session_keys[0][0][i] = 0
	}
	copy(session_keys[0][:], session_keys[0][1:])
	session_keys[0][len(session_keys[0])-1] = [qalqan.DEFAULT_KEY_LEN]byte{}
	if session_keys[0][0] == [32]byte{} {
		session_keys = session_keys[1:]
	}
	return rkey
}

func useAndDeleteCircleKey() []uint8 {
	if len(circle_keys) == 0 || len(circle_keys[0]) == 0 {
		fmt.Println("No session keys available")
		return nil
	}
	key := circle_keys[0][0][:qalqan.DEFAULT_KEY_LEN]
	rkey := make([]uint8, qalqan.EXPKLEN)
	qalqan.Kexp(key, qalqan.DEFAULT_KEY_LEN, qalqan.BLOCKLEN, rkey)
	for i := 0; i < qalqan.DEFAULT_KEY_LEN; i++ {
		circle_keys[0][0][i] = 0
	}
	copy(circle_keys[0][:], circle_keys[0][1:])
	circle_keys[0][len(circle_keys[0])-1] = [qalqan.DEFAULT_KEY_LEN]byte{}
	if circle_keys[0][0] == [32]byte{} {
		circle_keys = circle_keys[1:]
	}
	return rkey
}

var session_keys [][100][qalqan.DEFAULT_KEY_LEN]byte
var circle_keys [][10][qalqan.DEFAULT_KEY_LEN]byte

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("QalqanDS")
	myWindow.Resize(fyne.NewSize(675, 790))
	myWindow.CenterOnScreen()
	myWindow.SetFixedSize(false)

	btnColor := color.RGBA{115, 102, 175, 255}

	/*key := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // etalon key
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f}

	data := []uint8{0x10, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, // etalon data
		0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	res := make([]uint8, 16)
	data2 := make([]uint8, 16)

	qalqan.Kexp(key, 32, 16, rkey111)
	qalqan.Encrypt(data, rkey111, 32, 16, res)  // [255 247 218 248 163 247 226 11 36 110 25 52 4 11 163 120] - etalon encrypted data
	qalqan.Decrypt(res, rkey111, 32, 16, data2) // [16 17 34 51 68 85 102 119 136 153 170 187 204 221 238 255] - etalon decrypted data*/

	rKey := make([]uint8, qalqan.EXPKLEN)
	loadKeyLabel := canvas.NewText("Load Key", color.RGBA{75, 62, 145, 255})
	loadKeyLabel.Alignment = fyne.TextAlignCenter
	loadKeyLabel.TextStyle.Bold = true
	loadKeyLabel.TextSize = 18

	passwordEntry := widget.NewEntry()
	passwordEntry.SetPlaceHolder("Enter a password")
	passwordEntry.Password = true

	logOutput := widget.NewMultiLineEntry()
	logOutput.SetMinRowsVisible(6)
	logOutput.Disable()

	passwordButton := colorButton("OK", btnColor, func() {
		password := passwordEntry.Text
		if password == "" {
			dialog.ShowInformation("Error", "Enter a password!", myWindow)
			return
		}
		logOutput.SetText("Password entered: " + password)
		fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				logOutput.SetText("Error opening file: " + err.Error())
				return
			}
			if reader == nil {
				logOutput.SetText("No file selected.")
				return
			}
			defer reader.Close()

			data, err := io.ReadAll(reader)
			if err != nil {
				logOutput.SetText("Failed to read file: " + err.Error())
				return
			}
			ostream := bytes.NewBuffer(data)
			kikey := make([]byte, qalqan.DEFAULT_KEY_LEN)
			ostream.Read(kikey[:qalqan.DEFAULT_KEY_LEN])
			key := qalqan.Hash512(password)
			qalqan.Kexp(key[:], qalqan.DEFAULT_KEY_LEN, qalqan.BLOCKLEN, rKey)
			for i := 0; i < qalqan.DEFAULT_KEY_LEN; i += qalqan.BLOCKLEN {
				qalqan.DecryptOFB(kikey[i:i+qalqan.BLOCKLEN], rKey, qalqan.DEFAULT_KEY_LEN, qalqan.BLOCKLEN, kikey[i:i+qalqan.BLOCKLEN])
			}
			if len(data) < qalqan.BLOCKLEN {
				logOutput.SetText("The file is too short")
				return
			}
			imitstream := bytes.NewBuffer(data)
			imitFile := make([]byte, qalqan.BLOCKLEN)
			rimitkey := make([]byte, qalqan.EXPKLEN)
			qalqan.Kexp(kikey, qalqan.DEFAULT_KEY_LEN, qalqan.BLOCKLEN, rimitkey)
			qalqan.Qalqan_Imit(uint64(len(data)-qalqan.BLOCKLEN), rimitkey, imitstream, imitFile)
			rimit := make([]byte, qalqan.BLOCKLEN)
			imitstream.Read(rimit[:qalqan.BLOCKLEN])
			if !bytes.Equal(rimit, imitFile) {
				logOutput.SetText("The file is corrupted")
			}
			qalqan.LoadCircleKeys(data, ostream, rKey, &circle_keys)
			qalqan.LoadSessionKeys(data, ostream, rKey, &session_keys)
			fmt.Println("Session keys loaded successfully")
			dialog.ShowInformation("Success", "Keys loaded successfully!", myWindow)

			defer func() {
				if r := recover(); r != nil {
					logOutput.SetText(fmt.Sprintf("File open failed: %v", r))
				}
			}()
		}, myWindow)

		fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".bin"}))
		fileDialog.Show()
	})

	passwordButtonContainer := container.NewHBox(layout.NewSpacer(), passwordButton)

	passwordContainer := container.NewBorder(nil, nil, nil, passwordButtonContainer, passwordEntry)

	userSelect := widget.NewSelect([]string{"Abonent 1", "Abonent 2"}, func(value string) {})

	sessionKeysCheckbox := widget.NewCheck("Session Keys", nil)
	sessionKeysSelect := widget.NewSelect([]string{"32", "64"}, func(value string) {
		fmt.Println("Selected key size:", value)
	})
	keysLeftLabel := widget.NewLabel("keys left")
	sessionKeysContainer := container.NewHBox(sessionKeysCheckbox, sessionKeysSelect, keysLeftLabel)

	selectUserLabel := canvas.NewText("Select a User", color.RGBA{75, 62, 145, 255})
	selectUserLabel.Alignment = fyne.TextAlignCenter
	selectUserLabel.TextStyle.Bold = true
	selectUserLabel.TextSize = 18

	hashLabel := canvas.NewText("Key’s Hash", color.RGBA{75, 62, 145, 255})
	hashLabel.Alignment = fyne.TextAlignCenter
	hashLabel.TextStyle.Bold = true
	hashLabel.TextSize = 18

	hashedKey := qalqan.Hash512("some_value")
	hashHex := fmt.Sprintf("%x", hashedKey)

	hashValue := canvas.NewText(hashHex, color.Black)
	hashValue.Alignment = fyne.TextAlignCenter
	hashValue.TextSize = 18

	hashContainer := container.NewVBox(
		container.NewCenter(hashLabel),
		container.NewCenter(hashValue),
	)

	customMessageCheck := widget.NewCheck("Custom message", nil)
	modeCheck := widget.NewCheck("Mode (for experts)", nil)
	modeSelect := widget.NewSelect([]string{"OFB", "CFB", "CBC"}, func(value string) {})
	optionsBar := container.NewHBox(customMessageCheck, layout.NewSpacer(), modeCheck, modeSelect)

	logOutput.SetMinRowsVisible(6)
	logOutput.Disable()
	clearLogButton := colorButton("Clear log", btnColor, func() { logOutput.SetText("") })

	fromEntry := widget.NewEntry()
	toEntry := widget.NewEntry()
	dateEntry := widget.NewEntry()
	regEntry := widget.NewEntry()

	tableBar := container.NewGridWithColumns(4,
		container.NewVBox(
			container.NewCenter(func() *canvas.Text {
				text := canvas.NewText("From", color.RGBA{75, 62, 145, 255})
				text.TextStyle.Bold = true
				text.TextSize = 18
				return text
			}()),
			fromEntry,
		),
		container.NewVBox(
			container.NewCenter(func() *canvas.Text {
				text := canvas.NewText("To", color.RGBA{75, 62, 145, 255})
				text.TextStyle.Bold = true
				text.TextSize = 18
				return text
			}()),
			toEntry,
		),
		container.NewVBox(
			container.NewCenter(func() *canvas.Text {
				text := canvas.NewText("Date", color.RGBA{75, 62, 145, 255})
				text.TextStyle.Bold = true
				text.TextSize = 18
				return text
			}()),
			dateEntry,
		),
		container.NewVBox(
			container.NewCenter(func() *canvas.Text {
				text := canvas.NewText("Reg", color.RGBA{75, 62, 145, 255})
				text.TextStyle.Bold = true
				text.TextSize = 18
				return text
			}()),
			regEntry,
		),
	)

	outputLabel := widget.NewMultiLineEntry()
	outputLabel.SetMinRowsVisible(6)
	outputLabel.Disable()

	updateOutput := func() {
		outputLabel.SetText("From: " + fromEntry.Text + "\nTo: " + toEntry.Text + "\nDate: " + dateEntry.Text + "\nReg: " + regEntry.Text)
	}

	fromEntry.OnChanged = func(string) { updateOutput() }
	toEntry.OnChanged = func(string) { updateOutput() }
	dateEntry.OnChanged = func(string) { updateOutput() }
	regEntry.OnChanged = func(string) { updateOutput() }

	encryptButton := colorButton("Encrypt file", btnColor, func() {
		fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				logOutput.SetText("Error opening file: " + err.Error())
				return
			}
			if reader == nil {
				logOutput.SetText("No file selected.")
				return
			}
			defer reader.Close()

			data, err := io.ReadAll(reader)
			if err != nil {
				logOutput.SetText("Failed to read file: " + err.Error())
				return
			}

			ostream := bytes.NewBuffer(data)
			sstream := &bytes.Buffer{}

			defer func() {
				if r := recover(); r != nil {
					logOutput.SetText(fmt.Sprintf("Encryption failed: %v", r))
				}
			}()

			iv := make([]byte, qalqan.BLOCKLEN)
			_, err = rand.Read(iv)
			if err != nil {
				logOutput.SetText("Failed to generate IV: " + err.Error())
				return
			}

			rKey := useAndDeleteSessionKey()
			if rKey == nil {
				logOutput.SetText("No session key available for encryption.")
				return
			}

			qalqan.EncryptOFB_File(len(data), rKey, iv, ostream, sstream)

			saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
				if err != nil {
					logOutput.SetText("Error saving file: " + err.Error())
					return
				}
				if writer == nil {
					logOutput.SetText("No file selected for saving.")
					return
				}
				defer writer.Close()

				_, err = writer.Write(sstream.Bytes())
				if err != nil {
					logOutput.SetText("Failed to save encrypted file: " + err.Error())
					return
				}

				logOutput.SetText("File successfully encrypted and saved!")
			}, myWindow)

			saveDialog.SetFileName("encrypted_file.bin")
			saveDialog.Show()
		}, myWindow)
		fileDialog.Show()
	})

	decryptButton := colorButton("Decrypt file", btnColor, func() {
		fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				logOutput.SetText("Error opening file: " + err.Error())
				return
			}
			if reader == nil {
				logOutput.SetText("No file selected.")
				return
			}
			defer reader.Close()

			data, err := io.ReadAll(reader)
			if err != nil {
				logOutput.SetText("Failed to read file: " + err.Error())
				return
			}

			if len(data) < qalqan.BLOCKLEN {
				logOutput.SetText("Invalid file: too small to contain IV.")
				return
			}

			iv := data[:qalqan.BLOCKLEN]
			encryptedData := data[qalqan.BLOCKLEN:]

			rKey := useAndDeleteSessionKey()
			if rKey == nil {
				logOutput.SetText("No session key available for decryption.")
				return
			}

			ostream := bytes.NewBuffer(encryptedData)
			sstream := &bytes.Buffer{}

			defer func() {
				if r := recover(); r != nil {
					logOutput.SetText(fmt.Sprintf("Decryption failed: %v", r))
				}
			}()

			qalqan.DecryptOFB_File(uint64(len(encryptedData)), rKey, iv, ostream, sstream)

			saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
				if err != nil {
					logOutput.SetText("Error saving file: " + err.Error())
					return
				}
				if writer == nil {
					logOutput.SetText("No file selected for saving.")
					return
				}
				defer writer.Close()

				_, err = writer.Write(sstream.Bytes())
				if err != nil {
					logOutput.SetText("Failed to save decrypted file: " + err.Error())
					return
				}

				logOutput.SetText("File successfully decrypted and saved!")
			}, myWindow)

			saveDialog.SetFileName("decrypted_file.bin")
			saveDialog.Show()
		}, myWindow)

		fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".bin"}))
		fileDialog.Show()
	})

	buttonBar := container.NewHBox(layout.NewSpacer(), encryptButton, decryptButton, layout.NewSpacer())

	createMessageButton := colorButton("Create message", btnColor, func() { println("Create message") })

	content := container.NewVBox(
		container.NewCenter(loadKeyLabel),
		passwordContainer,
		container.NewCenter(selectUserLabel),
		userSelect,
		sessionKeysContainer,
		hashContainer,
		optionsBar,
		buttonBar,
		logOutput,
		clearLogButton,
		tableBar,
		outputLabel,
		createMessageButton,
	)

	icon, err := fyne.LoadResourceFromPath("icon.ico")
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		myWindow.SetIcon(icon)
	}

	myWindow.SetContent(content)
	myWindow.ShowAndRun()
}
