package simplejson

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"nuha.dev/gpstracker/internal/gpsv2/conn"
)

var errBadFrame = errors.New("Bad frame")

func ReadMessage(c *conn.Conn, msg *FrameMessage) error {
	return readMessage(c, msg)
}

func readMessage(c *conn.Conn, msg *FrameMessage) error {
	var length int //length field

	if len(msg.Buffer) < 5 {
		return fmt.Errorf("buffer too small")
	}

	_, err := io.ReadFull(c, msg.Buffer[:4])
	if err != nil {
		return err
	}
	//check startbit type
	if msg.Buffer[0] == 0x99 {
		length = int(binary.LittleEndian.Uint16(msg.Buffer[2:4]))
		msg.Protocol = msg.Buffer[1]
		msg.Length = length + 5

	} else {
		return errBadFrame
	}

	if len(msg.Buffer) < msg.Length {
		return fmt.Errorf("buffer too small")
	}

	_, err = io.ReadFull(c, msg.Buffer[4:msg.Length])
	if err != nil {
		return err
	}
	if msg.Buffer[msg.Length-1] != '\n' {
		return errBadFrame
	}

	//frame length is `length` + 5
	//payload length is `length` - 5

	msg.Payload = msg.Buffer[4 : msg.Length-1]
	return nil
}
