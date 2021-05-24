package util

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type BuffConn struct {
	rw   io.ReadWriter
	conn *net.TCPConn
}

func (bc *BuffConn) Read(p []byte) (n int, err error) {
	return bc.rw.Read(p)
}

func (bc *BuffConn) Write(p []byte) (n int, err error) {
	return bc.rw.Write(p)
}

func (bc *BuffConn) Close() error {
	return bc.conn.Close()
}

// From: https://blog.questionable.services/article/generating-secure-random-numbers-crypto-rand/

// genRandomString returns a URL-safe, base64 encoded securely generated random
// string.  It will return an error if the system's secure random number
// generator fails to function correctly, in which case the caller should not
// continue.
func GenRandomString(d []byte, n int) string {
	b := append(d, GenRandomBytes(n)...)
	return encode(b)
}

// genRandomBytes returns securely generated random bytes.  It will return an
// error if the system's secure random number generator fails to function
// correctly, in which case the caller should not continue.
func GenRandomBytes(n int) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	// Note that err != nil when we fail to read len(b) bytes.
	if err != nil {
		panic(err)
	}
	return b
}

func encode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func JsonWrite(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(v)
	if err != nil {
		panic(err)
	}
}

func Pan1c(err error) {
	if err != nil {
		panic(err)
	}
}

func CryptPwd(password string) string {
	x, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		panic(err)
	}
	return string(x)
}

func GenUUID() string {
	x, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}
	return x.String()
}
func GenUUIDb() []byte {
	x, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}
	d, _ := x.MarshalBinary()
	return d
}
