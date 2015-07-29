package otr3

import "math/big"

// FIXME should store g3a*, g2, g3, b3, Pb and Qb
// after generating smp2Message

type smp2State struct {
	y                  *big.Int
	b2, b3             *big.Int
	r2, r3, r4, r5, r6 *big.Int
	g2, g3             *big.Int
	g3a                *big.Int
	qb                 *big.Int
	msg                smp2Message
}

type smp2Message struct {
	g2b, g3b *big.Int
	c2, c3   *big.Int
	d2, d3   *big.Int
	pb, qb   *big.Int
	cp       *big.Int
	d5, d6   *big.Int
}

func (m smp2Message) tlv() tlv {
	return genSMPTLV(uint16(tlvTypeSMP2), m.g2b, m.c2, m.d2, m.g3b, m.c3, m.d3, m.pb, m.qb, m.cp, m.d5, m.d6)
}

func (c *Conversation) generateSMP2Parameters() (s smp2State, ok bool) {
	b := make([]byte, c.version.parameterLength())
	var ok1, ok2, ok3, ok4, ok5, ok6, ok7 bool
	s.b2, ok1 = c.randMPI(b)
	s.b3, ok2 = c.randMPI(b)
	s.r2, ok3 = c.randMPI(b)
	s.r3, ok4 = c.randMPI(b)
	s.r4, ok5 = c.randMPI(b)
	s.r5, ok6 = c.randMPI(b)
	s.r6, ok7 = c.randMPI(b)
	return s, ok1 && ok2 && ok3 && ok4 && ok5 && ok6 && ok7
}

func generateSMP2Message(s *smp2State, s1 smp1Message) smp2Message {
	var m smp2Message

	m.g2b = modExp(g1, s.b2)
	m.g3b = modExp(g1, s.b3)

	m.c2, m.d2 = generateZKP(s.r2, s.b2, 3)
	m.c3, m.d3 = generateZKP(s.r3, s.b3, 4)

	s.g3a = s1.g3a
	s.g2 = modExp(s1.g2a, s.b2)
	s.g3 = modExp(s1.g3a, s.b3)

	m.pb = modExp(s.g3, s.r4)
	m.qb = mulMod(modExp(g1, s.r4), modExp(s.g2, s.y), p)

	m.cp = hashMPIsBN(nil, 5,
		modExp(s.g3, s.r5),
		mulMod(modExp(g1, s.r5), modExp(s.g2, s.r6), p))

	m.d5 = subMod(s.r5, mul(s.r4, m.cp), q)
	m.d6 = subMod(s.r6, mul(s.y, m.cp), q)

	return m
}

func (c *Conversation) generateSMP2(secret *big.Int, s1 smp1Message) (s smp2State, ok bool) {
	if s, ok = c.generateSMP2Parameters(); !ok {
		return s, false
	}
	s.y = secret
	s.msg = generateSMP2Message(&s, s1)
	s.qb = s.msg.qb
	return
}

func (c *Conversation) verifySMP2(s1 smp1State, msg smp2Message) error {
	if !c.version.isGroupElement(msg.g2b) {
		return newOtrError("g2b is an invalid group element")
	}

	if !c.version.isGroupElement(msg.g3b) {
		return newOtrError("g3b is an invalid group element")
	}

	if !c.version.isGroupElement(msg.pb) {
		return newOtrError("Pb is an invalid group element")
	}

	if !c.version.isGroupElement(msg.qb) {
		return newOtrError("Qb is an invalid group element")
	}

	if !verifyZKP(msg.d2, msg.g2b, msg.c2, 3) {
		return newOtrError("c2 is not a valid zero knowledge proof")
	}

	if !verifyZKP(msg.d3, msg.g3b, msg.c3, 4) {
		return newOtrError("c3 is not a valid zero knowledge proof")
	}

	g2 := modExp(msg.g2b, s1.a2)
	g3 := modExp(msg.g3b, s1.a3)

	if !verifyZKP2(g2, g3, msg.d5, msg.d6, msg.pb, msg.qb, msg.cp, 5) {
		return newOtrError("cP is not a valid zero knowledge proof")
	}

	return nil
}
