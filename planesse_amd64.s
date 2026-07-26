#include "textflag.h"

// func elossyPlaneSseAsm(srcPtr, decPtr unsafe.Pointer, srcStride, decStride, width, height int) uint64
//
// Sum of squared differences over a width x height region of two independently
// pitched planes. Per-byte diffs are in [-255,255], so PMADDWL of a diff vector
// against itself squares each lane and adds adjacent pairs. The int32 lanes are
// drained into the 64-bit total once per row; a pair contributes at most 130050,
// so a lane would need a row past 130000 pixels wide to overflow. Only SSE2
// instructions are used.
TEXT ·elossyPlaneSseAsm(SB), NOSPLIT, $0-56
	MOVQ srcPtr+0(FP), SI
	MOVQ decPtr+8(FP), DI
	MOVQ srcStride+16(FP), R8
	MOVQ decStride+24(FP), R9
	MOVQ width+32(FP), R10
	MOVQ height+40(FP), R11
	XORQ R12, R12 // running total
	PXOR X7, X7   // zero (for byte->int16 widening)

rowLoop:
	MOVQ SI, AX
	MOVQ DI, BX
	MOVQ R10, CX
	PXOR X6, X6 // per-row accumulator: 4x int32

wide16:
	CMPQ  CX, $16
	JLT   tail8
	MOVOU (AX), X0
	MOVOU (BX), X1
	MOVO  X0, X2
	PUNPCKLBW X7, X2
	PUNPCKHBW X7, X0
	MOVO  X1, X3
	PUNPCKLBW X7, X3
	PUNPCKHBW X7, X1
	PSUBW X3, X2
	PSUBW X1, X0
	PMADDWL X2, X2
	PMADDWL X0, X0
	PADDD X2, X6
	PADDD X0, X6
	ADDQ  $16, AX
	ADDQ  $16, BX
	SUBQ  $16, CX
	JMP   wide16

tail8:
	CMPQ CX, $8
	JLT  tailBytes
	MOVQ (AX), X0
	MOVQ (BX), X1
	PUNPCKLBW X7, X0
	PUNPCKLBW X7, X1
	PSUBW X1, X0
	PMADDWL X0, X0
	PADDD X0, X6
	ADDQ  $8, AX
	ADDQ  $8, BX
	SUBQ  $8, CX

tailBytes:
	TESTQ CX, CX
	JZ    rowDone

byteLoop:
	MOVBLZX (AX), DX
	MOVBLZX (BX), R13
	SUBQ    R13, DX
	IMULQ   DX, DX
	ADDQ    DX, R12
	INCQ    AX
	INCQ    BX
	SUBQ    $1, CX
	JNE     byteLoop

rowDone:
	PSHUFD $0x0E, X6, X0
	PADDD  X0, X6
	PSHUFD $0x01, X6, X0
	PADDD  X0, X6
	MOVD   X6, DX
	MOVL   DX, DX // keep low 32 (lane 0), clear high
	ADDQ   DX, R12
	ADDQ   R8, SI
	ADDQ   R9, DI
	SUBQ   $1, R11
	JNE    rowLoop

	MOVQ R12, ret+48(FP)
	RET
