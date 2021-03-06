package quickfix

import (
	"bytes"
	"fmt"
	"math"
	"time"

	"github.com/quickfixgo/quickfix/enum"
)

//Header is first section of a FIX Message
type Header struct{ FieldMap }

//in the message header, the first 3 tags in the message header must be 8,9,35
func headerFieldOrdering(i, j Tag) bool {
	var ordering = func(t Tag) uint32 {
		switch t {
		case tagBeginString:
			return 1
		case tagBodyLength:
			return 2
		case tagMsgType:
			return 3
		}

		return math.MaxUint32
	}

	orderi := ordering(i)
	orderj := ordering(j)

	switch {
	case orderi < orderj:
		return true
	case orderi > orderj:
		return false
	}

	return i < j
}

//Init initializes the Header instance
func (h *Header) Init() {
	h.initWithOrdering(headerFieldOrdering)
}

//Body is the primary application section of a FIX message
type Body struct{ FieldMap }

//Init initializes the FIX message
func (b *Body) Init() {
	b.init()
}

//Trailer is the last section of a FIX message
type Trailer struct{ FieldMap }

// In the trailer, CheckSum (tag 10) must be last
func trailerFieldOrdering(i, j Tag) bool {
	switch {
	case i == tagCheckSum:
		return false
	case j == tagCheckSum:
		return true
	}

	return i < j
}

//Init initializes the FIX message
func (t *Trailer) Init() {
	t.initWithOrdering(trailerFieldOrdering)
}

//Message is a FIX Message abstraction.
type Message struct {
	Header  Header
	Trailer Trailer
	Body    Body

	//ReceiveTime is the time that this message was read from the socket connection
	ReceiveTime time.Time

	rawMessage *bytes.Buffer

	//slice of Bytes corresponding to the message body
	bodyBytes []byte

	//field bytes as they appear in the raw message
	fields TagValues

	//flag is true if this message should not be returned to pool after use
	keepMessage bool
}

//ToMessage returns the message itself
func (m Message) ToMessage() Message { return m }

//parseError is returned when bytes cannot be parsed as a FIX message.
type parseError struct {
	OrigError string
}

func (e parseError) Error() string { return fmt.Sprintf("error parsing message: %s", e.OrigError) }

//NewMessage returns a newly initialized Message instance
func NewMessage() (m Message) {
	m.Header.Init()
	m.Body.Init()
	m.Trailer.Init()

	return
}

//ParseMessage constructs a Message from a byte slice wrapping a FIX message.
func ParseMessage(msg *Message, rawMessage *bytes.Buffer) (err error) {
	msg.Header.Clear()
	msg.Body.Clear()
	msg.Trailer.Clear()
	msg.rawMessage = rawMessage

	rawBytes := rawMessage.Bytes()

	//allocate fields in one chunk
	fieldCount := 0
	for _, b := range rawBytes {
		if b == '\001' {
			fieldCount++
		}
	}

	if cap(msg.fields) < fieldCount {
		msg.fields = make(TagValues, fieldCount)
	} else {
		msg.fields = msg.fields[0:fieldCount]
	}

	fieldIndex := 0

	//message must start with begin string, body length, msg type
	if rawBytes, err = extractSpecificField(&msg.fields[fieldIndex], tagBeginString, rawBytes); err != nil {
		return
	}

	msg.Header.tagLookup[msg.fields[fieldIndex].tag] = msg.fields[fieldIndex : fieldIndex+1]
	fieldIndex++

	parsedFieldBytes := &msg.fields[fieldIndex]
	if rawBytes, err = extractSpecificField(parsedFieldBytes, tagBodyLength, rawBytes); err != nil {
		return
	}

	msg.Header.tagLookup[parsedFieldBytes.tag] = msg.fields[fieldIndex : fieldIndex+1]
	fieldIndex++

	parsedFieldBytes = &msg.fields[fieldIndex]
	if rawBytes, err = extractSpecificField(parsedFieldBytes, tagMsgType, rawBytes); err != nil {
		return
	}

	msg.Header.tagLookup[parsedFieldBytes.tag] = msg.fields[fieldIndex : fieldIndex+1]
	fieldIndex++

	trailerBytes := []byte{}
	foundBody := false
	for {
		parsedFieldBytes = &msg.fields[fieldIndex]
		rawBytes, err = extractField(parsedFieldBytes, rawBytes)
		if err != nil {
			return
		}

		switch {
		case parsedFieldBytes.tag.IsHeader():
			msg.Header.tagLookup[parsedFieldBytes.tag] = msg.fields[fieldIndex : fieldIndex+1]
		case parsedFieldBytes.tag.IsTrailer():
			msg.Trailer.tagLookup[parsedFieldBytes.tag] = msg.fields[fieldIndex : fieldIndex+1]
		default:
			foundBody = true
			trailerBytes = rawBytes
			msg.Body.tagLookup[parsedFieldBytes.tag] = msg.fields[fieldIndex : fieldIndex+1]
		}
		if parsedFieldBytes.tag == tagCheckSum {
			break
		}

		if !foundBody {
			msg.bodyBytes = rawBytes
		}

		fieldIndex++
	}

	//body length would only be larger than trailer if fields out of order
	if len(msg.bodyBytes) > len(trailerBytes) {
		msg.bodyBytes = msg.bodyBytes[:len(msg.bodyBytes)-len(trailerBytes)]
	}

	length := 0
	for _, field := range msg.fields {
		switch field.tag {
		case tagBeginString, tagBodyLength, tagCheckSum: //tags do not contribute to length
		default:
			length += field.length()
		}
	}

	bodyLength, err := msg.Header.GetInt(tagBodyLength)
	if err != nil {
		err = parseError{OrigError: err.Error()}
	} else if length != bodyLength {
		err = parseError{OrigError: fmt.Sprintf("Incorrect Message Length, expected %d, got %d", bodyLength, length)}
	}

	return
}

