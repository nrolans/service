package service

import (
	"net"
	"sync"
)

// General TCP service interface
type TCPServicer interface {
	Serve(net.TCPListener)
	Stop()
}

// Type to embed in more complex services
type BaseService struct {
	Stopper   chan bool
	WaitGroup sync.WaitGroup
}

func NewBaseService() *BaseService {
	b := new(BaseService)
	b.Stopper = make(chan bool)
	return b
}

// Stop the listener and wait for the handler to finish
func (s *BaseService) Stop() {
	close(s.Stopper)
	s.WaitGroup.Wait()
}
