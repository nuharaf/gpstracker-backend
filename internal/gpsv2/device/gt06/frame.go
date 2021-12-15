package gt06

import (
	"encoding/binary"
	"fmt"
	"io"

	"nuha.dev/gpstracker/internal/gpsv2/conn"
)

func ReadMessage(c *conn.Conn, msg *Message) error {
	return readMessage(c, msg)
}

func SendLoginOK(c *conn.Conn, serial int) error {
	_, err := c.Write(newFrame(loginMessage, []byte{}, serial))
	return err
}

func readMessage(c *conn.Conn, msg *Message) error {
	var length int       //length field
	var var_buf []byte   //start of variable length data
	var frame_length int //from the beginning of gt06.buffer (including trailer 0x0d 0x0a)

	if len(msg.Buffer) < 4 {
		return fmt.Errorf("buffer too small")
	}

	_, err := io.ReadFull(c, msg.Buffer[:4])
	if err != nil {
		return err
	}
	//check startbit type
	if msg.Buffer[0] == 0x78 {
		length = int(msg.Buffer[2])
		var_buf = msg.Buffer[3:]
		frame_length = length + 5
		msg.Length = frame_length
	} else if msg.Buffer[1] == 0x79 {
		length = int(binary.BigEndian.Uint16(msg.Buffer[2:4]))
		var_buf = msg.Buffer[4:]
		frame_length = length + 6
		msg.Length = frame_length
		msg.Extended = true
	} else {
		return errBadFrame
	}

	if len(msg.Buffer) < frame_length {
		return fmt.Errorf("buffer too small")
	}

	_, err = io.ReadFull(c, msg.Buffer[4:frame_length])
	if err != nil {
		return err
	}

	if msg.Buffer[frame_length-2] != 0x0D || msg.Buffer[frame_length-1] != 0x0A {
		return errBadFrame
	}

	//frame length is `length` + 5
	//payload length is `length` - 5

	msg.Protocol = var_buf[0]
	msg.Payload = var_buf[1 : length-4]
	msg.Serial = int(binary.BigEndian.Uint16(var_buf[length-4 : length-2]))
	return nil
}
