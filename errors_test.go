package webp

import (
	"errors"
	"testing"
)

func TestDecoderErrorUnwrap(t *testing.T) {
	cases := []struct {
		kind     DecoderErrorKind
		sentinel []error
	}{
		{DecErrInvalidParam, []error{ErrInvalidParam}},
		{DecErrNotEnoughData, []error{ErrNotEnoughData}},
		{DecErrBitstream, []error{ErrBitstream}},
		{DecErrUnsupported, []error{ErrUnsupported}},
		{DecErrAnimated, []error{ErrUnsupported, ErrAnimated}},
	}
	all := []error{ErrInvalidParam, ErrNotEnoughData, ErrBitstream, ErrUnsupported, ErrAnimated, ErrLossyAlpha}

	for _, tc := range cases {
		err := error(&DecoderError{tc.kind, "boom"})
		want := map[error]bool{}
		for _, s := range tc.sentinel {
			want[s] = true
			if !errors.Is(err, s) {
				t.Errorf("kind %d: errors.Is(%v) = false, want true", tc.kind, s)
			}
		}
		for _, s := range all {
			if !want[s] && errors.Is(err, s) {
				t.Errorf("kind %d: errors.Is(%v) = true, want false", tc.kind, s)
			}
		}
		if err.Error() == "" {
			t.Errorf("kind %d: empty Error() string", tc.kind)
		}
	}
}

func TestEncoderErrorUnwrap(t *testing.T) {
	cases := []struct {
		kind     EncoderErrorKind
		sentinel []error
	}{
		{EncErrInvalidParam, []error{ErrInvalidParam}},
		{EncErrBitstream, []error{ErrBitstream}},
		{EncErrAlphaUnsupported, []error{ErrInvalidParam, ErrLossyAlpha}},
	}
	all := []error{ErrInvalidParam, ErrNotEnoughData, ErrBitstream, ErrUnsupported, ErrAnimated, ErrLossyAlpha}

	for _, tc := range cases {
		err := error(&EncoderError{tc.kind, "boom"})
		want := map[error]bool{}
		for _, s := range tc.sentinel {
			want[s] = true
			if !errors.Is(err, s) {
				t.Errorf("kind %d: errors.Is(%v) = false, want true", tc.kind, s)
			}
		}
		for _, s := range all {
			if !want[s] && errors.Is(err, s) {
				t.Errorf("kind %d: errors.Is(%v) = true, want false", tc.kind, s)
			}
		}
		if err.Error() == "" {
			t.Errorf("kind %d: empty Error() string", tc.kind)
		}
	}
}

func TestErrorConstructors(t *testing.T) {
	if !errors.Is(animatedErr("x"), ErrAnimated) {
		t.Error("animatedErr does not match ErrAnimated")
	}
	if !errors.Is(encAlphaUnsupported("x"), ErrLossyAlpha) {
		t.Error("encAlphaUnsupported does not match ErrLossyAlpha")
	}
}
