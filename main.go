package main

import (
	"flag"
	"github.com/IrineSistiana/udpcat/forwarder"
	"log"
	"os"
	"os/signal"
	"time"
)

var (
	listen  = flag.String("l", "", "listen addr")
	dst     = flag.String("d", "", "destination addr")
	timeout = flag.Duration("t", time.Second*300, "timeout")

	udp     = flag.Bool("u", false, "forward udp")
	udpOnly = flag.Bool("U", false, "only forward udp (disable tcp forwarding)")
)

func main() {
	flag.Parse()

	if len(*listen) == 0 || len(*dst) == 0 {
		log.Fatal("-l or -s is missing")
	}

	if !*udpOnly {
		log.Printf("forward tcp: %s --> %s", *listen, *dst)
		go func() {
			err := forwarder.ForwardTCP(*listen, *dst, *timeout)
			if err != nil {
				log.Fatal(err)
			}
		}()
	}

	if *udp {
		log.Printf("forward udp: %s --> %s", *listen, *dst)
		go func() {
			err := forwarder.ForwardUDP(*listen, *dst, *timeout)
			if err != nil {
				log.Fatal(err)
			}
		}()
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, os.Interrupt, os.Kill)
	select {
	case s := <-sc:
		log.Printf("recevied signal %s, exiting", s)
		os.Exit(0)
	}
}
