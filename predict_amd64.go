//go:build amd64

package webp

import "unsafe"

// elossyTmPredictAsm fills n rows of n bytes with the TrueMotion prediction.
// It handles n == 16 and n == 8 only; the caller guarantees this.
//
//go:noescape
func elossyTmPredictAsm(topPtr, leftPtr, outPtr unsafe.Pointer, topLeft, n int)

func elossyTmPredict16(top, left *[16]uint8, topLeft uint8, out *[256]uint8) {
	elossyTmPredictAsm(unsafe.Pointer(top), unsafe.Pointer(left), unsafe.Pointer(out), int(topLeft), 16)
}

func elossyTmPredict8(top, left *[8]uint8, topLeft uint8, out *[64]uint8) {
	elossyTmPredictAsm(unsafe.Pointer(top), unsafe.Pointer(left), unsafe.Pointer(out), int(topLeft), 8)
}
