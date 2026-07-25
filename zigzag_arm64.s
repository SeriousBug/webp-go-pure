#include "textflag.h"

// func elossyZigzagLastAsm(levels, zigzagged *[16]int16, firstMask uint64) int
//
// Permutes the 16 levels into zigzag scan order and returns the index of the
// last non-zero one, or -1 if there is none. firstMask clears the low lane's
// nibble so a caller scanning from index 1 ignores the DC level.
//
// The non-zero lanes become a nibble each via CMTST + SHRN, so the last index
// is a count of leading zeros over two 64-bit words rather than a backwards
// scan through the zigzag indirection.
// WORD-encoded ops (cmtst/shrn) were produced by clang and verified with
// objdump; the rest are native.
TEXT ·elossyZigzagLastAsm(SB), NOSPLIT, $0-32
	MOVD levels+0(FP), R0
	MOVD zigzagged+8(FP), R1
	MOVD firstMask+16(FP), R6

	MOVD $·elossyZigzagBytes(SB), R7
	VLD1 (R7), [V8.B16, V9.B16]
	VLD1 (R0), [V0.B16, V1.B16]

	VTBL V8.B16, [V0.B16, V1.B16], V4.B16
	VTBL V9.B16, [V0.B16, V1.B16], V5.B16
	VST1 [V4.B16, V5.B16], (R1)

	WORD $0x4e648c86 // cmtst v6.8h, v4.8h, v4.8h   all-ones per non-zero lane
	WORD $0x4e658ca7 // cmtst v7.8h, v5.8h, v5.8h
	WORD $0x0f0c84c6 // shrn v6.8b, v6.8h, #4       one 0xFF byte per lane
	WORD $0x0f0c84e7 // shrn v7.8b, v7.8h, #4

	VMOV V6.D[0], R2
	VMOV V7.D[0], R3
	AND  R6, R2, R2

	MOVD $63, R5
	CBZ  R3, low
	CLZ  R3, R4
	SUB  R4, R5, R4
	LSR  $3, R4, R4
	ADD  $8, R4, R4
	MOVD R4, ret+24(FP)
	RET

low:
	CBZ R2, none
	CLZ R2, R4
	SUB R4, R5, R4
	LSR $3, R4, R4
	MOVD R4, ret+24(FP)
	RET

none:
	MOVD $-1, R4
	MOVD R4, ret+24(FP)
	RET
