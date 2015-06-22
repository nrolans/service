package scp

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

type TransferStatus int

const (
	Wait TransferStatus = iota
	Ready
	Refused
)

// Status codes
type StatusCode byte

const (
	OK      StatusCode = '\000'
	Warning StatusCode = '\001'
	Fatal   StatusCode = '\002'
)

type Status struct {
	Code    StatusCode
	Message string
}

func NewStatus(sc StatusCode, msg string) *Status {
	s := new(Status)
	s.Code = sc
	s.Message = msg
	return s
}

func (s Status) Bytes() []byte {
	buffer := bytes.NewBufferString("")
	buffer.WriteByte((byte)(s.Code))
	if s.Code != OK {
		buffer.WriteString(s.Message)
		buffer.WriteString("\n")
	}
	return buffer.Bytes()
}

// Parameters
type Parameter int

const (
	Verbose Parameter = 1 << iota
	Directory
	Sink
	Source
	Recursive
)

func (p *Parameter) Parse(params string) {
	for _, param := range params {
		switch strings.ToLower(string(param)) {
		case "d":
			*p += Directory
		case "f":
			*p += Source
		case "r":
			*p += Recursive
		case "t":
			*p += Sink
		case "v":
			*p += Verbose
		}
	}
}

func (p Parameter) String() string {
	str := make([]string, 0)
	if p^Verbose > 0 {
		str = append(str, "Verbose")
	}
	if p^Directory > 0 {
		str = append(str, "Directory")
	}
	if p^Sink > 0 {
		str = append(str, "Sink")
	}
	if p^Source > 0 {
		str = append(str, "Source")
	}
	if p^Recursive > 0 {
		str = append(str, "Recursive")
	}
	return strings.Join(str, ",")
}

// Handler interfaces
type SinkHandler interface {
	SinkRequest(conn ssh.ServerConn, parameters Parameter, pattern string) bool
	FileRequest(mode os.FileMode, size int64, filename string) Status
	FileCopy(r io.Reader) Status
	DirRequest(mode os.FileMode, size int64, dirname string) Status
	DirEndRequest() Status
}

type SourceHandler interface{}

func parseSCPCopy(line string) (mode os.FileMode, length int64, filename string, err error) {

	tokens := strings.SplitN(line, " ", 3)
	if len(tokens[0]) != 5 || tokens[0][0] != 'C' {
		err = fmt.Errorf("SCP Parsing error: %s", line)
		return
	}

	var modeInt uint64
	modeInt, err = strconv.ParseUint(tokens[0][1:5], 10, 32)
	if err != nil {
		return
	}
	mode = (os.FileMode)(modeInt)

	length, err = strconv.ParseInt(tokens[1], 10, 64)
	if err != nil {
		return
	}

	filename = tokens[2]

	return
}

type exitStatus struct {
	Status uint32
}
