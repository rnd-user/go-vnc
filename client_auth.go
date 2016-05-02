package vnc

import (
	"crypto/des"
	"io"
)

type SecurityType uint8

const (
	InvalidSecType = SecurityType(iota)
	NoneSecType
	VNCSecType
)

// A ClientAuth implements a method of authenticating with a remote server.
type ClientAuth interface {
	// Type returns the byte identifier sent by the server to
	// identify this authentication scheme.
	Type() SecurityType

	// Handshake is called when the authentication handshake should be
	// performed, as part of the general RFB handshake. (see 7.2.1)
	Handshake(*ClientConn) error
}

// NoneAuth is the "none" authentication. See 7.2.1
type NoneAuth struct{}

func (*NoneAuth) Type() SecurityType {
	return NoneSecType
}

func (*NoneAuth) Handshake(*ClientConn) error {
	return nil
}

// VNCAuth is VNC authentication, 7.2.2
type VNCAuth struct {
	Password string
}

func (a *VNCAuth) Type() SecurityType {
	return VNCSecType
}

func (a *VNCAuth) Handshake(c *ClientConn) error {
	challenge := make([]byte, 16)
	if _, err := io.ReadFull(c.r, challenge); err != nil {
		return err
	}

	crypted, err := a.encrypt(a.Password, challenge)
	if err != nil {
		return err
	}

	if _, err := c.c.Write(crypted); err != nil {
		return err
	}

	return nil
}

func (a *VNCAuth) encrypt(pw string, bytes []byte) ([]byte, error) {
	key := make([]byte, 8)
	copy(key, pw)

	// Each byte of the password needs to be reversed. This is a
	// non RFC-documented behaviour of VNC clients and servers
	for i := range key {
		key[i] = (key[i]&0x55)<<1 | (key[i]&0xAA)>>1 // Swap adjacent bits
		key[i] = (key[i]&0x33)<<2 | (key[i]&0xCC)>>2 // Swap adjacent pairs
		key[i] = (key[i]&0x0F)<<4 | (key[i]&0xF0)>>4 // Swap the 2 halves
	}

	cypher, err := des.NewCipher(key)
	if err != nil {
		return nil, err
	}

	result1 := make([]byte, 8)
	cypher.Encrypt(result1, bytes)
	result2 := make([]byte, 8)
	cypher.Encrypt(result2, bytes[8:])

	crypted := append(result1, result2...)

	return crypted, nil
}
