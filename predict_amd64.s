#include "textflag.h"

// func elossyTmPredictAsm(topPtr, leftPtr, outPtr unsafe.Pointer, topLeft, n int)
//
// TrueMotion fill: out[row][col] = clip(left[row] + top[col] - topLeft).
//
// Splitting top-topLeft into its positive and negative parts (only one of which
// is ever non-zero for a given column) turns the clip into an unsigned
// saturating add followed by an unsigned saturating subtract, so a whole 16-wide
// row is PADDUSB plus PSUBUSB. The byte splats use PSHUFB (SSSE3), which
// predscore_amd64.s already relies on the same way.
TEXT ·elossyTmPredictAsm(SB), NOSPLIT, $0-40
	MOVQ topPtr+0(FP), SI
	MOVQ leftPtr+8(FP), DI
	MOVQ outPtr+16(FP), DX
	MOVQ topLeft+24(FP), AX
	MOVQ n+32(FP), CX

	PXOR X3, X3 // PSHUFB index vector: replicate byte 0
	MOVD AX, X1
	PSHUFB X3, X1 // topLeft

	CMPQ CX, $16
	JEQ  wide

	MOVQ (SI), X0 // 8 top bytes
	MOVO X0, X2
	PSUBUSB X1, X2 // dpos = max(top-topLeft, 0)
	PSUBUSB X0, X1 // dneg = max(topLeft-top, 0)

loop8:
	MOVBLZX (DI), BX
	INCQ    DI
	MOVD    BX, X4
	PSHUFB  X3, X4
	PADDUSB X2, X4
	PSUBUSB X1, X4
	MOVQ    X4, (DX)
	ADDQ    $8, DX
	SUBQ    $1, CX
	JNE     loop8
	RET

wide:
	MOVOU (SI), X0 // 16 top bytes
	MOVO  X0, X2
	PSUBUSB X1, X2
	PSUBUSB X0, X1

loop16:
	MOVBLZX (DI), BX
	INCQ    DI
	MOVD    BX, X4
	PSHUFB  X3, X4
	PADDUSB X2, X4
	PSUBUSB X1, X4
	MOVOU   X4, (DX)
	ADDQ    $16, DX
	SUBQ    $1, CX
	JNE     loop16
	RET
