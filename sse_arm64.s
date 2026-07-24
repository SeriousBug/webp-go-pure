#include "textflag.h"

// func elossyBlockSseAsm(srcPtr, recPtr unsafe.Pointer, srcStride, n int) uint64
//
// Sum of squared differences over an n-wide, n-row block. Per-byte diffs are in
// [-255,255], so a 16-bit multiply yields exact squares (<=65025). Squares are
// accumulated into four uint32 lanes (V6) via widening pairwise adds, then summed.
// The Go assembler exposes no vector integer multiply, so VMUL V.8H is emitted as
// a raw WORD: 0x4e649c85 == "VMUL V4.H8, V4.H8, V5.H8".
TEXT ·elossyBlockSseAsm(SB), NOSPLIT, $0-40
	MOVD srcPtr+0(FP), R0
	MOVD recPtr+8(FP), R1
	MOVD srcStride+16(FP), R2
	MOVD n+24(FP), R3
	VEOR V6.B16, V6.B16, V6.B16 // accumulator: 4x uint32

	CMP $16, R3
	BEQ loop16

loop8:
	VLD1 (R0), [V0.B8]
	VLD1 (R1), [V1.B8]
	VUXTL V0.B8, V2.H8
	VUXTL V1.B8, V3.H8
	VSUB V3.H8, V2.H8, V4.H8
	WORD $0x4e649c85 // VMUL V4.H8, V4.H8, V5.H8
	VUADDW V5.H4, V6.S4, V6.S4
	VUADDW2 V5.H8, V6.S4, V6.S4
	ADD R2, R0, R0
	ADD $8, R1, R1
	SUBS $1, R3, R3
	BNE loop8
	B done

loop16:
	VLD1 (R0), [V0.B16]
	VLD1 (R1), [V1.B16]
	VUXTL V0.B8, V2.H8
	VUXTL V1.B8, V3.H8
	VSUB V3.H8, V2.H8, V4.H8
	WORD $0x4e649c85 // VMUL V4.H8, V4.H8, V5.H8
	VUADDW V5.H4, V6.S4, V6.S4
	VUADDW2 V5.H8, V6.S4, V6.S4
	VUXTL2 V0.B16, V2.H8
	VUXTL2 V1.B16, V3.H8
	VSUB V3.H8, V2.H8, V4.H8
	WORD $0x4e649c85 // VMUL V4.H8, V4.H8, V5.H8
	VUADDW V5.H4, V6.S4, V6.S4
	VUADDW2 V5.H8, V6.S4, V6.S4
	ADD R2, R0, R0
	ADD $16, R1, R1
	SUBS $1, R3, R3
	BNE loop16

done:
	VUADDLV V6.S4, V7
	VMOV V7.D[0], R4
	MOVD R4, ret+32(FP)
	RET
