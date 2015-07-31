package otr3

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
)

type Conversation struct {
	version otrVersion
	Rand    io.Reader

	msgState msgState

	ourInstanceTag   uint32
	theirInstanceTag uint32

	ourKey   *PrivateKey
	theirKey *PublicKey

	keys keyManagementContext

	ssid         [8]byte
	policies     policies
	ake          *ake
	smp          smp
	fragmentSize uint16
}

type msgState int

const (
	plainText msgState = iota
	encrypted
	finished
)

var (
	queryMarker = []byte("?OTR")
	msgMarker   = []byte("?OTR:")
)

func (c *Conversation) Send(msg []byte) ([][]byte, error) {
	if !c.policies.isOTREnabled() {
		return [][]byte{msg}, nil
	}
	switch c.msgState {
	case plainText:
		if c.policies.has(requireEncryption) {
			return [][]byte{c.queryMessage()}, nil
		}
		if c.policies.has(sendWhitespaceTag) {
			msg = c.appendWhitespaceTag(msg)
		}
		return [][]byte{msg}, nil
	case encrypted:
		return c.encode(c.genDataMsg(msg).serialize(c)), nil
	case finished:
		return nil, errors.New("otr: cannot send message because secure conversation has finished")
	}

	return nil, errors.New("otr: cannot send message in current state")
}

func parseOTRMessage(msg []byte) ([]byte, bool) {
	if bytes.HasPrefix(msg, msgMarker) && msg[len(msg)-1] == '.' {
		return msg[len(msgMarker) : len(msg)-1], true
	}

	return msg, false
}

func (c *Conversation) decode(encoded []byte) ([]byte, error) {
	msg := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	msgLen, err := base64.StdEncoding.Decode(msg, encoded)

	if err != nil {
		return nil, errInvalidOTRMessage
	}

	return msg[:msgLen], nil
}

// This should be used by the xmpp-client to received OTR messages in plain
//TODO For the exported Receive, toSend needs fragmentation
func (c *Conversation) Receive(message []byte) (plain []byte, toSend [][]byte, err error) {
	if !c.policies.isOTREnabled() {
		plain = message
		return
	}

	//TODO: warn the user for REQUIRE_ENCRYPTION
	//See: Receiving plaintext with/without the whitespace tag

	var unencodedMsg []byte
	if m, ok := parseOTRMessage(message); ok {
		message, err = c.decode(m)
		if err != nil {
			return
		}
	} else {
		//queryMSG or plain
	}

	plain, unencodedMsg, err = c.receive(message)
	if err != nil {
		return
	}

	toSend = c.encode(unencodedMsg)

	return
}

func (c *Conversation) receive(message []byte) (plain, toSend []byte, err error) {
	if isQueryMessage(message) {
		toSend, err = c.receiveQueryMessage(message)
		return
	}

	//FIXME: Where should this be? Before of after the base64 decoding?
	message, toSend, err = c.processWhitespaceTag(message)
	if err != nil || toSend != nil {
		return
	}

	// TODO check the message instanceTag for V3
	// I should ignore the message if it is not for my Conversation

	_, msgProtocolVersion, ok := extractShort(message)
	if !ok {
		err = errInvalidOTRMessage
		return
	}

	msgType := message[2]
	if msgType != msgTypeDHCommit && c.version.protocolVersion() != msgProtocolVersion {
		err = errWrongProtocolVersion
		return
	}

	switch msgType {
	case msgTypeData:
		if c.msgState != encrypted {
			toSend = c.restart()
			err = errEncryptedMessageWithNoSecureChannel
			return
		}

		plain, toSend, err = c.processDataMessage(message)
		if err != nil {
			return
		}

	default:
		toSend, err = c.receiveAKE(message)
	}

	return
}

func (c *Conversation) messageHeader(msgType byte) []byte {
	return c.version.messageHeader(c, msgType)
}

func (c *Conversation) parseMessageHeader(msg []byte) ([]byte, error) {
	return c.version.parseMessageHeader(c, msg)
}

func (c *Conversation) IsEncrypted() bool {
	return c.msgState == encrypted
}

func (c *Conversation) End() (toSend [][]byte, ok bool) {
	ok = true
	switch c.msgState {
	case plainText:
	case encrypted:
		c.msgState = plainText
		toSend = c.encode(c.genDataMsg(nil, tlv{tlvType: tlvTypeDisconnected}).serialize(c))
	case finished:
		c.msgState = plainText
	}
	ok = false
	return
}

func (c *Conversation) encode(msg []byte) [][]byte {
	msgPrefix := []byte("?OTR:")
	b64 := make([]byte, base64.StdEncoding.EncodedLen(len(msg))+len(msgPrefix)+1)
	base64.StdEncoding.Encode(b64[len(msgPrefix):], msg)
	copy(b64, msgPrefix)
	b64[len(b64)-1] = '.'

	bytesPerFragment := c.fragmentSize - c.version.minFragmentSize()
	return c.fragment(b64, bytesPerFragment, uint32(0), uint32(0))
}

/*TODO: Authenticate
func (c *Conversation) Authenticate(question string, mutualSecret []byte) (toSend [][]byte, err error) {
	return [][]byte{}, nil
}
*/
/*TODO: SMPQuestion
func (c *Conversation) SMPQuestion() string {
	return c.smp.question
}
*/
