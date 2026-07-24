#include "textflag.h"

// MUL32 computes dst = low32(src * konst) for each of the four int32 lanes.
// SSE2 has no packed 32-bit multiply, so emulate it with PMULUDQ (unsigned
// 32x32->64): the low 32 bits of a product are identical for signed and
// unsigned operands. PMULUDQ reads lanes 0 and 2; shifting src right one dword
// brings lanes 1 and 3 into those slots. Clobbers dst and t; src, konst kept
// (unless dst aliases src).
#define MUL32(dst, src, konst, t) \
	MOVO      src, dst   \
	PMULULQ   konst, dst \
	MOVO      src, t     \
	PSRLO     $4, t      \
	PMULULQ   konst, t   \
	PSHUFD    $0x08, dst, dst \
	PSHUFD    $0x08, t, t     \
	PUNPCKLLQ t, dst

// MUL1 computes dst = ((src * kC1) >> 16) + src  (VP8 transform mul1).
#define MUL1(dst, src, t) \
	MUL32(dst, src, X14, t) \
	PSRAL $16, dst \
	PADDL src, dst

// MUL2 computes dst = (src * kC2) >> 16  (VP8 transform mul2).
#define MUL2(dst, src, t) \
	MUL32(dst, src, X15, t) \
	PSRAL $16, dst

// TRANSPOSE transposes the 4x4 int32 matrix held in r0..r3 (row-major) in
// place, so afterwards r_k holds column k. t0..t3 are scratch.
#define TRANSPOSE(r0, r1, r2, r3, t0, t1, t2, t3) \
	MOVO       r0, t0 \
	PUNPCKLLQ  r1, t0 \
	MOVO       r2, t1 \
	PUNPCKLLQ  r3, t1 \
	MOVO       r0, t2 \
	PUNPCKHLQ  r1, t2 \
	MOVO       r2, t3 \
	PUNPCKHLQ  r3, t3 \
	MOVO       t0, r0 \
	PUNPCKLQDQ t1, r0 \
	MOVO       t0, r1 \
	PUNPCKHQDQ t1, r1 \
	MOVO       t2, r2 \
	PUNPCKLQDQ t3, r2 \
	MOVO       t2, r3 \
	PUNPCKHQDQ t3, r3

// SEXT32 sign-extends the low four int16 lanes of src into the four int32
// lanes of dst. SSE2 lacks PMOVSXWD, so shift each word into the high half of
// a dword and arithmetic-shift back. Clobbers dst; src preserved.
#define SEXT32(dst, src) \
	MOVO      src, dst \
	PUNPCKLWL dst, dst \
	PSRAL     $16, dst

