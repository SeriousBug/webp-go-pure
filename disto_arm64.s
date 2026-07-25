#include "textflag.h"

// Weighted Hadamard SATD weights (libwebp kWeightY) as int16, in the lane order
// the kernel leaves its coefficients in.
DATA elossyTDistoWeights<>+0(SB)/2, $38
DATA elossyTDistoWeights<>+2(SB)/2, $32
DATA elossyTDistoWeights<>+4(SB)/2, $20
DATA elossyTDistoWeights<>+6(SB)/2, $9
DATA elossyTDistoWeights<>+8(SB)/2, $32
DATA elossyTDistoWeights<>+10(SB)/2, $28
DATA elossyTDistoWeights<>+12(SB)/2, $17
DATA elossyTDistoWeights<>+14(SB)/2, $7
DATA elossyTDistoWeights<>+16(SB)/2, $20
DATA elossyTDistoWeights<>+18(SB)/2, $17
DATA elossyTDistoWeights<>+20(SB)/2, $10
DATA elossyTDistoWeights<>+22(SB)/2, $4
DATA elossyTDistoWeights<>+24(SB)/2, $9
DATA elossyTDistoWeights<>+26(SB)/2, $7
DATA elossyTDistoWeights<>+28(SB)/2, $4
DATA elossyTDistoWeights<>+30(SB)/2, $2
GLOBL elossyTDistoWeights<>(SB), RODATA|NOPTR, $32

// TDISTO_BLOCK scores one 4x4 residual: source rows at R0 with pitch R2 against
// prediction rows at R1 with pitch R3, weights preloaded in V28/V29, result in
// R7. R0 and R1 are left untouched; R4, R5, R6 and V0-V26 are clobbered.
//
// The four rows of each block are gathered into two vectors (rows 0,1 and rows
// 2,3), widened and subtracted, so the residual lives in V8 = [row0|row1] and
// V9 = [row2|row3] as int16. Each butterfly pass combines the four rows
// lane-wise, which is the vertical transform; the pass runs twice with a
// transpose in between, so the second one is the horizontal transform.
// Coefficients stay within int16 (|d| <= 255, gain 16). The weights are
// symmetric, so it does not matter that the kernel leaves the result transposed
// relative to the scalar version.
//
// The Go assembler has no vector ABS or widening multiply-accumulate, so those
// are emitted as raw WORDs:
//   0x4e60b914 == "abs  v20.8h, v8.8h"
//   0x4e60b935 == "abs  v21.8h, v9.8h"
//   0x0e7c8298 == "smlal  v24.4s, v20.4h, v28.4h"
//   0x4e7c8298 == "smlal2 v24.4s, v20.8h, v28.8h"
//   0x0e7d82b9 == "smlal  v25.4s, v21.4h, v29.4h"
//   0x4e7d82b9 == "smlal2 v25.4s, v21.8h, v29.8h"
#define TDISTO_BLOCK \
	MOVD  R0, R4                        \
	MOVD  R1, R5                        \
	MOVWU (R4), R6                      \
	VMOV  R6, V0.S[0]                   \
	ADD   R2, R4, R4                    \
	MOVWU (R4), R6                      \
	VMOV  R6, V0.S[1]                   \
	ADD   R2, R4, R4                    \
	MOVWU (R4), R6                      \
	VMOV  R6, V1.S[0]                   \
	ADD   R2, R4, R4                    \
	MOVWU (R4), R6                      \
	VMOV  R6, V1.S[1]                   \
	MOVWU (R5), R6                      \
	VMOV  R6, V2.S[0]                   \
	ADD   R3, R5, R5                    \
	MOVWU (R5), R6                      \
	VMOV  R6, V2.S[1]                   \
	ADD   R3, R5, R5                    \
	MOVWU (R5), R6                      \
	VMOV  R6, V3.S[0]                   \
	ADD   R3, R5, R5                    \
	MOVWU (R5), R6                      \
	VMOV  R6, V3.S[1]                   \
	VUXTL V0.B8, V4.H8                  \
	VUXTL V2.B8, V5.H8                  \
	VSUB  V5.H8, V4.H8, V8.H8           \
	VUXTL V1.B8, V6.H8                  \
	VUXTL V3.B8, V7.H8                  \
	VSUB  V7.H8, V6.H8, V9.H8           \
	VADD  V9.H8, V8.H8, V10.H8          \
	VSUB  V9.H8, V8.H8, V11.H8          \
	VEXT  $8, V10.B16, V10.B16, V12.B16 \
	VEXT  $8, V11.B16, V11.B16, V13.B16 \
	VADD  V12.H8, V10.H8, V14.H8        \
	VSUB  V12.H8, V10.H8, V15.H8        \
	VADD  V13.H8, V11.H8, V16.H8        \
	VSUB  V13.H8, V11.H8, V17.H8        \
	VZIP1 V16.D2, V14.D2, V8.D2         \
	VZIP1 V15.D2, V17.D2, V9.D2         \
	VZIP1 V9.H8, V8.H8, V10.H8          \
	VZIP2 V9.H8, V8.H8, V11.H8          \
	VZIP1 V11.H8, V10.H8, V8.H8         \
	VZIP2 V11.H8, V10.H8, V9.H8         \
	VADD  V9.H8, V8.H8, V10.H8          \
	VSUB  V9.H8, V8.H8, V11.H8          \
	VEXT  $8, V10.B16, V10.B16, V12.B16 \
	VEXT  $8, V11.B16, V11.B16, V13.B16 \
	VADD  V12.H8, V10.H8, V14.H8        \
	VSUB  V12.H8, V10.H8, V15.H8        \
	VADD  V13.H8, V11.H8, V16.H8        \
	VSUB  V13.H8, V11.H8, V17.H8        \
	VZIP1 V16.D2, V14.D2, V8.D2         \
	VZIP1 V15.D2, V17.D2, V9.D2         \
	WORD  $0x4e60b914                   \
	WORD  $0x4e60b935                   \
	VEOR  V24.B16, V24.B16, V24.B16     \
	VEOR  V25.B16, V25.B16, V25.B16     \
	WORD  $0x0e7c8298                   \
	WORD  $0x4e7c8298                   \
	WORD  $0x0e7d82b9                   \
	WORD  $0x4e7d82b9                   \
	VADD    V25.S4, V24.S4, V24.S4      \
	VUADDLV V24.S4, V26                 \
	VMOV    V26.D[0], R7                \
	LSR     $5, R7, R7

