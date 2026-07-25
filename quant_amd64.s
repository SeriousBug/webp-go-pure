#include "textflag.h"

// func elossyQuantizeAcAsm(coeffs, out *[16]int16, bias, multiplier, shift uint32)
//
// Quantizes all 16 coefficients with the AC step: level = ((|c| + bias) *
// multiplier) >> shift, clamped to 2047, with the sign reapplied. The divide by
// the quantizer step is a magic multiply (see quant_reciprocal.go), since SSE2
// has no integer divide. The caller overwrites lane 0, which uses the DC step.
//
// SSE2 has no 32-bit lane multiply, so each pair of lanes goes through PMULULQ:
// even lanes directly, odd lanes shuffled down. The 64-bit products are shifted
// in place and their low halves reinterleaved, which is exact because the
// multiplier is chosen so the product fits in 32 bits. Only SSE2 is used.
TEXT ·elossyQuantizeAcAsm(SB), NOSPLIT, $0-28
	MOVQ coeffs+0(FP), SI
	MOVQ out+8(FP), DI

	MOVL bias+16(FP), AX
	MOVQ AX, X10
	PSHUFD $0x00, X10, X10 // bias in each 32-bit lane
	MOVL multiplier+20(FP), AX
	MOVQ AX, X11
	PSHUFD $0x00, X11, X11
	MOVL shift+24(FP), AX
	MOVQ AX, X12 // shift count for PSRLQ
	MOVL $0x07FF07FF, AX
	MOVQ AX, X13
	PSHUFD $0x00, X13, X13 // 2047 in each 16-bit lane
	PXOR X7, X7

	MOVQ $0, CX

loop:
	MOVOU (SI)(CX*1), X0

	MOVOU X0, X1
	PSRAW $15, X1 // sign mask
	PXOR  X1, X0
	PSUBW X1, X0 // |c|

	MOVOU     X0, X2
	PUNPCKLWL X7, X0 // lanes 0..3 as uint32
	PUNPCKHWL X7, X2 // lanes 4..7
	PADDL     X10, X0
	PADDL     X10, X2

	MOVOU  X0, X3
	PSHUFD $0xF5, X3, X3 // odd lanes moved to even positions
	PMULULQ X11, X0      // products of lanes 0, 2
	PMULULQ X11, X3      // products of lanes 1, 3
	PSRLQ  X12, X0
	PSRLQ  X12, X3
	PSHUFD $0x08, X0, X0 // low halves of both products, packed low
	PSHUFD $0x08, X3, X3
	PUNPCKLLQ X3, X0     // back to lane order 0,1,2,3

	MOVOU  X2, X4
	PSHUFD $0xF5, X4, X4
	PMULULQ X11, X2
	PMULULQ X11, X4
	PSRLQ  X12, X2
	PSRLQ  X12, X4
	PSHUFD $0x08, X2, X2
	PSHUFD $0x08, X4, X4
	PUNPCKLLQ X4, X2

	PACKSSLW X2, X0 // eight levels, saturated well above the 2047 clamp
	PMINSW   X13, X0
	PXOR     X1, X0
	PSUBW    X1, X0
	MOVOU    X0, (DI)(CX*1)

	ADDQ $16, CX
	CMPQ CX, $32
	JLT  loop
	RET
