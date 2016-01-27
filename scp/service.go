package scp

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	srvssh "github.com/nrolans/service/ssh"
	"golang.org/x/crypto/ssh"

	"strings"
	"sync"
)

// Config
type SCPConfig struct {
	*ssh.ServerConfig
}

type SCPService struct {
	srvssh.SSHService
	HandlerFunc func(*net.TCPConn, *SCPConfig)
	config      *SCPConfig
	Sink        SinkHandler
	Source      SourceHandler
}

func NewSCPService(config *SCPConfig, sinkHandler SinkHandler, srcHandler SourceHandler) *SCPService {
	scp := new(SCPService)
	scp.config = config
	fn := func(srvConn *ssh.ServerConn, chans <-chan ssh.NewChannel, reqs <-chan *ssh.Request) {
		sshHandler(srvConn, chans, reqs, sinkHandler, srcHandler)
	}
	scp.SSHService = *srvssh.NewSSHService(fn, scp.config.ServerConfig)
	return scp
}

func (s *SCPService) Serve(listener net.TCPListener) {
	s.SSHService.Serve(listener)
}

func (s *SCPService) Stop() {
	s.SSHService.Stop()
}

func sshHandler(srvConn *ssh.ServerConn, chans <-chan ssh.NewChannel, reqs <-chan *ssh.Request, sink SinkHandler, source SourceHandler) {
	// log.Printf("Received new connection: %s", srvConn.RemoteAddr())

	// Discard the global requests
	go ssh.DiscardRequests(reqs)

	// Handle the new channels
	for newChannel := range chans {

		// Reject non-session channels
		if newChannel.ChannelType() != "session" {
			log.Printf("Rejecting channel of type %s", newChannel.ChannelType())
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		// Accept the sesssion channel
		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Println("Failed to accept session channel")
			continue
		}
		//log.Printf("Accepted new channel")

		var params Parameter
		var pattern string
		var transferStatus = Wait
		var transferCond = sync.NewCond(&sync.Mutex{})

		// Asynchronously handle requests on this channel
		// We wait until we receive the exec request with the SCP command,
		// or timeout after 10 seconds
		go func(in <-chan *ssh.Request) {
			execReceived := false
			execTimer := time.After(10 * time.Second)

			for {
				select {
				case <-execTimer:
					log.Println("Timed out while waiting for transfer request")
					// Ran out of time, cleanup and close the connection
					transferStatus = Refused
					transferCond.Signal()
				case req, open := <-in:
					if !open {
						return
					}
					//log.Printf("Received new requests: %s", req.Type)

					// Handle the request
					ok := false
					switch req.Type {
					case "exec":
						if execReceived {
							// We have already received the exec scp command, refuse new ones
							ok = false
						} else {

							// We haven't received the SCP command yet, checking this exec request
							//log.Printf("Received request type %s: <%s>", req.Type, req.Payload[4:])
							tokens := strings.Split(string(req.Payload[4:]), " ")
							if tokens[0] != "scp" {
								log.Printf("Unexpected exec command: %s", string(req.Payload[4:]))
								ok = false
								break
							}

							// Read the direction and filename
							for _, token := range tokens[1:] {
								// Skip double spaces
								if len(token) == 0 {
									continue
								}
								if token[0] == '-' && len(token) > 1 {
									params.Parse(token[1:])
								} else if pattern == "" {
									pattern = token
								}
							}

							// Sanity checks before calling the handler
							if pattern == "" {
								log.Printf("Missing pattern in scp command: %s", req.Payload[4:])
								ok = false
								transferStatus = Refused
								break
							}

							if params&Sink > 0 && params&Source > 0 {
								log.Printf("Cannot be sink and source: %s", req.Payload[4:])
								ok = false
								transferStatus = Refused
								break
							}

							if params&Sink == 0 && params&Source == 0 {
								log.Printf("Must be sink or source: %s", req.Payload[4:])
								ok = false
								transferStatus = Refused
								break
							}

							// Sink request
							if params&Sink == Sink {
								if sink.SinkRequest(*srvConn, params, pattern) {
									transferStatus = Ready
									ok = true
									//log.Printf("Accepting sink request")
								} else {
									transferStatus = Refused
									ok = false
									log.Printf("Refusing sink request")
								}
							} else { // Source request
								// TODO: implement source
								transferStatus = Refused
								ok = false
								log.Printf("Refusing source request, not implemented")
							}

							execReceived = true
						}
					case "env":
						// Accept environment variable (rejecting makes some clients unhappy)
						ok = true
						//log.Println("Accepting env request")
					}

					// Reply if necessary
					req.Reply(ok, nil)

					// Unblock the channel code if we have made a decision
					if transferStatus != Wait {
						transferCond.Signal()
					}
				}
			}
		}(requests)

		// Wait for an exec request on the channel
		transferCond.L.Lock()
		for transferStatus == Wait {
			transferCond.Wait()
		}
		transferCond.L.Unlock()

		if transferStatus != Ready {
			continue
		}

		// Ready to receive SCP data!

		// Start the scp communication
		fmt.Fprint(channel, "\000")

		// Prepare a buffer to read from the channel
		databuf := bufio.NewReader(channel)

		// TODO: loop through requests!

		// Read the mode, length, filename
		line, err := databuf.ReadString('\n')
		if err != nil {
			log.Printf("Failed to read from channel: %s", err)
			continue
		}

		// log.Printf("Received line: %s", line)

		switch string(line[0]) {
		case "C": // File transfer
			mode, length, filename, err := parseSCPCopy(line)
			if err != nil {
				log.Println("Failed to parse SCP message: %s", err)
				continue
			}

			// Pass the request to the handler
			status := sink.FileRequest(mode, length, filename)
			channel.Write(status.Bytes())
			if status.Code == OK {
				status = sink.FileCopy(io.LimitReader(channel, length))
				channel.Write(status.Bytes())
			}
			break

		case "D":
			// Directory
			break

		case "E":
			// End directory
			break
		}

		// log.Printf("Received a file from %s\n", srvConn.RemoteAddr())

		// Send a channel request with a successful exit-status
		channel.SendRequest("exit-status", false, ssh.Marshal(exitStatus{0}))

		channel.Close()
	}
}
