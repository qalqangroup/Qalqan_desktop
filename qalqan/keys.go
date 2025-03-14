/*
_______________________________________________
					All keys:				   |
* [32] byte - Kikey;						   |
* [10][32] byte - Circle key;				   |
* [100][32] byte - Session key for count users;|
* [16] byte - imit.							   |
_______________________________________________|
			16 byte data on files			   |
* 0 - 0;									   |
* 1 - user number;							   |
* 2 - 0x04;									   |
* 3 - 0x20;									   |
* 4 - 0x77 - file,;						       |
	  0x88 - photo,						       |
	  0x66 - text (message),				   |
	  0x55 - audio.							   |
* 5 - circle or session key;				   |
* 6 - circle number key;;					   |
* 7 - session number key;;			           |
* 8 - 15 - 0x00;							   |
------------------------------------------------
*/

package qalqan

import (
	"bytes"
	"crypto/sha512"
	"fmt"
)

func Hash512(value string) [32]byte {
	hash := []byte(value)
	for i := 0; i < 1000; i++ {
		sum := sha512.Sum512(hash)
		hash = sum[:]
	}
	var hash32 [32]byte
	copy(hash32[:], hash[:32])
	return hash32
}

func LoadSessionKeys(data []byte, ostream *bytes.Buffer, rKey []byte, session_keys *[][100][32]byte) {
	readSessionKeys := make([]byte, DEFAULT_KEY_LEN)
	var usr_cnt int
	usr_cnt = len(data) - DEFAULT_KEY_LEN - 10*DEFAULT_KEY_LEN - BLOCKLEN
	usr_cnt = usr_cnt / (100 * DEFAULT_KEY_LEN)
	*session_keys = make([][100][32]byte, usr_cnt)
	for k := 0; k < usr_cnt; k++ {
		for i := 0; i < 100; i++ {
			_, err := ostream.Read(readSessionKeys[:DEFAULT_KEY_LEN])
			if err != nil {
				fmt.Println("Error reading session key:", err)
				return
			}
			for j := 0; j < DEFAULT_KEY_LEN; j += BLOCKLEN {
				DecryptOFB(readSessionKeys[j:j+BLOCKLEN], rKey, 32, 16, readSessionKeys[j:j+BLOCKLEN])
			}
			copy((*session_keys)[k][i][:], readSessionKeys[:])
		}
	}
}

func LoadCircleKeys(data []byte, ostream *bytes.Buffer, rKey []byte, circle_keys *[][10][32]byte) {
	readCircleKey := make([]byte, DEFAULT_KEY_LEN)
	*circle_keys = make([][10][32]byte, 1)
	for i := 0; i < 10; i++ {
		n, err := ostream.Read(readCircleKey[:DEFAULT_KEY_LEN])
		if err != nil {
			fmt.Printf("failed to read circular key %d: %v\n", i, err)
			return
		}
		if n < DEFAULT_KEY_LEN {
			fmt.Printf("unexpected EOF while reading circular key %d\n", i)
			return
		}
		for j := 0; j < DEFAULT_KEY_LEN; j += BLOCKLEN {
			DecryptOFB(readCircleKey[j:j+BLOCKLEN], rKey, DEFAULT_KEY_LEN, BLOCKLEN, readCircleKey[j:j+BLOCKLEN])
		}
		copy((*circle_keys)[0][i][:], readCircleKey[:])
	}
}
