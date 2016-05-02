package vnc

type MessageID uint8

type ServerMessage interface {
	// ID returns the id of the message that is sent down on the wire.
	ID() MessageID

	// Receive reads the content of the message from the reader. At the point
	// this is called, the message type has already been read from the reader.
	// This should return a new ServerMessage having the appropriate type.
	Receive(*ClientConn) (ServerMessage, error)
}

type ClientMessage interface {
	// Send writes the content of the message to the writer, including the message type.
	Send(*ClientConn) error
}