// func elossyTDisto4x4Asm(srcPtr, predPtr unsafe.Pointer, srcStride, predStride int) uint32
//
// Weighted Hadamard SATD of a single 4x4 residual.
TEXT ·elossyTDisto4x4Asm(SB), NOSPLIT, $0-36
	MOVD srcPtr+0(FP), R0
	MOVD predPtr+8(FP), R1
	MOVD srcStride+16(FP), R2
	MOVD predStride+24(FP), R3
	MOVD $elossyTDistoWeights<>(SB), R9
	VLD1 (R9), [V28.H8, V29.H8]

	TDISTO_BLOCK

	MOVW R7, ret+32(FP)
	RET

// func elossyTDistoBlocksAsm(srcPtr, predPtr unsafe.Pointer, srcStride, predStride, cols, rows int) uint32
//
// Sums the SATD of a cols x rows grid of 4x4 blocks. Each block is rounded down
// on its own, exactly as the per-block callers did.
TEXT ·elossyTDistoBlocksAsm(SB), NOSPLIT, $0-52
	MOVD srcPtr+0(FP), R10
	MOVD predPtr+8(FP), R11
	MOVD srcStride+16(FP), R2
	MOVD predStride+24(FP), R3
	MOVD cols+32(FP), R14
	MOVD rows+40(FP), R15
	MOVD $elossyTDistoWeights<>(SB), R9
	VLD1 (R9), [V28.H8, V29.H8]

	LSL  $2, R2, R12 // 4 source rows
	LSL  $2, R3, R13 // 4 prediction rows
	MOVD $0, R19     // running total

rowLoop:
	MOVD R10, R0
	MOVD R11, R1
	MOVD R14, R20

colLoop:
	TDISTO_BLOCK
	ADD  R7, R19, R19
	ADD  $4, R0, R0
	ADD  $4, R1, R1
	SUBS $1, R20, R20
	BNE  colLoop

	ADD  R12, R10, R10
	ADD  R13, R11, R11
	SUBS $1, R15, R15
	BNE  rowLoop

	MOVW R19, ret+48(FP)
	RET
