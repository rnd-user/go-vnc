package vnc

import (
	"encoding/binary"
	"io"
)

func readFixedSize(r io.Reader, data interface{}) error {
	return binary.Read(r, binary.BigEndian, data)
}

func writeFixedSize(w io.Writer, data interface{}) error {
	return binary.Write(w, binary.BigEndian, data)
}
