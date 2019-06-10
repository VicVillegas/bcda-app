package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var private, encryptedKey, encryptedfilepath string

func init() {
	flag.StringVar(&encryptedKey, "key", "", "encrypted symmetric key used for file decryption (hex-encoded string)")
	flag.StringVar(&encryptedfilepath, "file", "", "location of encrypted file")
	flag.StringVar(&private, "pk", "", "location of private key to use for decryption of symmetric key")
	flag.Parse()

	if encryptedKey == "" || encryptedfilepath == "" || private == "" {
		fmt.Println("missing argument(s)")
		os.Exit(1)
	}
	r, _ := regexp.Compile("^[a-f0-9]{8}-?[a-f0-9]{4}-?4[a-f0-9]{3}-?[89ab][a-f0-9]{3}-?[a-f0-9]{12}")
	filename := path.Base(encryptedfilepath)
	uuid := strings.Split(filename, ".")[0]
	if !r.MatchString(uuid) {
		fmt.Printf("File name does not appear to be valid.\nPlease use the exact file name from the job status endpoint (i.e., of the format: <UUID>.ndjson).\n")
		os.Exit(2)
	}
}

func decryptCipher(ciphertext []byte, key *[32]byte) (plaintext []byte, err error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("malformed ciphertext")
	}
	return gcm.Open(nil,
		ciphertext[:gcm.NonceSize()],
		ciphertext[gcm.NonceSize():],
		nil,
	)
}

func decryptFile(privateKey *rsa.PrivateKey, encryptedKey []byte, filename string) {
	base := path.Base(filename)
	decryptedKey, err := rsa.DecryptOAEP(
		sha256.New(), rand.Reader, privateKey, encryptedKey, []byte(base))
	if err != nil {
		fmt.Println("Failed to decrypt encrypted key")
		os.Exit(3)
	}

	ciphertext, err := ioutil.ReadFile(filepath.Clean(filename))
	if err != nil {
		fmt.Println("Failed to read encrypted file")
		os.Exit(4)
	}

	var plaintext []byte
	key := [32]byte{}
	copy(key[:], decryptedKey[0:32])
	plaintext, err = decryptCipher(ciphertext, &key)
	if err != nil {
		fmt.Println("Failed to decrypt file")
		os.Exit(5)
	}

	fmt.Printf("%s", plaintext)
}

func getPrivateKey(loc string) *rsa.PrivateKey {

	pkFile, err := os.Open(filepath.Clean(loc))
	if err != nil {
		fmt.Println("Failed to open private key")
		os.Exit(6)
	}

	pemfileinfo, _ := pkFile.Stat()
	var size = pemfileinfo.Size()
	pembytes := make([]byte, size)
	buffer := bufio.NewReader(pkFile)

	_, err = buffer.Read(pembytes)
	if err != nil {
		fmt.Println("Failed to read private key")
		os.Exit(7)
	}

	data, _ := pem.Decode([]byte(pembytes))
	err = pkFile.Close()
	if err != nil {
		fmt.Println("Failed to close private key")
		os.Exit(8)
	}

	imported, err := x509.ParsePKCS1PrivateKey(data.Bytes)
	if err != nil {
		fmt.Println("Failed to parse private Key as PKCS1")
		os.Exit(9)
	}

	return imported
}

func main() {
	ek, err := hex.DecodeString(encryptedKey)
	if err != nil {
		fmt.Println("Failed to decode encrypted key")
		os.Exit(10)
	}
	pk := getPrivateKey(private)
	decryptFile(pk, ek, encryptedfilepath)
}
