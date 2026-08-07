package webp

import "errors"

// Sentinel errors for use with errors.Is. Every DecoderError and EncoderError
// unwraps to the sentinels matching its Kind.
var (
	ErrInvalidParam  = errors.New("webp: invalid parameter")
	ErrNotEnoughData = errors.New("webp: not enough data")
	ErrBitstream     = errors.New("webp: bitstream error")
	ErrUnsupported   = errors.New("webp: unsupported feature")
	// ErrAnimated reports that a still-image entry point was given animated
	// input. Use DecodeAnimation instead.
	ErrAnimated = errors.New("webp: animated input requires the animation decoder")
	// ErrLossyAlpha reports that the lossy encoder was given input with alpha,
	// which it does not support.
	ErrLossyAlpha = errors.New("webp: lossy encoder does not support alpha")
)

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
	// DecErrAnimated: a still-image entry point was given animated input.
	DecErrAnimated
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
	case DecErrAnimated:
		return "animated input: " + e.Msg
	default:
		return e.Msg
	}
}

func (e *DecoderError) Unwrap() []error {
	switch e.Kind {
	case DecErrInvalidParam:
		return []error{ErrInvalidParam}
	case DecErrNotEnoughData:
		return []error{ErrNotEnoughData}
	case DecErrBitstream:
		return []error{ErrBitstream}
	case DecErrUnsupported:
		return []error{ErrUnsupported}
	case DecErrAnimated:
		return []error{ErrUnsupported, ErrAnimated}
	default:
		return nil
	}
}

func invalidParam(msg string) error   { return &DecoderError{DecErrInvalidParam, msg} }
func notEnoughData(msg string) error  { return &DecoderError{DecErrNotEnoughData, msg} }
func bitstreamErr(msg string) error   { return &DecoderError{DecErrBitstream, msg} }
func unsupportedErr(msg string) error { return &DecoderError{DecErrUnsupported, msg} }
func animatedErr(msg string) error    { return &DecoderError{DecErrAnimated, msg} }

// EncoderErrorKind classifies an encode failure.
type EncoderErrorKind int

const (
	// EncErrInvalidParam: a caller-provided dimension or buffer length is invalid.
	EncErrInvalidParam EncoderErrorKind = iota
	// EncErrBitstream: internal encoder state would produce an invalid bitstream.
	EncErrBitstream
	// EncErrAlphaUnsupported: the lossy encoder was given input with alpha.
	EncErrAlphaUnsupported
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
	case EncErrAlphaUnsupported:
		return "alpha unsupported: " + e.Msg
	default:
		return e.Msg
	}
}

func (e *EncoderError) Unwrap() []error {
	switch e.Kind {
	case EncErrInvalidParam:
		return []error{ErrInvalidParam}
	case EncErrBitstream:
		return []error{ErrBitstream}
	case EncErrAlphaUnsupported:
		return []error{ErrInvalidParam, ErrLossyAlpha}
	default:
		return nil
	}
}

func encInvalidParam(msg string) error { return &EncoderError{EncErrInvalidParam, msg} }
func encBitstream(msg string) error    { return &EncoderError{EncErrBitstream, msg} }
func encAlphaUnsupported(msg string) error {
	return &EncoderError{EncErrAlphaUnsupported, msg}
}
