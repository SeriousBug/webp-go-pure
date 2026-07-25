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

// TDISTO_BLOCK scores one 4x4 residual: source rows at SI with pitch BX (and
// R10 == 3*BX) against prediction rows at DI with pitch R9 (and R11 == 3*R9),
// weights preloaded in X13/X14, zero in X12, result in AX. SI and DI are left
// untouched; X0-X11 are clobbered.
//
// The four rows of each block are gathered into two vectors (rows 0,1 and rows
// 2,3), widened and subtracted, so the residual lives in X0 = [row0|row1] and
// X2 = [row2|row3] as int16. Each butterfly pass combines the four rows
// lane-wise, which is the vertical transform; the pass runs twice with a
// transpose in between, so the second one is the horizontal transform.
// Coefficients stay within int16 (|d| <= 255, gain 16). The weights are
// symmetric, so it does not matter that the kernel leaves the result transposed
// relative to the scalar version.
//
// PMADDWL sums adjacent weighted pairs, which is all the final reduction needs
// since every coefficient is already non-negative after PABSW. PABSW is SSSE3,
// as are the PSHUFBs in predscore_amd64.s.
#define TDISTO_BLOCK \
	MOVD (SI), X0            \
	MOVD (SI)(BX*1), X1      \
	MOVD (SI)(BX*2), X2      \
	MOVD (SI)(R10*1), X3     \
	MOVD (DI), X4            \
	MOVD (DI)(R9*1), X5      \
	MOVD (DI)(R9*2), X6      \
	MOVD (DI)(R11*1), X7     \
	PUNPCKLLQ X1, X0         \
	PUNPCKLLQ X3, X2         \
	PUNPCKLLQ X5, X4         \
	PUNPCKLLQ X7, X6         \
	PUNPCKLBW X12, X0        \
	PUNPCKLBW X12, X2        \
	PUNPCKLBW X12, X4        \
	PUNPCKLBW X12, X6        \
	PSUBW X4, X0             \
	PSUBW X6, X2             \
	MOVO  X0, X8             \
	PADDW X2, X8             \
	PSUBW X2, X0             \
	PSHUFD $0x4E, X8, X9     \
	PSHUFD $0x4E, X0, X10    \
	MOVO  X8, X11            \
	PADDW X9, X11            \
	PSUBW X9, X8             \
	MOVO  X0, X9             \
	PADDW X10, X9            \
	PSUBW X10, X0            \
	PUNPCKLQDQ X9, X11       \
	PUNPCKLQDQ X8, X0        \
	MOVO  X0, X2             \
	MOVO  X11, X0            \
	MOVO  X0, X8             \
	PUNPCKLWL X2, X8         \
	MOVO  X0, X9             \
	PUNPCKHWL X2, X9         \
	MOVO  X8, X0             \
	PUNPCKLWL X9, X0         \
	PUNPCKHWL X9, X8         \
	MOVO  X8, X2             \
	MOVO  X0, X8             \
	PADDW X2, X8             \
	PSUBW X2, X0             \
	PSHUFD $0x4E, X8, X9     \
	PSHUFD $0x4E, X0, X10    \
	MOVO  X8, X11            \
	PADDW X9, X11            \
	PSUBW X9, X8             \
	MOVO  X0, X9             \
	PADDW X10, X9            \
	PSUBW X10, X0            \
	PUNPCKLQDQ X9, X11       \
	PUNPCKLQDQ X8, X0        \
	PABSW X11, X4            \
	PABSW X0, X5             \
	PMADDWL X14, X4          \
	PMADDWL X13, X5          \
	PADDD X5, X4             \
	PSHUFD $0x0E, X4, X6     \
	PADDD X6, X4             \
	PSHUFD $0x01, X4, X6     \
	PADDD X6, X4             \
	MOVD  X4, AX             \
	SHRL  $5, AX

// func elossyTDisto4x4Asm(srcPtr, predPtr unsafe.Pointer, srcStride, predStride int) uint32
//
// Weighted Hadamard SATD of a single 4x4 residual.
TEXT ·elossyTDisto4x4Asm(SB), NOSPLIT, $0-36
	MOVQ srcPtr+0(FP), SI
	MOVQ predPtr+8(FP), DI
	MOVQ srcStride+16(FP), BX
	MOVQ predStride+24(FP), R9
	LEAQ (BX)(BX*2), R10
	LEAQ (R9)(R9*2), R11
	PXOR X12, X12
	MOVOU elossyTDistoWeights<>+0(SB), X14
	MOVOU elossyTDistoWeights<>+16(SB), X13

	TDISTO_BLOCK

	MOVL AX, ret+32(FP)
	RET

// func elossyTDistoBlocksAsm(srcPtr, predPtr unsafe.Pointer, srcStride, predStride, cols, rows int) uint32
//
// Sums the SATD of a cols x rows grid of 4x4 blocks. Each block is rounded down
// on its own, exactly as the per-block callers did.
TEXT ·elossyTDistoBlocksAsm(SB), NOSPLIT, $0-52
	MOVQ srcPtr+0(FP), R12
	MOVQ predPtr+8(FP), R13
	MOVQ srcStride+16(FP), BX
	MOVQ predStride+24(FP), R9
	MOVQ cols+32(FP), CX
	MOVQ rows+40(FP), DX
	LEAQ (BX)(BX*2), R10
	LEAQ (R9)(R9*2), R11
	PXOR X12, X12
	MOVOU elossyTDistoWeights<>+0(SB), X14
	MOVOU elossyTDistoWeights<>+16(SB), X13
	XORQ R8, R8 // running total

rowLoop:
	MOVQ R12, SI
	MOVQ R13, DI
	MOVQ CX, R15

colLoop:
	TDISTO_BLOCK
	ADDQ AX, R8
	ADDQ $4, SI
	ADDQ $4, DI
	SUBQ $1, R15
	JNE  colLoop

	LEAQ (R12)(BX*4), R12
	LEAQ (R13)(R9*4), R13
	SUBQ $1, DX
	JNE  rowLoop

	MOVL R8, ret+48(FP)
	RET
