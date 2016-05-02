package vnc

import (
	"fmt"
	"io"
)

const (
	ProtocolVersion3_3 = "RFB 003.003\n"
	ProtocolVersion3_7 = "RFB 003.007\n"
	ProtocolVersion3_8 = "RFB 003.008\n"
)

func (c *ClientConn) Handshake() (err error) {
	if err = c.hsProtocolVersion(); err != nil {
		return err
	}
	if err = c.hsSecurity(); err != nil {
		return err
	}
	if err = c.hsInit(); err != nil {
		return err
	}
	return
}

func (c *ClientConn) hsProtocolVersion() error {
	pvBuf := make([]byte, 12)

	// 7.1.1, read the ProtocolVersion message sent by the server.
	if _, err := io.ReadFull(c.r, pvBuf); err != nil {
		return err
	}

	var major, minor int
	if n, err := fmt.Sscanf(string(pvBuf), "RFB %d.%d\n", &major, &minor); err != nil {
		return err
	} else if n != 2 {
		return fmt.Errorf("Invalid Protocol Version format.")
	} else if major != 3 || minor < 3 {
		return fmt.Errorf("Unsupported Protocol Version.")
	}

	if minor < 7 {
		c.protocolVersion = ProtocolVersion3_3
	} else if minor == 7 {
		c.protocolVersion = ProtocolVersion3_7
	} else {
		c.protocolVersion = ProtocolVersion3_8
	}

	// Respond with the version we will support
	if _, err := c.c.Write([]byte(c.protocolVersion)); err != nil {
		return err
	}

	return nil
}

func (c *ClientConn) hsSecurity() error {
	var auth ClientAuth

	if c.protocolVersion >= ProtocolVersion3_7 {
		var numSecTypes uint8
		if err := readFixedSize(c.r, &numSecTypes); err != nil {
			return err
		} else if numSecTypes == 0 {
			if reason, err := c.hsErrorReason(); err != nil {
				return fmt.Errorf("No security types.")
			} else {
				return fmt.Errorf("No security types. Reason: %s", reason)
			}
		}

		serverSecTypes := make([]SecurityType, numSecTypes)
		if err := readFixedSize(c.r, serverSecTypes); err != nil {
			return err
		}

		clientSecTypes := c.config.Auth
	FindAuth:
		for _, curAuth := range clientSecTypes {
			for _, secType := range serverSecTypes {
				if curAuth.Type() == secType {
					// We use the first matching supported authentication
					auth = curAuth
					break FindAuth
				}
			}
		}
		if auth == nil {
			return fmt.Errorf("No suitable Auth scheme found. Server supported: %#v", serverSecTypes)
		}

		// Respond back with the security type we'll use
		if err := writeFixedSize(c.c, auth.Type()); err != nil {
			return err
		}

	} else { // v3.3
		var secType uint32
		if err := readFixedSize(c.r, &secType); err != nil {
			return err
		} else if secType == 0 { // Connection failed
			return fmt.Errorf("Failed to connect.")
		}

		for _, curAuth := range c.config.Auth {
			if curAuth.Type() == SecurityType(secType) {
				// We use the first matching supported authentication
				auth = curAuth
				break
			}
		}
		if auth == nil {
			return fmt.Errorf("No suitable Auth scheme found. Server requested: %d", secType)
		}
	}

	c.securityType = auth.Type()
	if err := auth.Handshake(c); err != nil {
		return err
	}

	if c.securityType == NoneSecType && c.protocolVersion < ProtocolVersion3_8 {
		return nil
	} else {
		return c.hsSecurityResult()
	}
}

func (c *ClientConn) hsSecurityResult() error {
	// 7.1.3 SecurityResult Handshake
	var secResult uint32
	if err := readFixedSize(c.r, &secResult); err != nil {
		return err
	}

	var errMsg string
	switch secResult {
	case 0:
		return nil
	case 1:
		errMsg = "Security handshake failed."
	case 2:
		errMsg = "Security handshake failed (too many attempts)."
	}

	if c.protocolVersion >= ProtocolVersion3_8 {
		if reason, err := c.hsErrorReason(); err == nil {
			errMsg = fmt.Sprintf("%s Reason: %s", errMsg, reason)
		}
	}

	return fmt.Errorf(errMsg)
}

func (c *ClientConn) hsInit() error {
	// 7.3.1 ClientInit
	var sharedFlag uint8 = 1
	if c.config.Exclusive {
		sharedFlag = 0
	}

	if err := writeFixedSize(c.c, sharedFlag); err != nil {
		return err
	}

	// 7.3.2 ServerInit
	if err := readFixedSize(c.r, &c.FrameBufferWidth); err != nil {
		return err
	}

	if err := readFixedSize(c.r, &c.FrameBufferHeight); err != nil {
		return err
	}

	// read pixel format
	rpf := new(RFBPixelFormat)
	if err := readFixedSize(c.r, rpf); err != nil {
		return err
	}
	c.pixelFormat = NewPixelFormat(rpf)

	// read desktop name
	var nameLength uint32
	if err := readFixedSize(c.r, &nameLength); err != nil {
		return err
	}

	nameBytes := make([]byte, nameLength)
	if err := readFixedSize(c.r, nameBytes); err != nil {
		return err
	}
	c.DesktopName = string(nameBytes)

	// there's more if Tight Security Type is chosen

	return nil
}

func (c *ClientConn) hsErrorReason() (string, error) {
	var reasonLen uint32
	if err := readFixedSize(c.r, &reasonLen); err != nil {
		return "", err
	}

	reason := make([]byte, reasonLen)
	if _, err := io.ReadFull(c.r, reason); err != nil {
		return "", err
	}

	return string(reason), nil
}
