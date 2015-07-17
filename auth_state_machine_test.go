package otr3

import (
	"crypto/sha256"
	"io"
	"testing"
)

func dhMsgType(msg []byte) byte {
	return msg[2]
}

func newAkeContext(v otrVersion, r io.Reader) akeContext {
	return akeContext{
		otrContext: newOtrContext(v, r),
		authState:  authStateNone{},
		policies:   policies{},
	}
}

func fixtureAKE() AKE {
	return fixtureAKEWithVersion(otrV3{})
}

func fixtureAKEV2() AKE {
	return fixtureAKEWithVersion(otrV2{})
}

func fixtureAKEWithVersion(v otrVersion) AKE {
	return AKE{
		akeContext: newAkeContext(v, fixtureRand()),
	}
}

func fixtureDHCommitMsg() []byte {
	ake := fixtureAKE()
	ake.senderInstanceTag = generateIntanceTag()
	msg, _ := ake.dhCommitMessage()
	return msg
}

func fixtureDHKeyMsg(v otrVersion) []byte {
	ake := fixtureAKEWithVersion(v)
	ake.ourKey = alicePrivateKey
	msg, _ := ake.dhKeyMessage()
	return msg
}

func fixtureRevealSigMsg() []byte {
	ake := fixtureAKEWithVersion(otrV3{})
	ake.akeContext = fixtureContextToReceiveDHKey()

	copy(ake.r[:], fixedr)
	ake.gy = fixedgy
	ake.ourKeyID = 1

	msg, _ := ake.revealSigMessage()

	return msg
}

func fixtureContextToReceiveDHKey() akeContext {
	c := newAkeContext(otrV3{}, fixtureRand())
	c.addPolicy(allowV3)
	c.authState = authStateAwaitingDHKey{}

	c.x = fixedx
	c.gx = fixedgx
	c.ourKey = bobPrivateKey

	return c
}

func Test_receiveQueryMessage_SendDHCommitAndTransitToStateAwaitingDHKey(t *testing.T) {
	states := []authState{
		authStateNone{},
		authStateAwaitingDHKey{},
		authStateAwaitingRevealSig{},
		authStateAwaitingSig{},
	}

	queryMsg := []byte("?OTRv3?")

	for _, s := range states {
		c := newAkeContext(nil, fixtureRand())
		c.addPolicy(allowV3)
		state, msg := s.receiveQueryMessage(&c, queryMsg)

		assertEquals(t, state, authStateAwaitingDHKey{})
		assertDeepEquals(t, fixtureDHCommitMsg(), msg)
	}
}

func Test_receiveQueryMessage_StoresXAndGx(t *testing.T) {
	fixture := fixtureAKE()
	fixture.dhCommitMessage()

	msg := []byte("?OTRv3?")
	cxt := newAkeContext(nil, fixtureRand())
	cxt.addPolicy(allowV3)

	cxt.receiveQueryMessage(msg)
	assertDeepEquals(t, cxt.x, fixture.x)
	assertDeepEquals(t, cxt.gx, fixture.gx)
}

func Test_parseOTRQueryMessage(t *testing.T) {
	var exp = map[string][]int{
		"?OTR?":     []int{1},
		"?OTRv2?":   []int{2},
		"?OTRv23?":  []int{2, 3},
		"?OTR?v2":   []int{1, 2},
		"?OTRv248?": []int{2, 4, 8},
		"?OTR?v?":   []int{1},
		"?OTRv?":    []int{},
	}

	for queryMsg, versions := range exp {
		m := []byte(queryMsg)
		assertDeepEquals(t, authStateNone{}.parseOTRQueryMessage(m), versions)
	}
}

func Test_acceptOTRRequest_returnsNilForUnsupportedVersions(t *testing.T) {
	p := policies{}
	msg := []byte("?OTR?")
	v := authStateNone{}.acceptOTRRequest(p, msg)

	assertEquals(t, v, nil)
}

