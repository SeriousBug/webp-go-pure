#include "textflag.h"

// func elossyPlaneSseAsm(srcPtr, decPtr unsafe.Pointer, srcStride, decStride, width, height int) uint64
//
// Sum of squared differences over a width x height region of two independently
// pitched planes. Per-byte diffs are in [-255,255], so a 16-bit multiply yields
// exact squares. Squares accumulate in four uint32 lanes that are drained into
// the 64-bit total once per row, which keeps the lanes far from overflow for
// any plausible row width.
TEXT ·elossyPlaneSseAsm(SB), NOSPLIT, $0-56
	MOVD srcPtr+0(FP), R0
	MOVD decPtr+8(FP), R1
	MOVD srcStride+16(FP), R2
	MOVD decStride+24(FP), R3
	MOVD width+32(FP), R4
	MOVD height+40(FP), R5
	MOVD $0, R6

rowLoop:
	MOVD R0, R7
	MOVD R1, R8
	MOVD R4, R9
	VEOR V6.B16, V6.B16, V6.B16

	CMP $16, R9
	BLT tail8

wide16:
	VLD1.P 16(R7), [V0.B16]
	VLD1.P 16(R8), [V1.B16]
	VUXTL  V0.B8, V2.H8
	VUXTL  V1.B8, V3.H8
	VSUB   V3.H8, V2.H8, V4.H8
	WORD   $0x4e649c85 // VMUL V4.H8, V4.H8, V5.H8
	VUADDW V5.H4, V6.S4, V6.S4
	VUADDW2 V5.H8, V6.S4, V6.S4
	VUXTL2 V0.B16, V2.H8
	VUXTL2 V1.B16, V3.H8
	VSUB   V3.H8, V2.H8, V4.H8
	WORD   $0x4e649c85 // VMUL V4.H8, V4.H8, V5.H8
	VUADDW V5.H4, V6.S4, V6.S4
	VUADDW2 V5.H8, V6.S4, V6.S4
	SUB    $16, R9, R9
	CMP    $16, R9
	BGE    wide16

tail8:
	CMP $8, R9
	BLT tailBytes

	VLD1.P 8(R7), [V0.B8]
	VLD1.P 8(R8), [V1.B8]
	VUXTL  V0.B8, V2.H8
	VUXTL  V1.B8, V3.H8
	VSUB   V3.H8, V2.H8, V4.H8
	WORD   $0x4e649c85 // VMUL V4.H8, V4.H8, V5.H8
	VUADDW V5.H4, V6.S4, V6.S4
	VUADDW2 V5.H8, V6.S4, V6.S4
	SUB    $8, R9, R9

tailBytes:
	CBZ R9, rowDone

byteLoop:
	MOVBU.P 1(R7), R10
	MOVBU.P 1(R8), R11
	SUB     R11, R10, R10
	MUL     R10, R10, R10
	ADD     R10, R6, R6
	SUBS    $1, R9, R9
	BNE     byteLoop

rowDone:
	VUADDLV V6.S4, V7
	VMOV    V7.D[0], R12
	ADD     R12, R6, R6
	ADD     R2, R0, R0
	ADD     R3, R1, R1
	SUBS    $1, R5, R5
	BNE     rowLoop

	MOVD R6, ret+48(FP)
	RET
