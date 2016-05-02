package vnc

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
)

type EncodingType int32

const (
	RawEncType                     = EncodingType(0)
	CopyRectEncType                = EncodingType(1)
	HextileEncType                 = EncodingType(5)
	TightEncType                   = EncodingType(7) //
	DesktopSizePseudoEncType       = EncodingType(-223)
	CursorPseudoEncType            = EncodingType(-239)
	TightPNGEncType                = EncodingType(-260) //
	ContinuousUpdatesPseudoEncType = EncodingType(-313) //
)

// Rectangle represents a rectangle of pixel data.
type Rectangle struct {
	X      uint16
	Y      uint16
	Width  uint16
	Height uint16
	Encoding
}

// An Encoding implements a method for encoding pixel data that is
// sent by the server to the client.
type Encoding interface {
	// The number that uniquely identifies this encoding type.
	Type() EncodingType

	// Read reads the contents of the encoded pixel data from the reader.
	// This should return a new Encoding implementation that contains
	// the proper data.
	Read(*ClientConn, *Rectangle) (Encoding, error)
}

// RawEncoding is raw pixel data sent by the server.
//
// See RFC 6143 Section 7.7.1
type RawEncoding struct {
	rgba []byte
}

func (*RawEncoding) Type() EncodingType {
	return RawEncType
}

func (*RawEncoding) Read(c *ClientConn, rect *Rectangle) (Encoding, error) {
	var err error
	enc := new(RawEncoding)
	if enc.rgba, err = c.pixelFormat.ReadPixels(c.r, int(rect.Height)*int(rect.Width)); err != nil {
		return nil, err
	}

	return enc, nil
}

func (enc *RawEncoding) RGBA(*Rectangle) ([]byte, error) {
	return getData(enc.rgba)
}

func (enc *RawEncoding) PNG(rect *Rectangle) ([]byte, error) {
	return rgbaToPNG(enc.rgba, int(rect.Width), int(rect.Height))
}

type CopyRectEncoding struct {
	SX, SY uint16
}

func (*CopyRectEncoding) Type() EncodingType {
	return CopyRectEncType
}

func (*CopyRectEncoding) Read(c *ClientConn, rect *Rectangle) (Encoding, error) {
	enc := new(CopyRectEncoding)
	if err := readFixedSize(c.r, enc); err != nil {
		return nil, err
	}
	return enc, nil
}

type DesktopSizePseudoEncoding struct{}

func (*DesktopSizePseudoEncoding) Type() EncodingType {
	return DesktopSizePseudoEncType
}

func (*DesktopSizePseudoEncoding) Read(*ClientConn, *Rectangle) (Encoding, error) {
	return new(DesktopSizePseudoEncoding), nil
}

type CursorPseudoEncoding struct {
	rgba []byte
}

func (*CursorPseudoEncoding) Type() EncodingType {
	return CursorPseudoEncType
}

func (*CursorPseudoEncoding) Read(c *ClientConn, rect *Rectangle) (Encoding, error) {
	var err error
	var rgbaBuffer []byte
	if rgbaBuffer, err = c.pixelFormat.ReadPixels(c.r, int(rect.Height)*int(rect.Width)); err != nil {
		return nil, err
	}
	enc := new(CursorPseudoEncoding)
	enc.rgba = rgbaBuffer

	mask := make([]byte, (rect.Width+7)/8*rect.Height)
	if _, err := io.ReadFull(c.r, mask); err != nil {
		return nil, err
	}

	// set masked pixels to black (not just alpha because we're using pre-multiplied RGBA)
	rectStride := 4 * rect.Width
	for i := uint16(0); i < rect.Height; i++ {
		for j := uint16(0); j < rect.Width; j += 8 {
			for idx, k := j/8, 7; k >= 0; k-- {
				if (mask[idx] & (1 << uint(k))) == 0 {
					pIdx := j*4 + i*rectStride
					rgbaBuffer[pIdx] = 0
					rgbaBuffer[pIdx+1] = 0
					rgbaBuffer[pIdx+2] = 0
					rgbaBuffer[pIdx+3] = 0
				}
			}
		}
	}

	return enc, nil
}

func (enc *CursorPseudoEncoding) RGBA(*Rectangle) ([]byte, error) {
	return getData(enc.rgba)
}

func (enc *CursorPseudoEncoding) PNG(rect *Rectangle) ([]byte, error) {
	return rgbaToPNG(enc.rgba, int(rect.Width), int(rect.Height))
}