// func elossyAddTransformAsm(planePtr unsafe.Pointer, stride int, coeffs *[16]int16)
//
// SSE2 port of the VP8 4x4 inverse transform + add-to-plane (libwebp
// TransformOne). Two butterfly passes with a 4x4 int32 transpose between them.
// The mul1/mul2 constant multiplies use full 32-bit products (MUL32), so the
// result matches the scalar int32 reference bit-for-bit including wrap on
// overflow. The clip to [0,255] is PACKSSLW then PACKUSWB.
TEXT ·elossyAddTransformAsm(SB), NOSPLIT, $0-24
	MOVQ planePtr+0(FP), DI
	MOVQ stride+8(FP), BX
	MOVQ coeffs+16(FP), SI

	MOVL     $20091, AX
	MOVD     AX, X14
	PSHUFD   $0x00, X14, X14 // kC1 broadcast
	MOVL     $35468, AX
	MOVD     AX, X15
	PSHUFD   $0x00, X15, X15 // kC2 broadcast

	// Load coeffs rows C0..C3 (each 4x int16) and sign-extend to int32.
	MOVOU    (SI), X10       // coeffs[0..7]
	MOVOU    16(SI), X11     // coeffs[8..15]
	SEXT32(X0, X10)          // C0 = coeffs[0..3]
	PSRLO    $8, X10
	SEXT32(X1, X10)          // C1 = coeffs[4..7]
	SEXT32(X2, X11)          // C2 = coeffs[8..11]
	PSRLO    $8, X11
	SEXT32(X3, X11)          // C3 = coeffs[12..15]

	// Pass 1 (over columns, lane = column index).
	MOVO   X0, X4
	PADDL  X2, X4           // a = C0 + C2
	MOVO   X0, X5
	PSUBL  X2, X5           // b = C0 - C2
	MUL2(X6, X1, X13)       // mul2(C1)
	MUL1(X7, X3, X13)       // mul1(C3)
	PSUBL  X7, X6           // c = mul2(C1) - mul1(C3)
	MUL1(X7, X1, X13)       // mul1(C1)
	MUL2(X8, X3, X13)       // mul2(C3)
	PADDL  X8, X7           // d = mul1(C1) + mul2(C3)

	MOVO   X4, X9
	PADDL  X7, X9           // P0 = a + d
	PSUBL  X7, X4           // P3 = a - d
	MOVO   X5, X8
	PADDL  X6, X8           // P1 = b + c
	PSUBL  X6, X5           // P2 = b - c
	MOVO   X9, X0           // P0
	MOVO   X8, X1           // P1
	MOVO   X5, X2           // P2
	MOVO   X4, X3           // P3

	TRANSPOSE(X0, X1, X2, X3, X4, X5, X6, X7) // U0..U3

	// Pass 2 (over rows, lane = row index).
	MOVL   $4, AX
	MOVD   AX, X12
	PSHUFD $0x00, X12, X12
	MOVO   X0, X4
	PADDL  X12, X4          // dc = U0 + 4
	MOVO   X4, X5
	PADDL  X2, X4           // a = dc + U2
	PSUBL  X2, X5           // b = dc - U2
	MUL2(X6, X1, X13)       // mul2(U1)
	MUL1(X7, X3, X13)       // mul1(U3)
	PSUBL  X7, X6           // c = mul2(U1) - mul1(U3)
	MUL1(X7, X1, X13)       // mul1(U1)
	MUL2(X8, X3, X13)       // mul2(U3)
	PADDL  X8, X7           // d = mul1(U1) + mul2(U3)

	MOVO   X4, X9
	PADDL  X7, X9
	PSRAL  $3, X9           // O0 = (a+d) >> 3
	MOVO   X4, X8
	PSUBL  X7, X8
	PSRAL  $3, X8           // O3 = (a-d) >> 3
	MOVO   X5, X4
	PADDL  X6, X4
	PSRAL  $3, X4           // O1 = (b+c) >> 3
	PSUBL  X6, X5
	PSRAL  $3, X5           // O2 = (b-c) >> 3
	MOVO   X9, X0           // O0
	MOVO   X4, X1           // O1
	MOVO   X5, X2           // O2
	MOVO   X8, X3           // O3

	TRANSPOSE(X0, X1, X2, X3, X4, X5, X6, X7) // Z0..Z3 (row deltas)

	PXOR   X7, X7           // zero for byte widening

	// Row 0.
	MOVL      (DI), X5
	PUNPCKLBW X7, X5
	PUNPCKLWL X7, X5
	PADDL     X0, X5
	PACKSSLW  X5, X5
	PACKUSWB  X5, X5
	MOVL      X5, (DI)
	ADDQ      BX, DI
	// Row 1.
	MOVL      (DI), X5
	PUNPCKLBW X7, X5
	PUNPCKLWL X7, X5
	PADDL     X1, X5
	PACKSSLW  X5, X5
	PACKUSWB  X5, X5
	MOVL      X5, (DI)
	ADDQ      BX, DI
	// Row 2.
	MOVL      (DI), X5
	PUNPCKLBW X7, X5
	PUNPCKLWL X7, X5
	PADDL     X2, X5
	PACKSSLW  X5, X5
	PACKUSWB  X5, X5
	MOVL      X5, (DI)
	ADDQ      BX, DI
	// Row 3.
	MOVL      (DI), X5
	PUNPCKLBW X7, X5
	PUNPCKLWL X7, X5
	PADDL     X3, X5
	PACKSSLW  X5, X5
	PACKUSWB  X5, X5
	MOVL      X5, (DI)
	RET

// BCAST loads a 32-bit immediate into all four lanes of dst via AX.
#define BCAST(dst, imm) \
	MOVL   imm, AX \
	MOVD   AX, dst \
	PSHUFD $0x00, dst, dst

// LOADDIFF loads a strided 4-byte src/pred row, widens both to int32, and
// stores src-pred into dst. Advances SI/DI by their strides. X12 must be zero.
#define LOADDIFF(dst) \
	MOVL      (SI), X4 \
	PUNPCKLBW X12, X4 \
	PUNPCKLWL X12, X4 \
	MOVL      (DI), X5 \
	PUNPCKLBW X12, X5 \
	PUNPCKLWL X12, X5 \
	PSUBL     X5, X4 \
	MOVO      X4, dst \
	ADDQ      R8, SI \
	ADDQ      R9, DI