// MsgType returns MsgType (tag 35) field's value
func (m Message) MsgType() (enum.MsgType, MessageRejectError) {
	s, err := m.Header.GetString(tagMsgType)
	if err != nil {
		return enum.MsgType(""), err
	}
	return enum.MsgType(s), nil
}

// IsMsgTypeOf returns true if the Header contains MsgType (tag 35) field and its value is the specified one.
func (m Message) IsMsgTypeOf(msgType enum.MsgType) bool {
	if v, err := m.MsgType(); err == nil {
		return v == msgType
	}
	return false
}

//reverseRoute returns a message builder with routing header fields initialized as the reverse of this message.
func (m Message) reverseRoute() Message {
	reverseMsg := NewMessage()

	copy := func(src Tag, dest Tag) {
		var field FIXString
		if m.Header.GetField(src, &field) == nil {
			if len(field) != 0 {
				reverseMsg.Header.SetField(dest, field)
			}
		}
	}

	copy(tagSenderCompID, tagTargetCompID)
	copy(tagSenderSubID, tagTargetSubID)
	copy(tagSenderLocationID, tagTargetLocationID)

	copy(tagTargetCompID, tagSenderCompID)
	copy(tagTargetSubID, tagSenderSubID)
	copy(tagTargetLocationID, tagSenderLocationID)

	copy(tagOnBehalfOfCompID, tagDeliverToCompID)
	copy(tagOnBehalfOfSubID, tagDeliverToSubID)
	copy(tagDeliverToCompID, tagOnBehalfOfCompID)
	copy(tagDeliverToSubID, tagOnBehalfOfSubID)

	//tags added in 4.1
	var beginString FIXString
	if m.Header.GetField(tagBeginString, &beginString) == nil {
		if string(beginString) != enum.BeginStringFIX40 {
			copy(tagOnBehalfOfLocationID, tagDeliverToLocationID)
			copy(tagDeliverToLocationID, tagOnBehalfOfLocationID)
		}
	}

	return reverseMsg
}

func extractSpecificField(field *TagValue, expectedTag Tag, buffer []byte) (remBuffer []byte, err error) {
	remBuffer, err = extractField(field, buffer)
	switch {
	case err != nil:
		return
	case field.tag != expectedTag:
		err = parseError{OrigError: fmt.Sprintf("extractSpecificField: Fields out of order, expected %d, got %d", expectedTag, field.tag)}
		return
	}

	return
}

func extractField(parsedFieldBytes *TagValue, buffer []byte) (remBytes []byte, err error) {
	endIndex := bytes.IndexByte(buffer, '\001')
	if endIndex == -1 {
		err = parseError{OrigError: "extractField: No Trailing Delim in " + string(buffer)}
		remBytes = buffer
		return
	}

	err = parsedFieldBytes.parse(buffer[:endIndex+1])
	return buffer[(endIndex + 1):], err
}

func (m Message) String() string {
	if m.rawMessage != nil {
		return m.rawMessage.String()
	}

	return string(m.build())
}

func newCheckSum(value int) FIXString {
	return FIXString(fmt.Sprintf("%03d", value))
}

//Build constructs a []byte from a Message instance
func (m Message) build() []byte {
	m.cook()

	var b bytes.Buffer
	m.Header.write(&b)
	m.Body.write(&b)
	m.Trailer.write(&b)
	return b.Bytes()
}

func (m Message) cook() {
	bodyLength := m.Header.length() + m.Body.length() + m.Trailer.length()
	m.Header.SetField(tagBodyLength, FIXInt(bodyLength))
	checkSum := (m.Header.total() + m.Body.total() + m.Trailer.total()) % 256
	m.Trailer.SetField(tagCheckSum, newCheckSum(checkSum))
}