func Test_acceptOTRRequest_acceptsOTRV3IfHasAllowV3Policy(t *testing.T) {
	msg := []byte("?OTRv32?")
	p := policies{}
	p.allowV2()
	p.allowV3()
	v := authStateNone{}.acceptOTRRequest(p, msg)

	assertEquals(t, v, otrV3{})
}

func Test_acceptOTRRequest_acceptsOTRV2IfHasOnlyAllowV2Policy(t *testing.T) {
	msg := []byte("?OTRv32?")
	p := policies{}
	p.allowV2()
	v := authStateNone{}.acceptOTRRequest(p, msg)

	assertEquals(t, v, otrV2{})
}

func Test_receiveDHCommit_TransitionsFromNoneToAwaitingRevealSigAndSendDHKeyMsg(t *testing.T) {
	c := newAkeContext(otrV3{}, fixtureRand())
	nextState, nextMsg := authStateNone{}.receiveDHCommitMessage(&c, fixtureDHCommitMsg())

	assertEquals(t, nextState, authStateAwaitingRevealSig{})
	assertEquals(t, dhMsgType(nextMsg), msgTypeDHKey)
}

func Test_receiveDHCommit_AtAuthStateNoneStoresGyAndY(t *testing.T) {
	c := newAkeContext(otrV3{}, fixtureRand())
	authStateNone{}.receiveDHCommitMessage(&c, fixtureDHCommitMsg())

	assertDeepEquals(t, c.gy, fixedgy)
	assertDeepEquals(t, c.y, fixedy)
}

func Test_receiveDHCommit_ResendPreviousDHKeyMsgFromAwaitingRevealSig(t *testing.T) {
	c := newAkeContext(otrV3{}, fixtureRand())

	authAwaitingRevSig, prevDHKeyMsg := authStateNone{}.receiveDHCommitMessage(&c, fixtureDHCommitMsg())
	assertEquals(t, authAwaitingRevSig, authStateAwaitingRevealSig{})

	nextState, msg := authAwaitingRevSig.receiveDHCommitMessage(&c, fixtureDHCommitMsg())

	assertEquals(t, nextState, authStateAwaitingRevealSig{})
	assertEquals(t, dhMsgType(msg), msgTypeDHKey)
	assertDeepEquals(t, prevDHKeyMsg, msg)
}

func Test_receiveDHCommit_AtAuthAwaitingRevealSigiForgetOldEncryptedGxAndHashedGx(t *testing.T) {
	c := newAkeContext(otrV3{}, fixtureRand())
	//TODO needs to stores encryptedGx and hashedGx when it is generated
	c.encryptedGx = []byte{0x02}         //some encryptedGx
	c.hashedGx = [sha256.Size]byte{0x05} //some hashedGx

	newDHCommitMsg := fixtureDHCommitMsg()
	hashedGxIndex, newEncryptedGx := extractData(newDHCommitMsg, 11)
	_, newHashedGx := extractData(newDHCommitMsg, hashedGxIndex)

	authStateNone{}.receiveDHCommitMessage(&c, fixtureDHCommitMsg())

	authStateAwaitingRevealSig{}.receiveDHCommitMessage(&c, newDHCommitMsg)
	assertDeepEquals(t, c.encryptedGx, newEncryptedGx)
	assertDeepEquals(t, c.hashedGx[:], newHashedGx)
}

func Test_receiveDHCommit_AtAuthAwaitingSigTransitionsToAwaitingRevSigAndSendsNewDHKeyMsg(t *testing.T) {
	c := newAkeContext(otrV3{}, fixtureRand())

	authAwaitingRevSig, msg := authStateAwaitingSig{}.receiveDHCommitMessage(&c, fixtureDHCommitMsg())
	assertEquals(t, authAwaitingRevSig, authStateAwaitingRevealSig{})
	assertEquals(t, dhMsgType(msg), msgTypeDHKey)
}

