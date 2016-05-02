package vnc

import (
	"fmt"
	"io"
)

const (
	FramebufferUpdateMID MessageID = iota
	SetColorMapEntriesMID
	BellMID
	ServerCutTextMID
)

// FramebufferUpdateMsg consists of a sequence of rectangles of
// pixel data that the client should put into its framebuffer.
type FramebufferUpdateMsg struct {
	Rectangles []Rectangle
}

func (*FramebufferUpdateMsg) ID() MessageID {
	return FramebufferUpdateMID
}

func (*FramebufferUpdateMsg) Receive(c *ClientConn) (ServerMessage, error) {
	// Read off the padding
	padding := make([]byte, 1)
	if _, err := io.ReadFull(c.r, padding); err != nil {
		return nil, err
	}

	var numRects uint16
	if err := readFixedSize(c.r, &numRects); err != nil {
		return nil, err
	}

	rects := make([]Rectangle, numRects)
	for i := uint16(0); i < numRects; i++ {
		rect := &rects[i]

		box := []*uint16{&rect.X, &rect.Y, &rect.Width, &rect.Height}
		for _, val := range box {
			if err := readFixedSize(c.r, val); err != nil {
				return nil, err
			}
		}

		var encType EncodingType
		if err := readFixedSize(c.r, &encType); err != nil {
			return nil, err
		}
		enc, ok := c.encodingMap[encType]
		if !ok {
			return nil, fmt.Errorf("unsupported encoding type: %d", encType)
		}

		var err error
		rect.Encoding, err = enc.Read(c, rect)
		if err != nil {
			return nil, err
		}
	}

	return &FramebufferUpdateMsg{rects}, nil
}

// SetColorMapEntriesMsg is sent by the server to set values into
// the color map. This message will automatically update the color map
// for the associated connection, but contains the color change data
// if the consumer wants to read it.
//
// See RFC 6143 Section 7.6.2
type SetColorMapEntriesMsg struct {
	FirstColor uint16
	Colors     ColorMap
}

func (*SetColorMapEntriesMsg) ID() MessageID {
	return SetColorMapEntriesMID
}

func (*SetColorMapEntriesMsg) Receive(c *ClientConn) (ServerMessage, error) {
	padding := make([]byte, 1)
	if _, err := io.ReadFull(c.r, padding); err != nil {
		return nil, err
	}

	msg := &SetColorMapEntriesMsg{}
	if err := readFixedSize(c.r, &msg.FirstColor); err != nil {
		return nil, err
	}

	var numColors uint16
	if err := readFixedSize(c.r, &numColors); err != nil {
		return nil, err
	}

	msg.Colors = make(ColorMap, numColors)
	if err := readFixedSize(c.r, msg.Colors); err != nil {
		return nil, err
	}

	return msg, nil
}

// Bell signals that an audible bell should be made on the client.
//
// See RFC 6143 Section 7.6.3
type BellMsg struct{}

func (*BellMsg) ID() MessageID {
	return BellMID
}

func (*BellMsg) Receive(*ClientConn) (ServerMessage, error) {
	return &BellMsg{}, nil
}

// ServerCutTextMsg indicates the server has new text in the cut buffer.
//
// See RFC 6143 Section 7.6.4
type ServerCutTextMsg struct {
	Text string
}

func (*ServerCutTextMsg) ID() MessageID {
	return ServerCutTextMID
}

func (*ServerCutTextMsg) Receive(c *ClientConn) (ServerMessage, error) {
	padding := make([]byte, 3)
	if _, err := io.ReadFull(c.r, padding); err != nil {
		return nil, err
	}

	var textLength uint32
	if err := readFixedSize(c.r, &textLength); err != nil {
		return nil, err
	}

	textBytes := make([]byte, textLength)
	if _, err := io.ReadFull(c.r, textBytes); err != nil {
		return nil, err
	}

	return &ServerCutTextMsg{string(textBytes)}, nil
}
