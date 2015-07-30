package otr3

const tlvHeaderLength = 4

const (
	tlvTypePadding      = 0
	tlvTypeDisconnected = 1
	tlvTypeSMP1         = 2
	tlvTypeSMP2         = 3
	tlvTypeSMP3         = 4
	tlvTypeSMP4         = 5
	tlvTypeSMPAbort     = 6
	//TODO: Question is not done
	tlvTypeSMP1WithQuestion = 7
)

type tlv struct {
	tlvType   uint16
	tlvLength uint16
	tlvValue  []byte
}

func (c tlv) serialize() []byte {
	out := appendShort([]byte{}, c.tlvType)
	out = appendShort(out, c.tlvLength)
	return append(out, c.tlvValue...)
}

func (c *tlv) deserialize(tlvsBytes []byte) error {
	var ok bool
	tlvsBytes, c.tlvType, ok = extractShort(tlvsBytes)
	if !ok {
		return newOtrError("wrong tlv type")
	}
	tlvsBytes, c.tlvLength, ok = extractShort(tlvsBytes)
	if !ok {
		return newOtrError("wrong tlv length")
	}
	if len(tlvsBytes) < int(c.tlvLength) {
		return newOtrError("wrong tlv value")
	}
	c.tlvValue = tlvsBytes[:int(c.tlvLength)]
	return nil
}

func (t tlv) isSMPMessage() bool {
	return t.tlvType >= tlvTypeSMP1 && t.tlvType <= tlvTypeSMP1WithQuestion
}

func (t tlv) smpMessage() (smpMessage, bool) {
	switch t.tlvType {
	case 0x02:
		return toSmpMessage1(t)
	case 0x03:
		return toSmpMessage2(t)
	case 0x04:
		return toSmpMessage3(t)
	case 0x05:
		return toSmpMessage4(t)
	}

	return nil, false
}

func toSmpMessage1(t tlv) (msg smp1Message, ok bool) {
	_, mpis, ok := extractMPIs(t.tlvValue)
	if !ok || len(mpis) < 6 {
		return msg, false
	}
	msg.g2a = mpis[0]
	msg.c2 = mpis[1]
	msg.d2 = mpis[2]
	msg.g3a = mpis[3]
	msg.c3 = mpis[4]
	msg.d3 = mpis[5]
	return msg, true
}

func toSmpMessage2(t tlv) (msg smp2Message, ok bool) {
	_, mpis, ok := extractMPIs(t.tlvValue)
	if !ok || len(mpis) < 11 {
		return msg, false
	}
	msg.g2b = mpis[0]
	msg.c2 = mpis[1]
	msg.d2 = mpis[2]
	msg.g3b = mpis[3]
	msg.c3 = mpis[4]
	msg.d3 = mpis[5]
	msg.pb = mpis[6]
	msg.qb = mpis[7]
	msg.cp = mpis[8]
	msg.d5 = mpis[9]
	msg.d6 = mpis[10]
	return msg, true
}

func toSmpMessage3(t tlv) (msg smp3Message, ok bool) {
	_, mpis, ok := extractMPIs(t.tlvValue)
	if !ok || len(mpis) < 8 {
		return msg, false
	}
	msg.pa = mpis[0]
	msg.qa = mpis[1]
	msg.cp = mpis[2]
	msg.d5 = mpis[3]
	msg.d6 = mpis[4]
	msg.ra = mpis[5]
	msg.cr = mpis[6]
	msg.d7 = mpis[7]
	return msg, true
}

func toSmpMessage4(t tlv) (msg smp4Message, ok bool) {
	_, mpis, ok := extractMPIs(t.tlvValue)
	if !ok || len(mpis) < 3 {
		return msg, false
	}
	msg.rb = mpis[0]
	msg.cr = mpis[1]
	msg.d7 = mpis[2]
	return msg, true
}
