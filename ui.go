package main

import (
	"QalqanDS/qalqan"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

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

func useAndDeleteCircleKey(randomNum int) []uint8 {
	if len(circle_keys) == 0 || len(circle_keys[0]) == 0 {
		fmt.Println("No session keys available")
		return nil
	}
	key := circle_keys[randomNum][:qalqan.DEFAULT_KEY_LEN]
	//copy(key, circle_keys[randomNum][:])
	rkey := make([]uint8, qalqan.EXPKLEN)
	qalqan.Kexp(key, qalqan.DEFAULT_KEY_LEN, qalqan.BLOCKLEN, rkey)
	return rkey
}

var session_keys [][100][qalqan.DEFAULT_KEY_LEN]byte
var circle_keys [10][qalqan.DEFAULT_KEY_LEN]byte

func InitUI(w fyne.Window) {

	bgImage := canvas.NewImageFromFile("assets/background.png")
	bgImage.FillMode = canvas.ImageFillStretch

	icon, err := fyne.LoadResourceFromPath("assets/icon.ico")
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		w.SetIcon(icon)
	}

	logs := widget.NewMultiLineEntry()
	logs.SetPlaceHolder("Logs output...")
	logs.Disable()
	logs.Wrapping = fyne.TextWrapWord
	logs.Scroll = container.ScrollBoth

	logs.Resize(fyne.NewSize(800, 150))
	logsContainer := container.NewGridWrap(fyne.NewSize(800, 150), logs)

	rKey := make([]uint8, qalqan.EXPKLEN)

	selectSource := widget.NewSelect([]string{"File", "Key"}, nil)
	selectSource.PlaceHolder = "Select source of key"

	passwordEntry := widget.NewEntry()
	passwordEntry.SetPlaceHolder("Enter a password")

	spacer1 := widget.NewLabel(" ")

	hashLabel := widget.NewLabelWithStyle(
		"Hash of Key",
		fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true},
	)

	hashValue := widget.NewEntry()
	hashValue.Disable()
	hashValue.Resize(fyne.NewSize(400, 40))

	hashContainer := container.NewVBox(
		hashLabel,
		hashValue,
	)

	spacer2 := widget.NewLabel(" ")

	okButton := widget.NewButton("OK", func() {
		password := passwordEntry.Text
		if password == "" {
			dialog.ShowInformation("Error", "Enter a password!", w)
			return
		}
		logs.SetText("Password entered: " + password)
		fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				logs.SetText("Error opening file: " + err.Error())
				return
			}
			if reader == nil {
				logs.SetText("No file selected.")
				return
			}
			defer reader.Close()

			data, err := io.ReadAll(reader)
			if err != nil {
				logs.SetText("Failed to read file: " + err.Error())
				return
			}
			ostream := bytes.NewBuffer(data)
			kikey := make([]byte, qalqan.DEFAULT_KEY_LEN)
			ostream.Read(kikey[:qalqan.DEFAULT_KEY_LEN])
			key := qalqan.Hash512(password)
			keyBytes := hex.EncodeToString(key[:])
			hashValue.SetText(keyBytes)
			qalqan.Kexp(key[:], qalqan.DEFAULT_KEY_LEN, qalqan.BLOCKLEN, rKey)
			for i := 0; i < qalqan.DEFAULT_KEY_LEN; i += qalqan.BLOCKLEN {
				qalqan.DecryptOFB(kikey[i:i+qalqan.BLOCKLEN], rKey, qalqan.DEFAULT_KEY_LEN, qalqan.BLOCKLEN, kikey[i:i+qalqan.BLOCKLEN])
			}
			if len(data) < qalqan.BLOCKLEN {
				logs.SetText("The file is too short")
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
				logs.SetText("The file is corrupted")
			}
			circle_keys = [10][qalqan.DEFAULT_KEY_LEN]byte{}
			qalqan.LoadCircleKeys(data, ostream, rKey, &circle_keys)
			qalqan.LoadSessionKeys(data, ostream, rKey, &session_keys)
			fmt.Println("Session keys loaded successfully")
			dialog.ShowInformation("Success", "Keys loaded successfully!", w)

			defer func() {
				if r := recover(); r != nil {
					logs.SetText(fmt.Sprintf("File open failed: %v", r))
				}
			}()
		}, w)

		fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".bin"}))
		fileDialog.Show()
	})

	customMessage := widget.NewRadioGroup([]string{"Custom Message"}, nil)

	topRow := container.NewGridWithColumns(4,
		selectSource,
		passwordEntry,
		okButton,
		customMessage,
	)

	sessionKeys := widget.NewRadioGroup([]string{"Session Keys"}, nil)
	keysLeftEntry := widget.NewEntry()
	keysLeftEntry.SetPlaceHolder("Keys left")

	leftContainer := container.NewVBox(sessionKeys, keysLeftEntry)

	keyTypeLabel := widget.NewLabelWithStyle(
		"Key Type",
		fyne.TextAlignCenter,
		fyne.TextStyle{Bold: false},
	)

	centeredKeyTypeLabel := container.NewCenter(keyTypeLabel)

	keyTypeSelect := widget.NewSelect(
		[]string{"Circular", "Session"},
		func(selected string) {
			fmt.Println("Выбран тип ключа:", selected)
		},
	)

	keyTypeSelect.PlaceHolder = "Select key type"

	centerContainer := container.NewVBox(
		centeredKeyTypeLabel,
		keyTypeSelect,
	)

	modeExperts := widget.NewRadioGroup([]string{"Mode (for experts)"}, nil)
	selectModeEntry := widget.NewEntry()
	selectModeEntry.SetPlaceHolder("Select mode")

	rightContainer := container.NewVBox(modeExperts, selectModeEntry)

	sessionModeContainer := container.NewGridWithColumns(3,
		leftContainer,
		centerContainer,
		rightContainer,
	)

	spacer3 := widget.NewLabel(" ")

	iconEncrypt, err := fyne.LoadResourceFromPath("assets/encrypt.png")
	if err != nil {
		fmt.Println("Ошибка загрузки иконки:", err)
		iconEncrypt = theme.ConfirmIcon()
	}

	encryptButton := widget.NewButtonWithIcon(
		"Encrypt a file",
		iconEncrypt,
		func() {
			//fmt.Println("Нажата кнопка Encrypt")
			fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
				if err != nil {
					logs.SetText("Error opening file: " + err.Error())
					return
				}
				if reader == nil {
					logs.SetText("No file selected.")
					return
				}
				defer reader.Close()

				data, err := io.ReadAll(reader)
				if err != nil {
					logs.SetText("Failed to read file: " + err.Error())
					return
				}

				ostream := bytes.NewBuffer(data)
				sstream := &bytes.Buffer{}

				defer func() {
					if r := recover(); r != nil {
						logs.SetText(fmt.Sprintf("Encryption failed: %v", r))
					}
				}()

				iv := make([]byte, qalqan.BLOCKLEN)
				for i := range qalqan.BLOCKLEN {
					iv[i] = byte(rand.Intn(256))
				}

				/*
					rKey := useAndDeleteSessionKey() // test use of encryption on session keys
					if rKey == nil {
						logOutput.SetText("No session key available for encryption.")
						return
					}
				*/
				fmt.Println("circle_keys:", circle_keys)
				randomNum := rand.Intn(10)
				rKey := useAndDeleteCircleKey(randomNum)
				if rKey == nil {
					logs.SetText("No session key available for encryption.")
					return
				}

				qalqan.EncryptOFB_File(len(data), rKey, iv, ostream, sstream)

				saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
					if err != nil {
						logs.SetText("Error saving file: " + err.Error())
						return
					}
					if writer == nil {
						logs.SetText("No file selected for saving.")
						return
					}
					defer writer.Close()

					_, err = writer.Write(sstream.Bytes())
					if err != nil {
						logs.SetText("Failed to save encrypted file: " + err.Error())
						return
					}

					logs.SetText("File successfully encrypted and saved!")
				}, w)

				saveDialog.SetFileName("encrypted_file.qln")
				saveDialog.Show()
			}, w)
			fileDialog.Show()
		})

	encryptButton.Resize(fyne.NewSize(300, 100))

	iconDecrypt, err := fyne.LoadResourceFromPath("assets/decrypt.png")
	if err != nil {
		fmt.Println("Ошибка загрузки иконки:", err)
		iconDecrypt = theme.CancelIcon()
	}

	decryptButton := widget.NewButtonWithIcon(
		"Decrypt a file",
		iconDecrypt,
		func() {
			fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
				if err != nil {
					logs.SetText("Error opening file: " + err.Error())
					return
				}
				if reader == nil {
					logs.SetText("No file selected.")
					return
				}
				defer reader.Close()

				data, err := io.ReadAll(reader)
				if err != nil {
					logs.SetText("Failed to read file: " + err.Error())
					return
				}

				if len(data) < qalqan.BLOCKLEN {
					logs.SetText("Invalid file: too small to contain IV.")
					return
				}

				iv := data[:qalqan.BLOCKLEN]
				encryptedData := data[qalqan.BLOCKLEN:]
				/*
					rKey := useAndDeleteSessionKey() // test use of decryption on session keys
					if rKey == nil {
						logOutput.SetText("No session key available for decryption.")
						return
					}
				*/

				rKey := useAndDeleteCircleKey(1) // test use of decryption on session keys
				if rKey == nil {
					logs.SetText("No session key available for decryption.")
					return
				}
				ostream := bytes.NewBuffer(encryptedData)
				sstream := &bytes.Buffer{}

				defer func() {
					if r := recover(); r != nil {
						logs.SetText(fmt.Sprintf("Decryption failed: %v", r))
					}
				}()

				qalqan.DecryptOFB_File(int(len(encryptedData)), rKey, iv, ostream, sstream)

				saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
					if err != nil {
						logs.SetText("Error saving file: " + err.Error())
						return
					}
					if writer == nil {
						logs.SetText("No file selected for saving.")
						return
					}
					defer writer.Close()

					_, err = writer.Write(sstream.Bytes())
					if err != nil {
						logs.SetText("Failed to save decrypted file: " + err.Error())
						return
					}

					logs.SetText("File successfully decrypted and saved!")
				}, w)

				saveDialog.SetFileName("decrypted_file.txt")
				saveDialog.Show()
			}, w)

			fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".qln"}))
			fileDialog.Show()
		},
	)
	decryptButton.Resize(fyne.NewSize(300, 100))

	buttonContainer := container.NewHBox(
		layout.NewSpacer(),
		encryptButton,
		layout.NewSpacer(),
		decryptButton,
		layout.NewSpacer(),
	)

	spacer4 := widget.NewLabel(" ")

	iconClear, err := fyne.LoadResourceFromPath("assets/clear.jpg")
	if err != nil {
		fmt.Println("Ошибка загрузки иконки:", err)
		iconClear = theme.CancelIcon()
	}

	clearLogsButton := container.NewGridWrap(fyne.NewSize(100, 35),
		widget.NewButtonWithIcon(
			"Clear",
			iconClear,
			func() {
				logs.SetText("")
				fmt.Println("Логи очищены")
			},
		),
	)

	centeredButton := container.NewCenter(clearLogsButton)

	spacer5 := widget.NewLabel(" ")

	logsContainer = container.NewVBox(
		logsContainer,
		centeredButton,
	)

	fromEntry := widget.NewEntry()
	fromEntry.SetPlaceHolder("From")

	toEntry := widget.NewEntry()
	toEntry.SetPlaceHolder("To")

	dateEntry := widget.NewEntry()
	dateEntry.SetPlaceHolder("Date")

	regEntry := widget.NewEntry()
	regEntry.SetPlaceHolder("Registration No.")

	tableBar := container.NewGridWithColumns(4,
		fromEntry,
		toEntry,
		dateEntry,
		regEntry,
	)

	outputLabel := widget.NewMultiLineEntry()
	outputLabel.SetMinRowsVisible(6)
	outputLabel.Disable()

	updateOutput := func() {
		outputLabel.SetText(
			"From: " + fromEntry.Text + "\n" +
				"To: " + toEntry.Text + "\n" +
				"Date: " + dateEntry.Text + "\n" +
				"Registration No.: " + regEntry.Text,
		)
	}

	fromEntry.OnChanged = func(string) { updateOutput() }
	toEntry.OnChanged = func(string) { updateOutput() }
	dateEntry.OnChanged = func(string) { updateOutput() }
	regEntry.OnChanged = func(string) { updateOutput() }

	updateOutput()

	messageSend := widget.NewMultiLineEntry()
	messageSend.SetPlaceHolder("Your message...")
	messageSend.Enable()
	messageSend.Wrapping = fyne.TextWrapWord
	messageSend.Scroll = container.ScrollBoth

	messageSend.Resize(fyne.NewSize(800, 150))

	iconEncrMessage, err := fyne.LoadResourceFromPath("assets/encryptMessage.png")
	if err != nil {
		fmt.Println("Ошибка загрузки иконки:", err)
		iconEncrMessage = theme.CancelIcon()
	}

	clearMessageButton := widget.NewButtonWithIcon(
		"Encrypt message",
		iconEncrMessage,
		func() {
			messageSend.SetText("")
			fmt.Println("Очищено")
		},
	)

	centeredButtonMessage := container.NewCenter(clearMessageButton)

	messageContainer := container.NewVBox(
		messageSend,
		centeredButtonMessage,
	)

	mainUI := container.NewVBox(
		widget.NewLabel(" "),
		topRow,
		spacer1,
		hashContainer,
		spacer2,
		sessionModeContainer,
		spacer3,
		buttonContainer,
		spacer4,
		logsContainer,
		spacer5,
		tableBar,
		messageContainer,
	)

	content := container.NewStack(bgImage, mainUI)

	w.SetContent(content)
}