func Test_receiveDHCommit_AtAwaitingDHKeyIgnoreIncomingMsgAndResendOurDHCommitMsgIfOurHashIsHigher(t *testing.T) {
	ourDHCommitAKE := fixtureAKE()
	ourDHMsg, _ := ourDHCommitAKE.dhCommitMessage()

	//make sure we store the same alues when creating the DH commit
	c := newAkeContext(otrV3{}, fixtureRand())
	c.encryptedGx = ourDHCommitAKE.encryptedGx
	c.gx = ourDHCommitAKE.gx

	// force their hashedGx to be lower than ours
	msg := fixtureDHCommitMsg()
	i, _ := extractData(msg, 11)
	msg[i+4] = 0x00

	state, newMsg := authStateAwaitingDHKey{}.receiveDHCommitMessage(&c, msg)
	assertEquals(t, state, authStateAwaitingRevealSig{})
	assertDeepEquals(t, newMsg, ourDHMsg)
}

func Test_receiveDHCommit_AtAwaitingDHKeyForgetOurGxAndSendDHKeyMsgAndGoToAwaitingRevealSig(t *testing.T) {
	ourDHCommitAKE := fixtureAKE()
	ourDHCommitAKE.dhCommitMessage()

	//make sure we store the same values when creating the DH commit
	c := newAkeContext(otrV3{}, fixtureRand())
	c.gx = ourDHCommitAKE.gx

	// force their hashedGx to be higher than ours
	msg := fixtureDHCommitMsg()
	i, _ := extractData(msg, 11)
	msg[i+4] = 0xFF

	state, newMsg := authStateAwaitingDHKey{}.receiveDHCommitMessage(&c, msg)
	assertEquals(t, state, authStateAwaitingRevealSig{})
	assertDeepEquals(t, dhMsgType(newMsg), msgTypeDHKey)
	assertDeepEquals(t, c.gy, fixedgy)
	assertDeepEquals(t, c.y, fixedy)
}

func Test_receiveDHKey_AtAuthStateNoneOrAuthStateAwaitingRevealSigIgnoreIt(t *testing.T) {
	var nilB []byte
	c := newAkeContext(otrV3{}, fixtureRand())
	dhKeymsg := fixtureDHKeyMsg(otrV3{})

	states := []authState{
		authStateNone{},
		authStateAwaitingRevealSig{},
	}

	for _, s := range states {
		state, msg := s.receiveDHKeyMessage(&c, dhKeymsg)
		assertEquals(t, state, s)
		assertDeepEquals(t, msg, nilB)
	}
}

func Test_receiveDHKey_TransitionsFromAwaitingDHKeyToAwaitingSigAndSendsRevealSig(t *testing.T) {
	ourDHCommitAKE := fixtureAKE()
	ourDHCommitAKE.dhCommitMessage()

	c := fixtureContextToReceiveDHKey()

	state, msg := authStateAwaitingDHKey{}.receiveDHKeyMessage(&c, fixtureDHKeyMsg(otrV3{}))

	//TODO before generate rev si need to extract their gy from DH commit
	assertEquals(t, state, authStateAwaitingSig{})
	assertDeepEquals(t, c.gy, fixedgy)
	assertDeepEquals(t, dhMsgType(msg), msgTypeRevealSig)
}

func Test_receiveRevealSig_TransitionsFromAwaitingRevealSigToNoneOnSuccess(t *testing.T) {
	revealSignMsg := fixtureRevealSigMsg()
	_, r := extractData(revealSignMsg, 11)

	c := newAkeContext(otrV3{}, fixtureRand())
	gxMPI := appendMPI([]byte{}, fixedgx)
	c.hashedGx = sha256Sum(gxMPI)
	c.encryptedGx, _ = encrypt(r[:], fixedgx.Bytes())

	//TODO make sure they be stored by the state machine
	c.gx = fixedgx
	c.gy = fixedgy
	c.y = fixedy
	c.ourKey = bobPrivateKey

	state, msg := authStateAwaitingRevealSig{}.receiveRevealSigMessage(&c, revealSignMsg)

	assertEquals(t, state, authStateNone{})
	assertDeepEquals(t, dhMsgType(msg), msgTypeSig)
}

