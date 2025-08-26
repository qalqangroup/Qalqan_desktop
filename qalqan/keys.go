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
	const perUser = 100 * DEFAULT_KEY_LEN

	rem := ostream.Len()
	if rem < BLOCKLEN {
		fmt.Println("LoadSessionKeys: not enough data (no room for IMIT)")
		return
	}
	sessBytes := rem - BLOCKLEN
	if sessBytes%perUser != 0 {
		fmt.Printf("LoadSessionKeys: malformed length: %d is not multiple of %d\n", sessBytes, perUser)
		return
	}
	usr_cnt := sessBytes / perUser
	if usr_cnt <= 0 || usr_cnt > 255 {
		fmt.Printf("LoadSessionKeys: suspicious user count: %d\n", usr_cnt)
		return
	}

	*session_keys = make([][100][32]byte, usr_cnt)

	readSessionKey := make([]byte, DEFAULT_KEY_LEN)
	for u := 0; u < usr_cnt; u++ {
		for i := 0; i < 100; i++ {
			n, err := ostream.Read(readSessionKey[:DEFAULT_KEY_LEN])
			if err != nil {
				fmt.Println("LoadSessionKeys: error reading session key:", err)
				return
			}
			if n != DEFAULT_KEY_LEN {
				fmt.Println("LoadSessionKeys: unexpected EOF in session key")
				return
			}
			for j := 0; j < DEFAULT_KEY_LEN; j += BLOCKLEN {
				DecryptOFB(readSessionKey[j:j+BLOCKLEN], rKey, DEFAULT_KEY_LEN, BLOCKLEN, readSessionKey[j:j+BLOCKLEN])
			}
			copy((*session_keys)[u][i][:], readSessionKey[:])
		}
	}
}

func LoadCircleKeys(data []byte, ostream *bytes.Buffer, rKey []byte, circle_keys *[10][32]byte) {
	*circle_keys = [10][32]byte{}

	if ostream.Len() < 10*DEFAULT_KEY_LEN {
		fmt.Printf("LoadCircleKeys: not enough data for 10 circle keys (have %d)\n", ostream.Len())
		return
	}

	readCircleKey := make([]byte, DEFAULT_KEY_LEN)
	for i := 0; i < 10; i++ {
		n, err := ostream.Read(readCircleKey[:DEFAULT_KEY_LEN])
		if err != nil {
			fmt.Printf("LoadCircleKeys: failed to read circle key %d: %v\n", i, err)
			return
		}
		if n != DEFAULT_KEY_LEN {
			fmt.Printf("LoadCircleKeys: unexpected EOF while reading circle key %d\n", i)
			return
		}
		for j := 0; j < DEFAULT_KEY_LEN; j += BLOCKLEN {
			DecryptOFB(readCircleKey[j:j+BLOCKLEN], rKey, DEFAULT_KEY_LEN, BLOCKLEN, readCircleKey[j:j+BLOCKLEN])
		}
		copy((*circle_keys)[i][:], readCircleKey[:])
	}
}