type HextileEncoding struct {
	png []byte
}

func (*HextileEncoding) Type() EncodingType {
	return HextileEncType
}

func (enc *HextileEncoding) Read(c *ClientConn, rect *Rectangle) (Encoding, error) {
	var err error
	width := int(rect.Width)
	height := int(rect.Height)
	pf := c.pixelFormat
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	bg := image.NewUniform(color.Black)
	fg := image.NewUniform(color.Black)

	tw := 16
	th := 16
	twLast := width % 16
	thLast := height % 16
	txLast := width - twLast
	tyLast := height - thLast
	pixelBuffer := make([]byte, pf.ByPP)
	subrectBox := make([]byte, 2)
	for ty := 0; ty < height; ty += 16 {
		if ty == tyLast {
			th = thLast
		}
		tw = 16

		for tx := 0; tx < width; tx += 16 {
			if tx == txLast {
				tw = twLast
			}

			// read subencoding mask
			var subencoding uint8
			if err = readFixedSize(c.r, &subencoding); err != nil {
				return nil, err
			}

			dstRect := image.Rect(tx, ty, tx+tw, ty+th)

			// raw
			if subencoding&1 != 0 {
				var rgbaBuffer []byte
				if rgbaBuffer, err = pf.ReadPixels(c.r, tw*th); err != nil {
					return nil, err
				}
				draw.Draw(img, dstRect, newRGBAImage(rgbaBuffer, tw, th), image.ZP, draw.Src)
				continue
			}

			// background/foreground specified
			if subencoding&2 != 0 {
				if bg, err = enc.readPixelToUniform(c.r, pf, pixelBuffer); err != nil {
					return nil, err
				}
			}
			if subencoding&4 != 0 {
				if fg, err = enc.readPixelToUniform(c.r, pf, pixelBuffer); err != nil {
					return nil, err
				}
			}

			// draw background first
			draw.Draw(img, dstRect, bg, image.ZP, draw.Src)

			// done if no subrects
			if subencoding&8 == 0 {
				continue
			}

			// colored?
			var subrectColored bool = false
			if subencoding&16 != 0 {
				subrectColored = true
			}

			var numSubRect uint8
			if err = readFixedSize(c.r, &numSubRect); err != nil {
				return nil, err
			}

			// draw subrects
			for i := uint8(0); i < numSubRect; i++ {
				uImg := fg
				if subrectColored {
					if uImg, err = enc.readPixelToUniform(c.r, pf, pixelBuffer); err != nil {
						return nil, err
					}
				}

				if _, err := io.ReadFull(c.r, subrectBox); err != nil {
					return nil, err
				}
				sy := ty + int(subrectBox[0]&0xF)
				sx := tx + int(subrectBox[0]>>4)
				sh := int(subrectBox[1]&0xF) + 1
				sw := int(subrectBox[1]>>4) + 1
				dstRect = image.Rect(sx, sy, sx+sw, sy+sh)
				draw.Draw(img, dstRect, uImg, image.ZP, draw.Src)
			}
		}
	}

	hEnc := new(HextileEncoding)
	if hEnc.png, err = pngEncode(img); err != nil {
		return nil, err
	}
	return hEnc, nil
}

func (*HextileEncoding) readPixelToUniform(r io.Reader, pf *PixelFormat, buffer []byte) (*image.Uniform, error) {
	var err error
	if buffer, err = pf.ReadPixels(r, 1); err != nil {
		return nil, err
	}

	return image.NewUniform(color.RGBA{buffer[0], buffer[1], buffer[2], buffer[3]}), nil
}

func (enc *HextileEncoding) PNG(*Rectangle) ([]byte, error) {
	return getData(enc.png)
}

// utils functions

func getData(rgba []byte) ([]byte, error) {
	if rgba == nil {
		return nil, fmt.Errorf("data not available")
	} else {
		return rgba, nil
	}
}

func newRGBAImage(rgba []byte, width int, height int) image.Image {
	img := &image.RGBA{Stride: 4 * width}
	img.Pix = rgba
	img.Rect.Max.X = width
	img.Rect.Max.Y = height
	return img
}

func rgbaToPNG(rgba []byte, width int, height int) ([]byte, error) {
	var err error
	if rgba, err = getData(rgba); err != nil {
		return nil, err
	}

	// rgba buffer should not be modified
	return pngEncode(newRGBAImage(rgba, width, height))
}

func pngEncode(img image.Image) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
