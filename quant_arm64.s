#include "textflag.h"

// func elossyQuantizeAcAsm(coeffs, out *[16]int16, bias, multiplier, negShift uint32)
//
// Quantizes all 16 coefficients with the AC step: level = ((|c| + bias) *
// multiplier) >> shift, clamped to 2047, with the sign reapplied. The divide by
// the quantizer step is a magic multiply (see quant_reciprocal.go), since NEON
// has no integer divide. negShift is -shift, so USHL's per-lane signed amount
// shifts right. The caller overwrites lane 0, which uses the DC step.
// WORD-encoded ops (sshr/mul/ushl/umin/xtn) were produced by clang and verified
// with objdump; the rest are native.
TEXT ·elossyQuantizeAcAsm(SB), NOSPLIT, $0-28
	MOVD coeffs+0(FP), R0
	MOVD out+8(FP), R1
	MOVWU bias+16(FP), R2
	MOVWU multiplier+20(FP), R3
	MOVWU negShift+24(FP), R4

	VDUP R2, V27.S4
	VDUP R3, V28.S4
	VDUP R4, V29.S4
	MOVD $2047, R5
	VDUP R5, V30.S4

	VLD1 (R0), [V0.H8, V1.H8]

	WORD $0x4f110402 // sshr v2.8h, v0.8h, #15   sign mask
	WORD $0x4f110423 // sshr v3.8h, v1.8h, #15
	VEOR V2.B16, V0.B16, V0.B16
	VSUB V2.H8, V0.H8, V0.H8 // |c|, lanes 0..7
	VEOR V3.B16, V1.B16, V1.B16
	VSUB V3.H8, V1.H8, V1.H8 // |c|, lanes 8..15

	VUXTL V0.H4, V4.S4
	VUXTL2 V0.H8, V5.S4
	VUXTL V1.H4, V6.S4
	VUXTL2 V1.H8, V7.S4

	VADD V27.S4, V4.S4, V4.S4
	VADD V27.S4, V5.S4, V5.S4
	VADD V27.S4, V6.S4, V6.S4
	VADD V27.S4, V7.S4, V7.S4

	WORD $0x4ebc9c84 // mul v4.4s, v4.4s, v28.4s
	WORD $0x4ebc9ca5 // mul v5.4s, v5.4s, v28.4s
	WORD $0x4ebc9cc6 // mul v6.4s, v6.4s, v28.4s
	WORD $0x4ebc9ce7 // mul v7.4s, v7.4s, v28.4s

	WORD $0x6ebd4484 // ushl v4.4s, v4.4s, v29.4s
	WORD $0x6ebd44a5 // ushl v5.4s, v5.4s, v29.4s
	WORD $0x6ebd44c6 // ushl v6.4s, v6.4s, v29.4s
	WORD $0x6ebd44e7 // ushl v7.4s, v7.4s, v29.4s

	WORD $0x6ebe6c84 // umin v4.4s, v4.4s, v30.4s
	WORD $0x6ebe6ca5 // umin v5.4s, v5.4s, v30.4s
	WORD $0x6ebe6cc6 // umin v6.4s, v6.4s, v30.4s
	WORD $0x6ebe6ce7 // umin v7.4s, v7.4s, v30.4s

	WORD $0x0e612884 // xtn v4.4h, v4.4s
	WORD $0x4e6128a4 // xtn2 v4.8h, v5.4s
	WORD $0x0e6128c5 // xtn v5.4h, v6.4s
	WORD $0x4e6128e5 // xtn2 v5.8h, v7.4s

	VEOR V2.B16, V4.B16, V4.B16
	VSUB V2.H8, V4.H8, V4.H8
	VEOR V3.B16, V5.B16, V5.B16
	VSUB V3.H8, V5.H8, V5.H8

	VST1 [V4.H8, V5.H8], (R1)
	RET
