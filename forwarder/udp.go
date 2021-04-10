package forwarder

import (
	"errors"
	"log"
	"net"
	"sync"
	"time"
)

func ForwardUDP(src, dst string, timeout time.Duration) error {
	l, err := net.ListenPacket("udp", src)
	if err != nil {
		return err
	}
	f := newUDPForwarder(l.(*net.UDPConn), dst, timeout)
	return f.run()
}

type addr struct {
	ip   [16]byte
	port int
}

func udpAddr2Addr(ua *net.UDPAddr) (a addr) {
	copy(a.ip[:], ua.IP)
	a.port = ua.Port
	return
}

type udpForwarder struct {
	listen  *net.UDPConn
	dst     string
	timeout time.Duration

	ml           sync.RWMutex
	m            map[addr]chan []byte
	closeOnce    sync.Once
	closedNotify chan struct{}
}

const udpBufSize = 2048

var udpBufPool = sync.Pool{New: func() interface{} { return make([]byte, udpBufSize) }}

func getUDPBuf() []byte {
	return udpBufPool.Get().([]byte)
}

func releaseUDPBuf(b []byte) {
	if cap(b) != udpBufSize {
		panic("unexpected buf size")
	}
	udpBufPool.Put(b[:udpBufSize])
}

func newUDPForwarder(src *net.UDPConn, dst string, timeout time.Duration) *udpForwarder {
	return &udpForwarder{
		listen:       src,
		dst:          dst,
		timeout:      timeout,
		m:            make(map[addr]chan []byte),
		closedNotify: make(chan struct{}),
	}
}

func (f *udpForwarder) run() error {
	for {
		buf := getUDPBuf()
		n, src, err := f.listen.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				log.Printf("WARNNING: temporary err: %v", err)
				time.Sleep(time.Second)
			}
		}

		f.ml.RLock()
		cc, ok := f.m[udpAddr2Addr(src)]
		f.ml.RUnlock()

		if ok { //send payload to workers
			select {
			case cc <- buf[:n]:
			default:
			}
		} else { // create a new worker
			f.newWorkerGoroutine(src, buf[:n])
		}
	}
}

var errForwarderClosed = errors.New("forwarder is closed")

func (f *udpForwarder) close() {
	f.closeOnce.Do(func() {
		close(f.closedNotify)
	})
}

func (f *udpForwarder) newWorkerGoroutine(src *net.UDPAddr, firstPayload []byte) {
	dc, err := net.Dial("udp", f.dst)
	if err != nil {
		log.Printf("ERROR: cannot dial dst: %v", err)
		return
	}

	wc := make(chan []byte, 512) // buffed chan
	wc <- firstPayload
	f.ml.Lock()
	f.m[udpAddr2Addr(src)] = wc // register this worker
	f.ml.Unlock()

	go func() {
		defer func() { // clean up
			dc.Close()
			f.ml.Lock()
			delete(f.m, udpAddr2Addr(src)) // remove this worker
			f.ml.Unlock()
		}()

		deadOnce := sync.Once{}
		connDeadNotify := make(chan struct{})
		var deadErr error
		connDead := func(err error) {
			deadOnce.Do(func() {
				dc.Close()            // this breaks read loop
				close(connDeadNotify) // this breaks write loop
				deadErr = err
			})
		}

		wg := new(sync.WaitGroup)
		wg.Add(2)

		// write loop
		go func() {
			defer wg.Done()
			for {
				dc.SetDeadline(time.Now().Add(f.timeout))
				select {
				case payload := <-wc:
					_, err := dc.Write(payload)
					releaseUDPBuf(payload)
					if err != nil {
						connDead(err)
						return
					}
				case <-connDeadNotify:
					return
				case <-f.closedNotify:
					connDead(errForwarderClosed)
					return
				}
			}
		}()

		// read loop
		go func() {
			defer wg.Done()
			buf := getUDPBuf()
			for {
				dc.SetDeadline(time.Now().Add(f.timeout))
				n, err := dc.Read(buf)
				if err != nil {
					connDead(err)
					return
				}

				if _, err := f.listen.WriteToUDP(buf[:n], src); err != nil {
					connDead(err)
					return
				}
			}
		}()

		wg.Wait()
		if deadErr != nil {
			if netErr, ok := deadErr.(net.Error); ok && netErr.Timeout() { // ignore timeout err
				return
			}
			log.Printf("WARNNING: %v", deadErr)
		}
	}()
}
