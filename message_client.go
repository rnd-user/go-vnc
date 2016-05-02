package vnc

import (
	"bytes"
	"fmt"
	"unicode"
)

const (
	SetPixelFormatMID MessageID = iota
	_
	SetEncodingsMID
	FramebufferUpdateRequestMID
	KeyEventMID
	PointerEventMID
	ClientCutTextMID
)

type SetPixelFormatMsg struct {
	ID MessageID
	_  [3]byte // padding
	RFBPixelFormat
}

func (m *SetPixelFormatMsg) Send(c *ClientConn) error {
	if err := writeFixedSize(c.c, m); err != nil {
		return err
	}

	c.pixelFormat = NewPixelFormat(&m.RFBPixelFormat)
	return nil
}

type SetEncodingsMsg struct {
	ID        MessageID
	Encodings []Encoding
}

func (m *SetEncodingsMsg) Send(c *ClientConn) error {
	numEncs := len(m.Encodings)
	encTypes := make([]EncodingType, 0, numEncs)
	encMap := make(map[EncodingType]Encoding, numEncs+1)
	encMap[RawEncType] = &RawEncoding{}

	for _, e := range m.Encodings {
		t := e.Type()
		encTypes = append(encTypes, t)
		encMap[t] = e
	}

	buf := make([]byte, 2, 4+4*numEncs)
	buf[0] = byte(m.ID)
	w := bytes.NewBuffer(buf)

	if err := writeFixedSize(w, uint16(numEncs)); err != nil {
		return err
	} else if err = writeFixedSize(w, encTypes); err != nil {
		return err
	} else if _, err = c.c.Write(w.Bytes()); err != nil {
		return err
	}

	// set encoding map
	c.encodingMap = encMap

	return nil
}

type FramebufferUpdateRequestMsg struct {
	ID          MessageID
	Incremental uint8
	X           uint16
	Y           uint16
	Width       uint16
	Height      uint16
}

func (m *FramebufferUpdateRequestMsg) Send(c *ClientConn) error {
	return writeFixedSize(c.c, m)
}

type KeyEventMsg struct {
	ID       MessageID
	DownFlag uint8
	_        [2]byte // padding
	Key      uint32
}

func (m *KeyEventMsg) Send(c *ClientConn) error {
	return writeFixedSize(c.c, m)
}

type PointerEventMsg struct {
	ID         MessageID
	ButtonMask uint8
	X          uint16
	Y          uint16
}

func (m *PointerEventMsg) Send(c *ClientConn) error {
	return writeFixedSize(c.c, m)
}

type ClientCutTextMsg struct {
	ID   MessageID
	Text string // Latin-1 (ISO 8859-1) characters only
}

func (m *ClientCutTextMsg) Send(c *ClientConn) error {
	for _, char := range m.Text {
		if char > unicode.MaxLatin1 {
			return fmt.Errorf("Character '%s' is not valid Latin-1", char)
		}
	}

	textBytes := []byte(m.Text)
	textLength := uint32(len(textBytes))
	buf := make([]byte, 4, 8+textLength)
	buf[0] = byte(m.ID)
	w := bytes.NewBuffer(buf)

	if err := writeFixedSize(w, textLength); err != nil {
		return err
	} else if _, err = w.Write(textBytes); err != nil {
		return err
	} else if _, err = c.c.Write(w.Bytes()); err != nil {
		return err
	}

	return nil
}
