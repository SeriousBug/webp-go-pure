package webp

// DecoderErrorKind classifies a decode/parse failure.
type DecoderErrorKind int

const (
	// DecErrInvalidParam: a caller-provided buffer size or dimension is invalid.
	DecErrInvalidParam DecoderErrorKind = iota
	// DecErrNotEnoughData: input ended before a required structure was available.
	DecErrNotEnoughData
	// DecErrBitstream: the bitstream violates the WebP container or codec format.
	DecErrBitstream
	// DecErrUnsupported: the input uses a feature that is intentionally unimplemented.
	DecErrUnsupported
)

// DecoderError is returned by decoding and parsing entry points.
type DecoderError struct {
	Kind DecoderErrorKind
	Msg  string
}

func (e *DecoderError) Error() string {
	switch e.Kind {
	case DecErrInvalidParam:
		return "invalid parameter: " + e.Msg
	case DecErrNotEnoughData:
		return "not enough data: " + e.Msg
	case DecErrBitstream:
		return "bitstream error: " + e.Msg
	case DecErrUnsupported:
		return "unsupported feature: " + e.Msg
	default:
		return e.Msg
	}
}

func invalidParam(msg string) error   { return &DecoderError{DecErrInvalidParam, msg} }
func notEnoughData(msg string) error  { return &DecoderError{DecErrNotEnoughData, msg} }
func bitstreamErr(msg string) error   { return &DecoderError{DecErrBitstream, msg} }
func unsupportedErr(msg string) error { return &DecoderError{DecErrUnsupported, msg} }

// EncoderErrorKind classifies an encode failure.
type EncoderErrorKind int

const (
	// EncErrInvalidParam: a caller-provided dimension or buffer length is invalid.
	EncErrInvalidParam EncoderErrorKind = iota
	// EncErrBitstream: internal encoder state would produce an invalid bitstream.
	EncErrBitstream
)

// EncoderError is returned by encoding entry points.
type EncoderError struct {
	Kind EncoderErrorKind
	Msg  string
}

func (e *EncoderError) Error() string {
	switch e.Kind {
	case EncErrInvalidParam:
		return "invalid parameter: " + e.Msg
	case EncErrBitstream:
		return "bitstream error: " + e.Msg
	default:
		return e.Msg
	}
}

func encInvalidParam(msg string) error { return &EncoderError{EncErrInvalidParam, msg} }
func encBitstream(msg string) error    { return &EncoderError{EncErrBitstream, msg} }
