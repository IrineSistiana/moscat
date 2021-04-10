package forwarder

import (
	"errors"
	"github.com/IrineSistiana/ctunnel"
	"log"
	"net"
	"time"
)

func ForwardTCP(src, dst string, timeout time.Duration) error {
	l, err := net.Listen("tcp", src)
	if err != nil {
		return err
	}
	f := tcpForwarder{
		listener: l,
		dst:      dst,
		timeout:  timeout,
	}
	return f.run()
}

type tcpForwarder struct {
	listener net.Listener
	dst      string
	timeout  time.Duration
}

func (f *tcpForwarder) run() error {
	// check
	if f.listener == nil {
		return errors.New("nil listener")
	}
	if len(f.dst) == 0 {
		return errors.New("empty destination address")
	}

	for {
		c, err := f.listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				time.Sleep(time.Second)
				continue
			}
		}

		go func() {
			if err := f.handle(c); err != nil {
				log.Printf("error: %v", err)
			}
		}()
	}
}

func (f *tcpForwarder) handle(src net.Conn) error {
	dst, err := net.Dial("tcp", f.dst)
	if err != nil {
		return err
	}
	return ctunnel.OpenTunnel(src, dst, f.timeout)
}
