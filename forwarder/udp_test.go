package forwarder

import (
	"bytes"
	"math/rand"
	"net"
	"testing"
	"time"
)

func TestForwardUDP(t *testing.T) {
	// echo server
	echoConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoConn.Close()

	done := false
	go func() {
		buf := getUDPBuf()
		for {
			n, from, err := echoConn.ReadFrom(buf)
			if err != nil {
				if !done {
					t.Error(err)
				}
				return
			}
			_, err = echoConn.WriteTo(buf[:n], from)
			if err != nil {
				if !done {
					t.Error(err)
				}
				return
			}
		}
	}()

	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := newUDPForwarder(l.(*net.UDPConn), echoConn.LocalAddr().String(), time.Second)
	go func() {
		if err := f.run(); err != nil {
			if !done {
				t.Error(err)
			}
		}
	}()

	for i := 0; i < 10; i++ {
		func() {
			clientConn, err := net.Dial("udp", l.LocalAddr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer clientConn.Close()

			payload := make([]byte, 1024)
			buf := getUDPBuf()
			for i := 0; i < 10; i++ {
				rand.Read(payload)
				if _, err := clientConn.Write(payload); err != nil {
					t.Fatal(err)
				}
				n, err := clientConn.Read(buf)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(payload, buf[:n]) {
					t.Fatal("broken data")
				}
			}
		}()
	}
	done = true

	// wait until timeout
	time.Sleep(time.Second * 2)
	if len(f.m) != 0 {
		t.Fatalf("res leaked")
	}
}
