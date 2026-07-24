#include "textflag.h"

// func elossyBlockSseAsm(srcPtr, recPtr unsafe.Pointer, srcStride, n int) uint64
//
// Sum of squared differences over an n-wide, n-row block. Per-byte diffs are in
// [-255,255], so they fit int16; PMADDWL of a diff vector against itself squares
// each lane and adds adjacent pairs, yielding int32 partial sums accumulated in
// X6. A 16x16 block sums to at most 256*65025 < 2^31, so the int32 lanes never
// overflow. Only SSE2 instructions are used.
TEXT ·elossyBlockSseAsm(SB), NOSPLIT, $0-40
	MOVQ srcPtr+0(FP), SI
	MOVQ recPtr+8(FP), DI
	MOVQ srcStride+16(FP), BX
	MOVQ n+24(FP), CX
	PXOR X7, X7 // zero (for byte->int16 widening)
	PXOR X6, X6 // accumulator: 4x int32

	CMPQ CX, $16
	JEQ loop16

loop8:
	MOVQ  (SI), X0 // 8 src bytes
	MOVQ  (DI), X1 // 8 rec bytes
	PUNPCKLBW X7, X0
	PUNPCKLBW X7, X1
	PSUBW X1, X0
	PMADDWL X0, X0
	PADDD X0, X6
	ADDQ BX, SI
	ADDQ $8, DI
	SUBQ $1, CX
	JNE  loop8
	JMP  done

loop16:
	MOVOU (SI), X0 // 16 src bytes
	MOVOU (DI), X1 // 16 rec bytes
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
	ADDQ BX, SI
	ADDQ $16, DI
	SUBQ $1, CX
	JNE  loop16

done:
	PSHUFD $0x0E, X6, X0 // [c,d,-,-]
	PADDD  X0, X6
	PSHUFD $0x01, X6, X0 // [b+d,-,-,-]
	PADDD  X0, X6
	MOVD   X6, AX
	MOVL   AX, AX // keep low 32 (lane 0), clear high
	MOVQ   AX, ret+32(FP)
	RET

// func elossyBlockSse4x4Asm(srcPtr, candPtr unsafe.Pointer, srcStride int) uint64
//
// Sum of squared differences over a 4x4 block: a contiguous 16-byte candidate
// against a strided source. The four 4-byte source rows are gathered into one
// 16-byte vector, then reuse the widen/subtract/square/accumulate path above.
TEXT ·elossyBlockSse4x4Asm(SB), NOSPLIT, $0-32
	MOVQ srcPtr+0(FP), SI
	MOVQ candPtr+8(FP), DI
	MOVQ srcStride+16(FP), BX

	MOVD (SI), X0
	ADDQ BX, SI
	MOVD (SI), X1
	ADDQ BX, SI
	MOVD (SI), X2
	ADDQ BX, SI
	MOVD (SI), X3
	PUNPCKLLQ  X1, X0 // [r0, r1, 0, 0]
	PUNPCKLLQ  X3, X2 // [r2, r3, 0, 0]
	PUNPCKLQDQ X2, X0 // [r0, r1, r2, r3]

	MOVOU (DI), X1

	PXOR X7, X7
	PXOR X6, X6
	MOVO X0, X2
	PUNPCKLBW X7, X2
	PUNPCKHBW X7, X0
	MOVO X1, X3
	PUNPCKLBW X7, X3
	PUNPCKHBW X7, X1
	PSUBW X3, X2
	PSUBW X1, X0
	PMADDWL X2, X2
	PMADDWL X0, X0
	PADDD X2, X6
	PADDD X0, X6

	PSHUFD $0x0E, X6, X0
	PADDD  X0, X6
	PSHUFD $0x01, X6, X0
	PADDD  X0, X6
	MOVD   X6, AX
	MOVL   AX, AX
	MOVQ   AX, ret+24(FP)
	RET
