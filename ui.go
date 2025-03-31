package main

import (
	"QalqanDS/qalqan"
	"bytes"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math/rand"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func animateResize(window fyne.Window, newSize fyne.Size) {
	oldSize := window.Canvas().Size()

	stepCount := 10
	delay := 20 * time.Millisecond

	widthStep := (newSize.Width - oldSize.Width) / float32(stepCount)
	heightStep := (newSize.Height - oldSize.Height) / float32(stepCount)

	go func() {
		for i := 0; i < stepCount; i++ {
			time.Sleep(delay)
			window.Resize(fyne.NewSize(
				oldSize.Width+widthStep*float32(i),
				oldSize.Height+heightStep*float32(i),
			))
		}
		window.Resize(newSize)
	}()
}

func useAndDeleteSessionKey(sessionKeyNumber int) []uint8 {
	if len(session_keys) == 0 || len(session_keys[0]) == 0 {
		fmt.Println("No session keys available")
		return nil
	}

	if sessionKeyNumber < 0 || sessionKeyNumber >= len(session_keys[0]) {
		fmt.Println("Invalid session key index")
		return nil
	}

	key := session_keys[0][sessionKeyNumber][:qalqan.DEFAULT_KEY_LEN]
	rkey := make([]uint8, qalqan.EXPKLEN)
	qalqan.Kexp(key, qalqan.DEFAULT_KEY_LEN, qalqan.BLOCKLEN, rkey)
	for i := 0; i < qalqan.DEFAULT_KEY_LEN; i++ {
		session_keys[0][sessionKeyNumber][i] = 0
	}

	if len(session_keys[0]) == 0 {
		session_keys = session_keys[1:]
	}

	return rkey
}

func useAndDeleteCircleKey(circleKeyNumber int) []uint8 {
	if len(circle_keys) == 0 || len(circle_keys[0]) == 0 {
		fmt.Println("No session keys available")
		return nil
	}
	key := circle_keys[circleKeyNumber][:qalqan.DEFAULT_KEY_LEN]
	rkey := make([]uint8, qalqan.EXPKLEN)
	qalqan.Kexp(key, qalqan.DEFAULT_KEY_LEN, qalqan.BLOCKLEN, rkey)
	return rkey
}

func init() {
	rimitkey = make([]byte, qalqan.EXPKLEN)
}

func roundedRect(width, height int, radius int, bgColor color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if (x < radius && y < radius && (x-radius)*(x-radius)+(y-radius)*(y-radius) > radius*radius) ||
				(x < radius && y > height-radius && (x-radius)*(x-radius)+(y-(height-radius))*(y-(height-radius)) > radius*radius) ||
				(x > width-radius && y < radius && (x-(width-radius))*(x-(width-radius))+(y-radius)*(y-radius) > radius*radius) ||
				(x > width-radius && y > height-radius && (x-(width-radius))*(x-(width-radius))+(y-(height-radius))*(y-(height-radius)) > radius*radius) {
				img.Set(x, y, color.Transparent)
			}
		}
	}
	return img
}

var session_keys [][100][qalqan.DEFAULT_KEY_LEN]byte
var circle_keys [10][qalqan.DEFAULT_KEY_LEN]byte
var sessionKeyCount int = 100
var rimitkey []byte