// func elossyForwardTransformAsm(srcPtr unsafe.Pointer, srcStride int, predPtr unsafe.Pointer, predStride int, out *[16]int16)
//
// SSE2 port of the VP8 forward transform (libwebp FTransform): per-row
// butterfly, transpose, per-column butterfly, writing 16 int16 coefficients.
// Multiplies use the exact 2217/5352 constants with full 32-bit products
// (MUL32). For byte-range diffs every intermediate fits int16, so the final
// PACKSSLW narrow is exact (matching int16() truncation, as libwebp's own SSE2
// kernel does). The out[4+i] "+1 if a3!=0" bias uses PCMPEQL then +1.
TEXT ·elossyForwardTransformAsm(SB), NOSPLIT, $0-40
	MOVQ srcPtr+0(FP), SI
	MOVQ srcStride+8(FP), R8
	MOVQ predPtr+16(FP), DI
	MOVQ predStride+24(FP), R9
	MOVQ out+32(FP), R10

	BCAST(X14, $2217)
	BCAST(X15, $5352)
	PXOR X12, X12

	LOADDIFF(X0) // D0
	LOADDIFF(X1) // D1
	LOADDIFF(X2) // D2
	LOADDIFF(X3) // D3

	TRANSPOSE(X0, X1, X2, X3, X4, X5, X6, X7) // E0..E3 (lane = row)

	// Pass 1 (rows).
	MOVO  X0, X4
	PADDL X3, X4  // a0 = E0 + E3
	MOVO  X1, X5
	PADDL X2, X5  // a1 = E1 + E2
	MOVO  X1, X6
	PSUBL X2, X6  // a2 = E1 - E2
	MOVO  X0, X7
	PSUBL X3, X7  // a3 = E0 - E3

	MOVO  X4, X0
	PADDL X5, X0
	PSLLL $3, X0  // T0 = (a0+a1)*8
	MOVO  X4, X2
	PSUBL X5, X2
	PSLLL $3, X2  // T2 = (a0-a1)*8

	MUL32(X1, X6, X14, X13) // a2*2217
	MUL32(X8, X7, X15, X13) // a3*5352
	PADDL X8, X1
	BCAST(X9, $1812)
	PADDL X9, X1
	PSRAL $9, X1  // T1

	MUL32(X8, X7, X14, X13) // a3*2217
	MUL32(X3, X6, X15, X13) // a2*5352
	PSUBL X3, X8
	BCAST(X9, $937)
	PADDL X9, X8
	PSRAL $9, X8  // T3
	MOVO  X8, X3

	TRANSPOSE(X0, X1, X2, X3, X4, X5, X6, X7) // G0..G3 (lane = column)

	// Pass 2 (columns).
	MOVO  X0, X4
	PADDL X3, X4  // a0 = G0 + G3
	MOVO  X1, X5
	PADDL X2, X5  // a1 = G1 + G2
	MOVO  X1, X6
	PSUBL X2, X6  // a2 = G1 - G2
	MOVO  X0, X7
	PSUBL X3, X7  // a3 = G0 - G3

	BCAST(X9, $7)
	MOVO  X4, X0
	PADDL X5, X0
	PADDL X9, X0
	PSRAL $4, X0  // O0 = (a0+a1+7)>>4
	MOVO  X4, X2
	PSUBL X5, X2
	PADDL X9, X2
	PSRAL $4, X2  // O2 = (a0-a1+7)>>4

	MUL32(X1, X6, X14, X13) // a2*2217
	MUL32(X8, X7, X15, X13) // a3*5352
	PADDL X8, X1
	BCAST(X9, $12000)
	PADDL X9, X1
	PSRAL $16, X1
	MOVO  X7, X10
	PCMPEQL X12, X10 // -1 where a3==0
	BCAST(X9, $1)
	PADDL X9, X10    // 1 where a3!=0, else 0
	PADDL X10, X1    // O1

	MUL32(X8, X7, X14, X13) // a3*2217
	MUL32(X3, X6, X15, X13) // a2*5352
	PSUBL X3, X8
	BCAST(X9, $51000)
	PADDL X9, X8
	PSRAL $16, X8    // O3
	MOVO  X8, X3

	PACKSSLW X0, X0
	MOVQ     X0, (R10)
	PACKSSLW X1, X1
	MOVQ     X1, 8(R10)
	PACKSSLW X2, X2
	MOVQ     X2, 16(R10)
	PACKSSLW X3, X3
	MOVQ     X3, 24(R10)
	RET
