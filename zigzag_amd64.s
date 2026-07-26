#include "textflag.h"

// PSHUFB index vectors for the zigzag permutation. PSHUFB only indexes within
// one register, so each output half is gathered from the low and high halves of
// the levels separately and merged with POR; an index with bit 7 set zeroes its
// byte, which is what the lanes sourced from the other half carry.
//
// The permutation is elossyZigzag = 0,1,4,8,5,2,3,6,9,12,13,10,7,11,14,15;
// output lane s takes the two bytes of input lane elossyZigzag[s].
DATA elossyZigzagShufALo<>+0(SB)/8, $0x8080090803020100
DATA elossyZigzagShufALo<>+8(SB)/8, $0x0d0c070605040b0a
GLOBL elossyZigzagShufALo<>(SB), RODATA|NOPTR, $16

DATA elossyZigzagShufAHi<>+0(SB)/8, $0x0100808080808080
DATA elossyZigzagShufAHi<>+8(SB)/8, $0x8080808080808080
GLOBL elossyZigzagShufAHi<>(SB), RODATA|NOPTR, $16

DATA elossyZigzagShufBLo<>+0(SB)/8, $0x8080808080808080
DATA elossyZigzagShufBLo<>+8(SB)/8, $0x8080808080800f0e
GLOBL elossyZigzagShufBLo<>(SB), RODATA|NOPTR, $16

DATA elossyZigzagShufBHi<>+0(SB)/8, $0x05040b0a09080302
DATA elossyZigzagShufBHi<>+8(SB)/8, $0x0f0e0d0c07068080
GLOBL elossyZigzagShufBHi<>(SB), RODATA|NOPTR, $16

// func elossyZigzagLastAsm(levels, zigzagged *[16]int16, firstMask uint64) int
//
// Permutes the 16 levels into zigzag scan order and returns the index of the
// last non-zero one, or -1 if there is none. firstMask clears the low lane's
// bit pair so a caller scanning from index 1 ignores the DC level.
//
// PCMPEQW plus PMOVMSKB turns the scan into a 32-bit word carrying two set bits
// per non-zero lane, so the last index is one BSR rather than a backwards scan
// through the zigzag indirection.
TEXT ·elossyZigzagLastAsm(SB), NOSPLIT, $0-32
	MOVQ levels+0(FP), SI
	MOVQ zigzagged+8(FP), DI
	MOVQ firstMask+16(FP), CX

	MOVOU (SI), X0
	MOVOU 16(SI), X1

	MOVOU elossyZigzagShufALo<>(SB), X8
	MOVOU elossyZigzagShufAHi<>(SB), X9
	MOVOU elossyZigzagShufBLo<>(SB), X10
	MOVOU elossyZigzagShufBHi<>(SB), X11

	MOVO   X0, X2
	PSHUFB X8, X2
	MOVO   X1, X3
	PSHUFB X9, X3
	POR    X3, X2

	MOVO   X0, X4
	PSHUFB X10, X4
	MOVO   X1, X5
	PSHUFB X11, X5
	POR    X5, X4

	MOVOU X2, (DI)
	MOVOU X4, 16(DI)

	PXOR    X6, X6
	PCMPEQW X6, X2 // 0xFFFF per zero lane
	PXOR    X7, X7
	PCMPEQW X7, X4

	PMOVMSKB X2, AX
	PMOVMSKB X4, BX
	SHLL     $16, BX
	ORL      BX, AX
	NOTL     AX // two set bits per non-zero lane
	ANDL     CX, AX

	BSRL AX, DX
	JZ   none
	SHRL $1, DX
	MOVQ DX, ret+24(FP)
	RET

none:
	MOVQ $-1, ret+24(FP)
	RET