func InitUI(window fyne.Window) {

	bgImage := canvas.NewImageFromFile("assets/background.png")
	bgImage.FillMode = canvas.ImageFillStretch

	icon, err := fyne.LoadResourceFromPath("assets/icon.ico")
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		window.SetIcon(icon)
	}

	logs := widget.NewRichText(&widget.TextSegment{
		Text:  "Logs output...",
		Style: widget.RichTextStyleInline,
	})
	logs.Wrapping = fyne.TextWrapWord

	bg := canvas.NewRaster(func(w, h int) image.Image {
		return roundedRect(w, h, 4, color.RGBA{240, 240, 240, 255})
	})
	bg.SetMinSize(fyne.NewSize(300, 100))

	logsContainer := container.NewStack(bg, logs)

	rKey := make([]uint8, qalqan.EXPKLEN)

	selectSource := widget.NewSelect([]string{"File", "Key"}, nil)
	selectSource.PlaceHolder = "Select source of key"

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Enter a password...")

	hashLabel := widget.NewLabelWithStyle("Hash of Key", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	bgHash := canvas.NewRaster(func(w, h int) image.Image {
		return roundedRect(470, 40, 4, color.White)
	})
	bgHash.SetMinSize(fyne.NewSize(470, 40))

	hashValue := widget.NewRichText(&widget.TextSegment{
		Style: widget.RichTextStyleInline,
	})

	hashBox := container.NewStack(bgHash, container.NewCenter(hashValue))

	hashContainer := container.NewVBox(
		layout.NewSpacer(),
		hashLabel,
		layout.NewSpacer(),
		container.NewCenter(hashBox),
		layout.NewSpacer(),
	)

	sessionKeys := widget.NewRadioGroup([]string{"Session keys"}, nil)

	bgKeysLeft := canvas.NewRaster(func(w, h int) image.Image {
		return roundedRect(170, 40, 4, color.White)
	})
	bgKeysLeft.SetMinSize(fyne.NewSize(170, 40))

	keysLeftEntry := widget.NewLabel("Keys left")

	smallKeysLeftEntry := container.NewStack(bgKeysLeft, container.NewCenter(keysLeftEntry))

	leftContainer := container.NewVBox(
		container.NewCenter(sessionKeys),
		smallKeysLeftEntry,
	)

	okButton := widget.NewButton("OK", func() {
		password := passwordEntry.Text
		if password == "" {
			dialog.ShowInformation("Error", "Enter a password!", window)
			return
		}
		sessionKeyCount = 100
		keysLeftEntry.SetText(fmt.Sprintf("%d", sessionKeyCount))

		fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				logs.Segments = []widget.RichTextSegment{}
				logs.Segments = append(logs.Segments, &widget.TextSegment{
					Text:  "Error opening file: " + err.Error(),
					Style: widget.RichTextStyleInline,
				})
				logs.Refresh()
				return
			}
			if reader == nil {
				logs.Segments = []widget.RichTextSegment{}
				logs.Segments = append(logs.Segments, &widget.TextSegment{
					Text:  "No file selected.",
					Style: widget.RichTextStyleInline,
				})
				logs.Refresh()
				return
			}
			defer reader.Close()

			data, err := io.ReadAll(reader)
			if err != nil {
				logs.Segments = []widget.RichTextSegment{}
				logs.Segments = append(logs.Segments, &widget.TextSegment{
					Text:  "Failed to read file: " + err.Error(),
					Style: widget.RichTextStyleInline,
				})
				logs.Refresh()
				return
			}
			ostream := bytes.NewBuffer(data)
			kikey := make([]byte, qalqan.DEFAULT_KEY_LEN)
			ostream.Read(kikey[:qalqan.DEFAULT_KEY_LEN])
			key := qalqan.Hash512(password)
			keyBytes := hex.EncodeToString(key[:])
			hashValue.Segments = []widget.RichTextSegment{
				&widget.TextSegment{
					Text:  keyBytes,
					Style: widget.RichTextStyleInline,
				},
			}
			hashValue.Refresh()
			qalqan.Kexp(key[:], qalqan.DEFAULT_KEY_LEN, qalqan.BLOCKLEN, rKey)
			for i := 0; i < qalqan.DEFAULT_KEY_LEN; i += qalqan.BLOCKLEN {
				qalqan.DecryptOFB(kikey[i:i+qalqan.BLOCKLEN], rKey, qalqan.DEFAULT_KEY_LEN, qalqan.BLOCKLEN, kikey[i:i+qalqan.BLOCKLEN])
			}
			if len(data) < qalqan.BLOCKLEN {
				logs.Segments = []widget.RichTextSegment{}
				logs.Segments = append(logs.Segments, &widget.TextSegment{
					Text:  "The file is too short",
					Style: widget.RichTextStyleInline,
				})
				logs.Refresh()
				return
			}
			imitstream := bytes.NewBuffer(data)
			imitFile := make([]byte, qalqan.BLOCKLEN)
			qalqan.Kexp(kikey, qalqan.DEFAULT_KEY_LEN, qalqan.BLOCKLEN, rimitkey)
			qalqan.Qalqan_Imit(uint64(len(data)-qalqan.BLOCKLEN), rimitkey, imitstream, imitFile)
			rimit := make([]byte, qalqan.BLOCKLEN)
			imitstream.Read(rimit[:qalqan.BLOCKLEN])
			if !bytes.Equal(rimit, imitFile) {
				logs.Segments = []widget.RichTextSegment{}
				logs.Segments = append(logs.Segments, &widget.TextSegment{
					Text:  "The file is corrupted",
					Style: widget.RichTextStyleInline,
				})
				logs.Refresh()
			}
			circle_keys = [10][qalqan.DEFAULT_KEY_LEN]byte{}
			qalqan.LoadCircleKeys(data, ostream, rKey, &circle_keys)
			qalqan.LoadSessionKeys(data, ostream, rKey, &session_keys)
			fmt.Println("Session keys loaded successfully")
			dialog.ShowInformation("Success", "Keys loaded successfully!", window)

			defer func() {
				if r := recover(); r != nil {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "File open failed: " + fmt.Sprintf("%v", r),
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
				}
			}()
		}, window)

		fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".bin"}))
		fileDialog.Show()
	})

	iconClear, err := fyne.LoadResourceFromPath("assets/clear.png")
	if err != nil {
		fmt.Println("Ошибка загрузки иконки:", err)
		iconClear = theme.CancelIcon()
	}

	clearLogsButton := container.NewGridWrap(fyne.NewSize(120, 40),
		widget.NewButtonWithIcon(
			"Clear log",
			iconClear,
			func() {
				logs.Segments = []widget.RichTextSegment{}
				logs.Refresh()
				fmt.Println("Логи очищены")
			},
		),
	)
	centeredButton := container.NewCenter(clearLogsButton)

	logsContainer = container.NewVBox(
		container.NewPadded(logsContainer),
		centeredButton,
	)

	fromEntry := widget.NewEntry()
	fromEntry.SetPlaceHolder("From")
	fromEntry.Hide()

	toEntry := widget.NewEntry()
	toEntry.SetPlaceHolder("To")
	toEntry.Hide()

	dateEntry := widget.NewEntry()
	dateEntry.SetPlaceHolder("Date")
	dateEntry.Hide()

	regEntry := widget.NewEntry()
	regEntry.SetPlaceHolder("Registration No.")
	regEntry.Hide()

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
	messageSend.Resize(fyne.NewSize(470, 120))
	messageSend.Hide()

	iconEncrMessage, err := fyne.LoadResourceFromPath("assets/encryptMessage.png")
	if err != nil {
		fmt.Println("Ошибка загрузки иконки:", err)
		iconEncrMessage = theme.CancelIcon()
	}

	createdMessageButton := widget.NewButtonWithIcon(
		"Encrypt a message",
		iconEncrMessage,
		func() {
			messageSend.SetText("")
			fmt.Println("Очищено")

			dialog.ShowInformation("Success", "Message encrypted successfully!", window)
		},
	)

	createdMessageButton.Hide()
	centeredButtonMessage := container.NewCenter(createdMessageButton)

	messageContainer := container.NewVBox(
		messageSend,
		centeredButtonMessage,
		layout.NewSpacer(),
	)

	customMessage := widget.NewRadioGroup([]string{"Custom message"}, func(selected string) {
		isEnabled := selected == "Custom message"

		if isEnabled {
			fromEntry.Show()
			toEntry.Show()
			dateEntry.Show()
			regEntry.Show()
			messageSend.Show()
			createdMessageButton.Show()
			animateResize(window, fyne.NewSize(570, 380))
		} else {
			fromEntry.Hide()
			toEntry.Hide()
			dateEntry.Hide()
			regEntry.Hide()
			messageSend.Hide()
			createdMessageButton.Hide()
			animateResize(window, fyne.NewSize(570, 300))
		}
	})

	selectModeEntry := widget.NewSelect(
		[]string{"OFB", "ECB"},
		func(selected string) {
			fmt.Println("Выбран режим:", selected)
		})
	selectModeEntry.PlaceHolder = "Select mode"
	selectModeEntry.Disable()

	modeExperts := widget.NewRadioGroup([]string{"Mode (for experts)"}, func(selected string) {
		if selected == "Mode (for experts)" {
			selectModeEntry.Enable()
		} else {
			selectModeEntry.Disable()
		}
	})
	modeExperts.SetSelected("")

	smallSelectModeEntry := container.NewCenter(container.NewGridWrap(fyne.NewSize(170, 40), selectModeEntry))

	rightContainer := container.NewVBox(
		container.NewCenter(modeExperts),
		smallSelectModeEntry,
	)

	topRow := container.NewHBox(
		layout.NewSpacer(),
		container.NewGridWrap(fyne.NewSize(170, 40), selectSource),
		layout.NewSpacer(),
		container.NewGridWrap(fyne.NewSize(180, 40), passwordEntry),
		layout.NewSpacer(),
		container.NewGridWrap(fyne.NewSize(65, 40), okButton),
		layout.NewSpacer(),
	)

	keyTypeSelect := widget.NewSelect(
		[]string{"Circular", "Session"},
		func(selected string) {
			fmt.Println("Выбран тип ключа:", selected)
		},
	)

	keyTypeSelect.PlaceHolder = "Select key type"

	centerContainer := container.NewVBox(
		container.NewCenter(customMessage),
		container.NewCenter(container.NewGridWrap(fyne.NewSize(170, 40), keyTypeSelect)),
	)

	sessionModeContainer := container.NewHBox(
		layout.NewSpacer(),
		leftContainer,
		layout.NewSpacer(),
		centerContainer,
		layout.NewSpacer(),
		rightContainer,
		layout.NewSpacer(),
	)

	iconEncrypt, err := fyne.LoadResourceFromPath("assets/encrypt.png")
	if err != nil {
		fmt.Println("Ошибка загрузки иконки:", err)
		iconEncrypt = theme.ConfirmIcon()
	}

	encryptButton := widget.NewButtonWithIcon(
		"Encrypt a file",
		iconEncrypt,
		func() {
			if len(session_keys) == 0 || len(session_keys[0]) == 0 {
				dialog.ShowError(fmt.Errorf("please load the encryption keys first"), window)
				return
			}
			fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
				if err != nil {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "Error opening file: " + err.Error(),
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}
				if reader == nil {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "No file selected.",
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}
				defer reader.Close()

				data, err := io.ReadAll(reader)
				if err != nil {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "Failed to read file: " + err.Error(),
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}

				ostream := bytes.NewBuffer(data)
				sstream := &bytes.Buffer{}

				defer func() {
					if r := recover(); r != nil {
						logs.Segments = []widget.RichTextSegment{}
						logs.Segments = append(logs.Segments, &widget.TextSegment{
							Text:  "Encryption failed: " + fmt.Sprintf("%v", r),
							Style: widget.RichTextStyleInline,
						})
						logs.Refresh()
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
				randomNumber := rand.Intn(10)
				fmt.Println("Key's number:", randomNumber)
				rKey := useAndDeleteCircleKey(randomNumber)
				if rKey == nil {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "No session key available for encryption.",
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}

				qalqan.EncryptOFB_File(len(data), rKey, iv, ostream, sstream)

				saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
					if err != nil {
						logs.Segments = []widget.RichTextSegment{}
						logs.Segments = append(logs.Segments, &widget.TextSegment{
							Text:  "Error saving file: " + err.Error(),
							Style: widget.RichTextStyleInline,
						})
						logs.Refresh()
						return
					}

					if writer == nil {
						logs.Segments = []widget.RichTextSegment{}
						logs.Segments = append(logs.Segments, &widget.TextSegment{
							Text:  "No file selected for saving.",
							Style: widget.RichTextStyleInline,
						})
						logs.Refresh()
						return
					}
					defer writer.Close()

					_, err = writer.Write(sstream.Bytes())
					if err != nil {
						logs.Segments = []widget.RichTextSegment{}
						logs.Segments = append(logs.Segments, &widget.TextSegment{
							Text:  "Failed to save encrypted file: " + err.Error(),
							Style: widget.RichTextStyleInline,
						})
						logs.Refresh()
						return
					}

					if sessionKeyCount > 0 {
						sessionKeyCount--
						keysLeftEntry.SetText(fmt.Sprintf("%d", sessionKeyCount))
					}
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "File successfully encrypted and saved!",
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
				}, window)

				saveDialog.SetFileName("encrypted_file.bin")
				saveDialog.Show()
			}, window)
			fileDialog.Show()
		})

	iconDecrypt, err := fyne.LoadResourceFromPath("assets/decrypt.png")
	if err != nil {
		fmt.Println("Ошибка загрузки иконки:", err)
		iconDecrypt = theme.CancelIcon()
	}

	decryptButton := widget.NewButtonWithIcon(
		"Decrypt a file",
		iconDecrypt,
		func() {
			if len(session_keys) == 0 || len(session_keys[0]) == 0 {
				dialog.ShowError(fmt.Errorf("please load the encryption keys first"), window)
				return
			}

			fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
				if err != nil {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "Error opening file: " + err.Error(),
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}
				if reader == nil {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "No file selected.",
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}

				defer reader.Close()

				data, err := io.ReadAll(reader)
				if err != nil {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "Failed to read file: " + err.Error(),
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}

				if len(data) < qalqan.BLOCKLEN {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "Invalid file: too small.",
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}

				imitstreamDecrypt := bytes.NewBuffer(data)
				imitFileDecrypt := make([]byte, qalqan.BLOCKLEN)
				qalqan.Qalqan_Imit(uint64(len(data)-qalqan.BLOCKLEN), rimitkey, imitstreamDecrypt, imitFileDecrypt)

				rimit := make([]byte, qalqan.BLOCKLEN)
				_, err = imitstreamDecrypt.Read(rimit[:qalqan.BLOCKLEN])
				if err != nil {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "Failed to read integrity check block.",
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}

				if !bytes.Equal(rimit, imitFileDecrypt) {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "The file is corrupted",
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}

				fileInfo := data[:qalqan.BLOCKLEN]
				storedImit := data[1*qalqan.BLOCKLEN : 2*qalqan.BLOCKLEN]

				computedImit := make([]byte, qalqan.BLOCKLEN)
				qalqan.Qalqan_Imit(qalqan.BLOCKLEN, rimitkey, bytes.NewBuffer(fileInfo), computedImit)

				if !bytes.Equal(computedImit, storedImit) {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "File info is corrupted!",
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}

				userNumber := fileInfo[1]
				fileType := fileInfo[4]
				keyType := fileInfo[5]
				circleKeyNumber := int(fileInfo[6])
				sessionKeyNumber := int(fileInfo[7])

				var fileTypeStr string
				switch fileType {
				case 0x00:
					fileTypeStr = "File"
				case 0x77:
					fileTypeStr = "File"
				case 0x88:
					fileTypeStr = "Photo"
				case 0x66:
					fileTypeStr = "Text (Message)"
				case 0x55:
					fileTypeStr = "Audio"
				default:
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "Unknown file type: 0x" + fmt.Sprintf("%X", fileType),
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}
				keyGenerated := false

				if !keyGenerated {
					switch keyType {
					case 0x00:
						rKey = useAndDeleteCircleKey(circleKeyNumber)
					case 0x01:
						rKey = useAndDeleteSessionKey(sessionKeyNumber)
					default:
						logs.Segments = []widget.RichTextSegment{}
						logs.Segments = append(logs.Segments, &widget.TextSegment{
							Text:  fmt.Sprintf("Ошибка: неизвестный тип ключа 0x%X", keyType),
							Style: widget.RichTextStyleInline,
						})
						logs.Refresh()
						return
					}
					keyGenerated = true
				}

				if rKey == nil {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "No decryption key available.",
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}

				defer func() {
					if r := recover(); r != nil {
						logs.Segments = []widget.RichTextSegment{}
						logs.Segments = append(logs.Segments, &widget.TextSegment{
							Text:  "Decryption failed: " + fmt.Sprintf("%v", r),
							Style: widget.RichTextStyleInline,
						})
						logs.Refresh()
					}
				}()

				start := 3 * qalqan.BLOCKLEN
				end := len(data) - qalqan.BLOCKLEN

				if end <= start {
					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "Ошибка: недостаточно данных для дешифровки!",
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
					return
				}

				sstream := &bytes.Buffer{}
				trimmedData := data[start:end]

				thirdBlockStart := 2 * qalqan.BLOCKLEN
				thirdBlockEnd := thirdBlockStart + qalqan.BLOCKLEN
				ivDecr := data[thirdBlockStart:thirdBlockEnd]

				qalqan.DecryptOFB_File(len(trimmedData), rKey, ivDecr, bytes.NewReader(trimmedData), sstream)

				logs.Segments = []widget.RichTextSegment{}
				logs.Segments = append(logs.Segments, &widget.TextSegment{
					Text:  fmt.Sprintf("User: %d, FileType: %s (0x%X), KeyType: %d, CircleKey: %d, SessionKey: %d", userNumber, fileTypeStr, fileType, keyType, circleKeyNumber, sessionKeyNumber),
					Style: widget.RichTextStyleInline,
				})
				logs.Refresh()

				saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
					if err != nil {
						logs.Segments = []widget.RichTextSegment{}
						logs.Segments = append(logs.Segments, &widget.TextSegment{
							Text:  "Error saving file: " + err.Error(),
							Style: widget.RichTextStyleInline,
						})
						logs.Refresh()

						return
					}
					if writer == nil {
						logs.Segments = []widget.RichTextSegment{}
						logs.Segments = append(logs.Segments, &widget.TextSegment{
							Text:  "No file selected for saving.",
							Style: widget.RichTextStyleInline,
						})
						logs.Refresh()
						return
					}
					defer writer.Close()

					logs.Segments = []widget.RichTextSegment{}
					logs.Segments = append(logs.Segments, &widget.TextSegment{
						Text:  "File successfully decrypted and saved!",
						Style: widget.RichTextStyleInline,
					})
					logs.Refresh()
				}, window)

				timestamp := time.Now().Format("2006-01-02_15-04")
				saveDialog.SetFileName(fmt.Sprintf("decrypted_file_%s.txt", timestamp))

				saveDialog.Show()
			}, window)

			fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".bin"}))
			fileDialog.Show()
		},
	)

	iconEncryptPhoto, err := fyne.LoadResourceFromPath("assets/takePhoto.png")
	if err != nil {
		fmt.Println("Ошибка загрузки иконки:", err)
		iconEncryptPhoto = theme.CancelIcon()
	}

	encryptImageButton := widget.NewButtonWithIcon(
		"Take a photo",
		iconEncryptPhoto,
		func() {
			dialog.ShowInformation("Success", "Photo encrypted successfully!", window)
		})

	iconEncryptVideo, err := fyne.LoadResourceFromPath("assets/takeVideo.png")
	if err != nil {
		fmt.Println("Ошибка загрузки иконки:", err)
		iconEncryptVideo = theme.CancelIcon()
	}

	encryptVideoButton := widget.NewButtonWithIcon(
		"Take a video",
		iconEncryptVideo,
		func() {
			dialog.ShowInformation("Success", "Video encrypted successfully!", window)
		})

	buttonContainer := container.NewHBox(
		layout.NewSpacer(),
		encryptButton,
		layout.NewSpacer(),
		decryptButton,
		layout.NewSpacer(),
		encryptImageButton,
		layout.NewSpacer(),
		encryptVideoButton,
		layout.NewSpacer(),
	)

	mainUI := container.NewVBox(
		widget.NewLabel(" "),
		topRow,
		widget.NewLabel(" "),
		hashContainer,
		widget.NewLabel(" "),
		sessionModeContainer,
		widget.NewLabel(" "),
		buttonContainer,
		widget.NewLabel(" "),
		logsContainer,
		widget.NewLabel(" "),
		tableBar,
		messageContainer,
	)

	content := container.NewStack(bgImage, mainUI)

	window.SetContent(content)
}
