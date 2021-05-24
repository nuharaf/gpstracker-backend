package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	yamux "github.com/hashicorp/yamux"
)

var session *yamux.Session
var eaddr = flag.String("eaddr", ":5555", "address for external connection")
var taddr = flag.String("taddr", ":5556", "address for tunnel connection")
var secret = flag.String("token", "token", "token for tunnel auth connection")
var certfile = flag.String("cert", "", "tls certificate file")
var keyfile = flag.String("key", "", "tls key file ")

var ylistener net.Listener
var listener net.Listener

func main() {
	flag.Parse()
	log.Printf("using external addr %s and tunnel addr %s", *eaddr, *taddr)

	var err error
	//open yamux port
	//no tls
	if *certfile == "" && *keyfile == "" {
		log.Println("starting non-tls listener")
		ylistener, err = net.Listen("tcp", *taddr)
		if err != nil {
			panic(err)
		}
	} else {
		log.Println("starting tls listener")
		cert, err := tls.LoadX509KeyPair(*certfile, *keyfile)
		if err != nil {
			panic(err)
		}
		tc := &tls.Config{Certificates: []tls.Certificate{cert}}
		ylistener, err = tls.Listen("tcp", *taddr, tc)
		if err != nil {
			panic(err)
		}
	}

	for {

		yconn, err := ylistener.Accept()
		log.Printf("accepting connection from %s\n", yconn.RemoteAddr().String())
		if err != nil {
			log.Print(err)
			continue
		}
		runServer(yconn)
		time.Sleep(2 * time.Second)
		log.Println("retrying")
	}

}

func runServer(yconn net.Conn) {
	// Accept a TCP connection

	token := make([]byte, 20)
	n, err := yconn.Read(token)
	if err != nil {
		log.Println(err)
		yconn.Close()
		return
	}
	if *secret != string(token[:n]) {
		_, _ = yconn.Write([]byte{'-'})
		yconn.Close()
		return
	} else {
		_, _ = yconn.Write([]byte{'+'})
	}
	log.Printf("establishing tunnel connection from %s\n", yconn.RemoteAddr())
	// Setup server side of yamux
	session, err = yamux.Server(yconn, nil)
	if err != nil {
		log.Printf("error trying to create server : %q", err)
		panic(err)
	}
	ylistener.Close()
	//open gps port
	listener, err = net.Listen("tcp", *eaddr)
	if err != nil {
		panic(err)
	}
	defer func() {
		log.Print("closing external listener")
		listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println(err)
			return
		}
		defer func() {
			log.Printf("closing connection from %s", conn.RemoteAddr())
			conn.Close()
		}()
		if session.IsClosed() {
			log.Printf("session is closed, retrying...")
			return
		}
		log.Printf("new connection from %s\n", conn.RemoteAddr())
		go func() {
			forward(conn)
			conn.Close()
			if session.IsClosed() {
				log.Printf("session is closed, closing listener and retrying...")
				listener.Close()
				return
			}
		}()
	}

}

func forward(conn net.Conn) {
	tstream, err := session.OpenStream()
	if err != nil {
		log.Printf("error trying to open stream : %q", err)
		return
	}
	log.Printf("new streamID : %d\n", tstream.StreamID())
	c := make(chan error)
	go func() {
		fmt.Fprintf(tstream, "%s\n", conn.RemoteAddr())
		_, err = io.Copy(tstream, conn)
		if err != nil {
			log.Printf("error copying to Stream %d from %s: %q", tstream.StreamID(), conn.RemoteAddr(), err)
		}
		tstream.Close()
		c <- err
	}()
	_, err0 := io.Copy(conn, tstream)
	if err0 != nil {
		log.Printf("error copying to %s from Stream %d: %q", conn.RemoteAddr(), tstream.StreamID(), err)
	}
	conn.Close()
	<-c
}
