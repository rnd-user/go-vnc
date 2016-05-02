package vnc

import (
	"encoding/binary"
	"fmt"
	"io"
)

// PixelFormat describes the way a pixel is formatted for a VNC connection.
//
// See RFC 6143 Section 7.4 for information on each of the fields.
type PixelFormat struct {
	ByPP      uint8
	ByteOrder binary.ByteOrder
	*RFBPixelFormat

	// If the pixel format uses a color map, then this is the color
	// map that is used. This should not be modified directly, since
	// the data comes from the server.
	ColorMap
}

type RFBPixelFormat struct {
	BPP        uint8
	Depth      uint8
	BigEndian  uint8
	TrueColor  uint8
	RedMax     uint16
	GreenMax   uint16
	BlueMax    uint16
	RedShift   uint8
	GreenShift uint8
	BlueShift  uint8
	_          [3]byte
}

func NewPixelFormat(rpf *RFBPixelFormat) *PixelFormat {
	pf := new(PixelFormat)
	pf.RFBPixelFormat = rpf
	pf.ByPP = rpf.BPP / 8

	// Reset the color map as according to RFC.
	if rpf.TrueColor == 0 {
		pf.ColorMap = make([]Color, 256)
	} else {
		pf.ColorMap = nil
	}

	if rpf.BigEndian == 0 {
		pf.ByteOrder = binary.LittleEndian
	} else {
		pf.ByteOrder = binary.BigEndian
	}

	return pf
}

func (pf *PixelFormat) ReadPixels(r io.Reader, numPixels int) ([]byte, error) {
	pixelBuffer := make([]byte, pf.ByPP)
	rgbaSize := numPixels * 4
	rgbaBuffer := make([]byte, rgbaSize)
	for i := 0; i < rgbaSize; i += 4 {
		if _, err := io.ReadFull(r, pixelBuffer); err != nil {
			return nil, err
		}

		rgbaBuffer[i], rgbaBuffer[i+1], rgbaBuffer[i+2] = pf.pixelToRGB(pixelBuffer)
		rgbaBuffer[i+3] = 255
	}

	return rgbaBuffer, nil
}

func (pf *PixelFormat) pixelToRGB(buffer []byte) (r, g, b uint8) {
	var pixel uint32
	switch pf.ByPP {
	case 1:
		pixel = uint32(buffer[0])
	case 2:
		pixel = uint32(pf.ByteOrder.Uint16(buffer))
	case 4:
		pixel = pf.ByteOrder.Uint32(buffer)
	}

	if pf.TrueColor != 0 {
		r = pf.scaleToUint8((pixel>>pf.RedShift)&uint32(pf.RedMax), pf.RedMax)
		g = pf.scaleToUint8((pixel>>pf.GreenShift)&uint32(pf.GreenMax), pf.GreenMax)
		b = pf.scaleToUint8((pixel>>pf.BlueShift)&uint32(pf.BlueMax), pf.BlueMax)
	} else {
		cm := pf.ColorMap
		r = pf.scaleToUint8(uint32(cm[pixel].R), 65535)
		g = pf.scaleToUint8(uint32(cm[pixel].G), 65535)
		b = pf.scaleToUint8(uint32(cm[pixel].B), 65535)
	}
	return
}

// good enough for pixel values?
func (pf *PixelFormat) scaleToUint8(num uint32, max uint16) uint8 {
	return uint8(float64(num)*255/float64(max) + 0.5)
}

type Color struct {
	R, G, B uint16
}

type ColorMap []Color

func (cm ColorMap) UpdateColorMap(firstColor uint16, colors []Color) error {
	if n := len(colors); copy(cm[int(firstColor):int(firstColor)+n], colors) != n {
		return fmt.Errorf("error occurred while updating color map")
	}
	return nil
}