func Test_receiveRevealSig_IgnoreMessageIfNotInStateAwaitingRevealSig(t *testing.T) {
	var nilB []byte

	states := []authState{
		authStateNone{},
		authStateAwaitingDHKey{},
		authStateAwaitingSig{},
	}

	revealSignMsg := fixtureRevealSigMsg()

	for _, s := range states {
		c := newAkeContext(otrV3{}, fixtureRand())
		state, msg := s.receiveRevealSigMessage(&c, revealSignMsg)

		assertEquals(t, state, s)
		assertDeepEquals(t, msg, nilB)
	}
}

func Test_generateDHCommitMsgInstanceTags(t *testing.T) {
	senderInstanceTag := uint32(0x00000101)

	dhCommitAke := fixtureAKE()
	dhCommitAke.senderInstanceTag = senderInstanceTag
	dhCommitMsg, _ := dhCommitAke.dhCommitMessage()

	ake := fixtureAKE()
	generateCommitMsgInstanceTags(&ake, dhCommitMsg)

	assertEquals(t, ake.receiverInstanceTag, senderInstanceTag)
	assertEquals(t, ake.senderInstanceTag, generateIntanceTag())
}

func Test_receiveMessage_ignoresDHCommitIfItsVersionIsNotInThePolicy(t *testing.T) {
	var nilB []byte
	cV2 := newAkeContext(otrV2{}, fixtureRand())
	cV2.addPolicy(allowV2)

	cV3 := newAkeContext(otrV3{}, fixtureRand())
	cV3.addPolicy(allowV3)

	ake := fixtureAKEV2()
	msgV2, _ := ake.dhCommitMessage()
	msgV3 := fixtureDHCommitMsg()

	toSend := cV2.receiveMessage(msgV3)
	assertEquals(t, cV2.authState, authStateNone{})
	assertDeepEquals(t, toSend, nilB)

	toSend = cV3.receiveMessage(msgV2)
	assertEquals(t, cV3.authState, authStateNone{})
	assertDeepEquals(t, toSend, nilB)
}

func Test_receiveMessage_ignoresDHKeyIfItsVersionIsNotInThePolicy(t *testing.T) {
	var nilB []byte
	cV2 := newAkeContext(otrV2{}, fixtureRand())
	cV2.authState = authStateAwaitingDHKey{}
	cV2.addPolicy(allowV2)

	cV3 := newAkeContext(otrV3{}, fixtureRand())
	cV3.authState = authStateAwaitingDHKey{}
	cV3.addPolicy(allowV3)

	msgV2 := fixtureDHKeyMsg(otrV2{})
	msgV3 := fixtureDHKeyMsg(otrV3{})

	toSend := cV2.receiveMessage(msgV3)
	assertEquals(t, cV2.authState, authStateAwaitingDHKey{})
	assertDeepEquals(t, toSend, nilB)

	toSend = cV3.receiveMessage(msgV2)
	assertEquals(t, cV3.authState, authStateAwaitingDHKey{})
	assertDeepEquals(t, toSend, nilB)
}

func Test_receiveMessage_ignoresRevealSignaureIfDoesNotAllowV2(t *testing.T) {
	var nilB []byte
	cV3 := newAkeContext(otrV3{}, fixtureRand())
	cV3.authState = authStateAwaitingRevealSig{}
	cV3.addPolicy(allowV3)

	msg := fixtureRevealSigMsg()

	toSend := cV3.receiveMessage(msg)
	assertEquals(t, cV3.authState, authStateAwaitingRevealSig{})
	assertDeepEquals(t, toSend, nilB)
}
