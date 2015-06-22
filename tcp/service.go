package tcp

import (
	"log"
	"net"
	"time"

	"github.com/nrolans/service"
)

// Define a TCP service.
// SetDeadline() isn't part of the net.Listener interface, so we
// take a *net.TCPListener
type TCPService struct {
	service.BaseService
	HandlerFunc func(*net.TCPConn)
}

func NewTCPService(handlerFunc func(*net.TCPConn)) *TCPService {
	s := new(TCPService)
	b := service.NewBaseService()
	s.BaseService = *b
	s.HandlerFunc = handlerFunc
	return s
}

// Start accepting TCP connections and send them to goroutines
// The listener is expected to be a net.TCPListener
func (s *TCPService) Serve(listener net.TCPListener) {
	for {
		// Check if we should still run
		select {
		case <-s.BaseService.Stopper:
			listener.Close()
			log.Println("No longer accepting new requests")
			return
		default:
		}

		// Start accepting connections
		listener.SetDeadline(time.Now().Add(1 * time.Second))
		conn, err := listener.AcceptTCP()
		if nil != err {
			if opErr, ok := err.(*net.OpError); ok && (opErr.Timeout() || opErr.Temporary()) {
				continue
			}
			log.Println(err)
			return
		}

		// Handle the new connection
		s.BaseService.WaitGroup.Add(1)
		go func() {
			s.HandlerFunc(conn)
			s.BaseService.WaitGroup.Done()
		}()
	}
}

func (s *TCPService) Stop() {
	s.BaseService.Stop()
}
