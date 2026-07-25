#include "textflag.h"

// func elossyTmPredictAsm(topPtr, leftPtr, outPtr unsafe.Pointer, topLeft, n int)
//
// TrueMotion fill: out[row][col] = clip(left[row] + top[col] - topLeft).
//
// The Go arm64 assembler exposes no saturating vector arithmetic, so the two
// halves of the clip are built from VUMIN plus a plain add/subtract:
//
//	uqadd(a, b) == a + min(b, 255-a)
//	uqsub(a, b) == a - min(a, b)
//
// Splitting top-topLeft into its positive and negative parts (only one of which
// is ever non-zero for a given column) turns the clip into an unsigned
// saturating add followed by an unsigned saturating subtract, all in 8-bit
// lanes, so a 16-wide row is one vector.
TEXT ·elossyTmPredictAsm(SB), NOSPLIT, $0-40
	MOVD topPtr+0(FP), R0
	MOVD leftPtr+8(FP), R1
	MOVD outPtr+16(FP), R2
	MOVD topLeft+24(FP), R3
	MOVD n+32(FP), R4

	CMP $16, R4
	BEQ setup16

	VLD1 (R0), [V0.B8]
	VDUP R3, V1.B8
	VUMIN V1.B8, V0.B8, V2.B8 // min(top, topLeft)
	VSUB  V2.B8, V0.B8, V3.B8 // dpos = max(top-topLeft, 0)
	VSUB  V2.B8, V1.B8, V4.B8 // dneg = max(topLeft-top, 0)

loop8:
	MOVBU (R1), R5
	ADD   $1, R1, R1
	EOR   $255, R5, R6        // 255 - left[row]
	VDUP  R5, V5.B8
	VDUP  R6, V6.B8
	VUMIN V6.B8, V3.B8, V7.B8 // min(255-left, dpos)
	VADD  V5.B8, V7.B8, V7.B8 // uqadd(left, dpos)
	VUMIN V7.B8, V4.B8, V8.B8 // min(that, dneg)
	VSUB  V8.B8, V7.B8, V7.B8 // uqsub(that, dneg)
	VST1  [V7.B8], (R2)
	ADD   $8, R2, R2
	SUBS  $1, R4, R4
	BNE   loop8
	RET

setup16:
	VLD1 (R0), [V0.B16]
	VDUP R3, V1.B16
	VUMIN V1.B16, V0.B16, V2.B16
	VSUB  V2.B16, V0.B16, V3.B16
	VSUB  V2.B16, V1.B16, V4.B16

loop16:
	MOVBU (R1), R5
	ADD   $1, R1, R1
	EOR   $255, R5, R6
	VDUP  R5, V5.B16
	VDUP  R6, V6.B16
	VUMIN V6.B16, V3.B16, V7.B16
	VADD  V5.B16, V7.B16, V7.B16
	VUMIN V7.B16, V4.B16, V8.B16
	VSUB  V8.B16, V7.B16, V7.B16
	VST1  [V7.B16], (R2)
	ADD   $16, R2, R2
	SUBS  $1, R4, R4
	BNE   loop16
	RET
