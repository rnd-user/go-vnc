// Package vnc implements the client side of the Remote Framebuffer protocol, typically used in VNC clients.
//
// References:
//   [PROTOCOL]: http://tools.ietf.org/html/rfc6143
package vnc

import (
	"bufio"
	"fmt"
	"net"
)

type ClientConn struct {
	c               net.Conn
	r               *bufio.Reader
	config          *ClientConnConfig
	protocolVersion string
	securityType    SecurityType

	// encodingMap supported by the client. This should not be modified
	// directly. Instead, SetEncodings should be used.
	encodingMap map[EncodingType]Encoding

	// The pixel format associated with the connection. This shouldn't
	// be modified. If you wish to set a new pixel format, use the
	// SetPixelFormat method.
	pixelFormat *PixelFormat

	// Width of the frame buffer in pixels, sent from the server.
	FrameBufferWidth uint16

	// Height of the frame buffer in pixels, sent from the server.
	FrameBufferHeight uint16

	// Name associated with the desktop, sent from the server.
	DesktopName string
}

// A ClientConnConfig structure is used to configure a ClientConn. After
// one has been passed to initialize a connection, it must not be modified.
type ClientConnConfig struct {
	Address string

	// A slice of ClientAuth methods. Only the first instance that is
	// suitable by the server will be used to authenticate.
	Auth []ClientAuth

	// Exclusive determines whether the connection is shared with other
	// clients. If true, then all other clients connected will be
	// disconnected when a connection is established to the VNC server.
	Exclusive bool

	// A map of supported messages that can be read from the server.
	// This only needs to contain NEW server messages, and doesn't
	// need to explicitly contain the RFC-required messages.
	ServerMessages map[MessageID]ServerMessage
}

func NewClientConn(cfg *ClientConnConfig, c net.Conn) (*ClientConn, error) {
	if c == nil {
		var err error
		if c, err = net.Dial("tcp", cfg.Address); err != nil {
			return nil, err
		}
	}

	// add NoneAuth if no authentication method is selected
	if cfg.Auth == nil {
		cfg.Auth = []ClientAuth{&NoneAuth{}}
	}

	// add required messages
	msgs := []ServerMessage{
		&FramebufferUpdateMsg{},
		&SetColorMapEntriesMsg{},
		&BellMsg{},
		&ServerCutTextMsg{},
	}
	for _, m := range msgs {
		cfg.ServerMessages[m.ID()] = m
	}

	return &ClientConn{
		c:           c,
		r:           bufio.NewReader(c),
		config:      cfg,
		encodingMap: map[EncodingType]Encoding{RawEncType: &RawEncoding{}},
	}, nil
}

func (c *ClientConn) Close() error {
	return c.c.Close()
}

func (c *ClientConn) ReceiveMsg() (ServerMessage, error) {
	var mid MessageID
	if err := readFixedSize(c.r, &mid); err != nil {
		return nil, err
	}

	var m ServerMessage
	if m = c.config.ServerMessages[mid]; m == nil {
		return nil, fmt.Errorf("Unsupported Server Message %v.", mid)
	}

	var err error
	if m, err = m.Receive(c); err != nil {
		return nil, err
	}

	return m, nil
}

func (c *ClientConn) SendMsg(m ClientMessage) error {
	return m.Send(c)
}

func (c *ClientConn) PixelFormat() *PixelFormat {
	return c.pixelFormat
}
