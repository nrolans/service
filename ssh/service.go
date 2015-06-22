package ssh

import (
	"log"
	"net"

	"golang.org/x/crypto/ssh"

	"github.com/nrolans/service/tcp"
)

type SSHService struct {
	tcp.TCPService
	HandlerFunc func(*ssh.ServerConn, <-chan ssh.NewChannel, <-chan *ssh.Request)
}

func NewSSHService(handlerFunc func(*ssh.ServerConn, <-chan ssh.NewChannel, <-chan *ssh.Request), config *ssh.ServerConfig) *SSHService {
	s := new(SSHService)
	fn := func(t *net.TCPConn) {
		tcpHandler(t, config, handlerFunc)
	}
	t := tcp.NewTCPService(fn)
	s.TCPService = *t
	return s
}

func (s *SSHService) Serve(listener net.TCPListener) {
	s.TCPService.Serve(listener)
}

func (s *SSHService) Stop() {
	s.TCPService.Stop()
}

func tcpHandler(c *net.TCPConn, config *ssh.ServerConfig, handlerFunc func(*ssh.ServerConn, <-chan ssh.NewChannel, <-chan *ssh.Request)) {
	srvConn, chans, reqs, err := ssh.NewServerConn(c, config)
	if err != nil {
		log.Printf("SSH handshake failed: %s", err)
		return
	}
	handlerFunc(srvConn, chans, reqs)
}
