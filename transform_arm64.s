#include "textflag.h"

// func elossyAddTransformAsm(planePtr unsafe.Pointer, stride int, coeffs *[16]int16)
//
// NEON port of the VP8 4x4 inverse transform + add-to-plane (libwebp TransformOne).
// Two butterfly passes with a 4x4 int32 transpose between them; the constant
// multiplies mul1(v)=((v*20091)>>16)+v and mul2(v)=(v*35468)>>16 use full 32-bit
// products, so results match the scalar int32 code bit-for-bit (including the
// wrap-on-overflow that the scalar code also relies on). The clip to [0,255] is a
// saturating narrow (SQXTUN then UQXTN).
//
// Go's assembler exposes no vector integer multiply, widen (SXTL), shift-right
// (SSHR), or saturating narrow (SQXTUN/UQXTN), so those are emitted as raw WORDs.
// Every encoding was produced by clang and verified with objdump; the mnemonic is
// in the trailing comment. VADD/VSUB/VTRN/VZIP/VDUP/VUXTL are native.
TEXT ·elossyAddTransformAsm(SB), NOSPLIT, $0-24
	MOVD planePtr+0(FP), R0
	MOVD stride+8(FP), R1
	MOVD coeffs+16(FP), R2

	MOVD $20091, R3
	VDUP R3, V30.S4 // kC1
	MOVD $35468, R4
	VDUP R4, V31.S4 // kC2
	MOVD $4, R5
	VDUP R5, V29.S4 // rounding bias for pass 2

	VLD1 (R2), [V0.H8, V1.H8] // V0=coeffs[0..7], V1=coeffs[8..15]

	WORD $0x0f10a402 // sxtl  v2.4s, v0.4h   -> R0 = coeffs[0..3]
	WORD $0x4f10a403 // sxtl2 v3.4s, v0.8h   -> R1 = coeffs[4..7]
	WORD $0x0f10a424 // sxtl  v4.4s, v1.4h   -> R2 = coeffs[8..11]
	WORD $0x4f10a425 // sxtl2 v5.4s, v1.8h   -> R3 = coeffs[12..15]

	// Pass 1 (vertical), lane-parallel over the four columns.
	WORD $0x4ebf9c66 // mul  v6.4s, v3.4s, v31.4s
	WORD $0x4f3004c6 // sshr v6.4s, v6.4s, #16          -> m2r1 = mul2(R1)
	WORD $0x4ebe9ca7 // mul  v7.4s, v5.4s, v30.4s
	WORD $0x4f3004e7 // sshr v7.4s, v7.4s, #16
	VADD V5.S4, V7.S4, V7.S4 //                          -> m1r3 = mul1(R3)
	WORD $0x4ebe9c68 // mul  v8.4s, v3.4s, v30.4s
	WORD $0x4f300508 // sshr v8.4s, v8.4s, #16
	VADD V3.S4, V8.S4, V8.S4 //                          -> m1r1 = mul1(R1)
	WORD $0x4ebf9ca9 // mul  v9.4s, v5.4s, v31.4s
	WORD $0x4f300529 // sshr v9.4s, v9.4s, #16           -> m2r3 = mul2(R3)

	VADD V2.S4, V4.S4, V10.S4 // a = R0 + R2
	VSUB V4.S4, V2.S4, V11.S4 // b = R0 - R2
	VSUB V7.S4, V6.S4, V12.S4 // c = m2r1 - m1r3
	VADD V8.S4, V9.S4, V13.S4 // d = m1r1 + m2r3
	VADD V10.S4, V13.S4, V14.S4 // P0 = a + d
	VADD V11.S4, V12.S4, V15.S4 // P1 = b + c
	VSUB V12.S4, V11.S4, V16.S4 // P2 = b - c
	VSUB V13.S4, V10.S4, V17.S4 // P3 = a - d

	// Transpose P0..P3 (rows) -> W0..W3 (columns).
	VTRN1 V15.S4, V14.S4, V18.S4
	VTRN2 V15.S4, V14.S4, V19.S4
	VTRN1 V17.S4, V16.S4, V20.S4
	VTRN2 V17.S4, V16.S4, V21.S4
	VZIP1 V20.D2, V18.D2, V22.D2 // W0
	VZIP1 V21.D2, V19.D2, V23.D2 // W1
	VZIP2 V20.D2, V18.D2, V24.D2 // W2
	VZIP2 V21.D2, V19.D2, V25.D2 // W3

	// Pass 2 (horizontal), lane-parallel over the four rows.
	VADD V22.S4, V29.S4, V26.S4 // dc = W0 + 4
	VADD V26.S4, V24.S4, V2.S4  // a = dc + W2
	VSUB V24.S4, V26.S4, V3.S4  // b = dc - W2
	WORD $0x4ebf9ee6 // mul  v6.4s, v23.4s, v31.4s
	WORD $0x4f3004c6 // sshr v6.4s, v6.4s, #16           -> m2w1 = mul2(W1)
	WORD $0x4ebe9f27 // mul  v7.4s, v25.4s, v30.4s
	WORD $0x4f3004e7 // sshr v7.4s, v7.4s, #16
	VADD V25.S4, V7.S4, V7.S4 //                          -> m1w3 = mul1(W3)
	VSUB V7.S4, V6.S4, V4.S4  // c = m2w1 - m1w3
	WORD $0x4ebe9ee8 // mul  v8.4s, v23.4s, v30.4s
	WORD $0x4f300508 // sshr v8.4s, v8.4s, #16
	VADD V23.S4, V8.S4, V8.S4 //                          -> m1w1 = mul1(W1)
	WORD $0x4ebf9f29 // mul  v9.4s, v25.4s, v31.4s
	WORD $0x4f300529 // sshr v9.4s, v9.4s, #16            -> m2w3 = mul2(W3)
	VADD V8.S4, V9.S4, V5.S4 // d = m1w1 + m2w3

	VADD V2.S4, V5.S4, V10.S4 // a + d
	WORD $0x4f3d054a          // sshr v10.4s, v10.4s, #3  -> O0
	VADD V3.S4, V4.S4, V11.S4 // b + c
	WORD $0x4f3d056b          // sshr v11.4s, v11.4s, #3  -> O1
	VSUB V4.S4, V3.S4, V12.S4 // b - c
	WORD $0x4f3d058c          // sshr v12.4s, v12.4s, #3  -> O2
	VSUB V5.S4, V2.S4, V13.S4 // a - d
	WORD $0x4f3d05ad          // sshr v13.4s, v13.4s, #3  -> O3

	// Transpose O0..O3 -> Z0..Z3, one output row per vector.
	VTRN1 V11.S4, V10.S4, V18.S4
	VTRN2 V11.S4, V10.S4, V19.S4
	VTRN1 V13.S4, V12.S4, V20.S4
	VTRN2 V13.S4, V12.S4, V21.S4
	VZIP1 V20.D2, V18.D2, V22.D2 // Z0 = row 0
	VZIP1 V21.D2, V19.D2, V23.D2 // Z1 = row 1
	VZIP2 V20.D2, V18.D2, V24.D2 // Z2 = row 2
	VZIP2 V21.D2, V19.D2, V25.D2 // Z3 = row 3

	// Add each output row to the plane pixels, clip to [0,255], store 4 bytes.
	FMOVS (R0), F26
	VUXTL V26.B8, V27.H8
	VUXTL V27.H4, V26.S4
	VADD V22.S4, V26.S4, V26.S4
	WORD $0x2e612b5b // sqxtun v27.4h, v26.4s
	WORD $0x2e214b7b // uqxtn  v27.8b, v27.8h
	FMOVS F27, (R0)
	ADD R1, R0, R0

	FMOVS (R0), F26
	VUXTL V26.B8, V27.H8
	VUXTL V27.H4, V26.S4
	VADD V23.S4, V26.S4, V26.S4
	WORD $0x2e612b5b // sqxtun v27.4h, v26.4s
	WORD $0x2e214b7b // uqxtn  v27.8b, v27.8h
	FMOVS F27, (R0)
	ADD R1, R0, R0

	FMOVS (R0), F26
	VUXTL V26.B8, V27.H8
	VUXTL V27.H4, V26.S4
	VADD V24.S4, V26.S4, V26.S4
	WORD $0x2e612b5b // sqxtun v27.4h, v26.4s
	WORD $0x2e214b7b // uqxtn  v27.8b, v27.8h
	FMOVS F27, (R0)
	ADD R1, R0, R0

	FMOVS (R0), F26
	VUXTL V26.B8, V27.H8
	VUXTL V27.H4, V26.S4
	VADD V25.S4, V26.S4, V26.S4
	WORD $0x2e612b5b // sqxtun v27.4h, v26.4s
	WORD $0x2e214b7b // uqxtn  v27.8b, v27.8h
	FMOVS F27, (R0)

	RET
