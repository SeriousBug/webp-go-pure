#include "textflag.h"

// func elosslessScorePredictorRowAsm(actPtr unsafe.Pointer, rowBytes, groups int, costs *[14]uint64)
//
// Scores 4 interior pixels per iteration for all 14 predictor modes and sums the
// per-mode wrapped-channel errors into costs. Each pixel is 4 packed bytes
// (B,G,R,A); the wrapped per-byte error is min((a-p)&0xff, (p-a)&0xff), and
// Average2(a,b) is (a&b) + ((a^b)>>1) per byte. The two clamped modes narrow a
// signed 16-bit result to a saturated unsigned byte with SQXTUN/SQXTUN2, which
// the Go assembler lacks, so those are emitted as raw WORDs.
//
// V0..V4  : ACT, LEFT, TOP, TL, TR (current 4 pixels + neighbours)
// V5..V15 : predictor scratch
// V16,V17 : error scratch
// V18..V31: 14 per-mode uint16x8 accumulators (bounded: <=32-wide tiles)

// w = min(ACT-PRED, PRED-ACT) per byte; ACC.H8 += widen(w)
#define ERR(PRED, ACC) \
	VSUB PRED.B16, V0.B16, V16.B16 \
	VSUB V0.B16, PRED.B16, V17.B16 \
	VUMIN V16.B16, V17.B16, V16.B16 \
	VUADDW V16.B8, ACC.H8, ACC.H8 \
	VUADDW2 V16.B16, ACC.H8, ACC.H8

// DST = Average2(A, B) per byte; clobbers T
#define AVG(A, B, DST, T) \
	VEOR A.B16, B.B16, T.B16 \
	VUSHR $1, T.B16, T.B16 \
	VAND A.B16, B.B16, DST.B16 \
	VADD T.B16, DST.B16, DST.B16

TEXT ·elosslessScorePredictorRowAsm(SB), NOSPLIT, $0-32
	MOVD actPtr+0(FP), R0
	MOVD rowBytes+8(FP), R1
	MOVD groups+16(FP), R2
	MOVD costs+24(FP), R3
	MOVD $0xff000000, R4

	VEOR V18.B16, V18.B16, V18.B16
	VEOR V19.B16, V19.B16, V19.B16
	VEOR V20.B16, V20.B16, V20.B16
	VEOR V21.B16, V21.B16, V21.B16
	VEOR V22.B16, V22.B16, V22.B16
	VEOR V23.B16, V23.B16, V23.B16
	VEOR V24.B16, V24.B16, V24.B16
	VEOR V25.B16, V25.B16, V25.B16
	VEOR V26.B16, V26.B16, V26.B16
	VEOR V27.B16, V27.B16, V27.B16
	VEOR V28.B16, V28.B16, V28.B16
	VEOR V29.B16, V29.B16, V29.B16
	VEOR V30.B16, V30.B16, V30.B16
	VEOR V31.B16, V31.B16, V31.B16

	CMP $0, R2
	BEQ done

loop:
	VLD1 (R0), [V0.B16]  // ACT
	SUB $4, R0, R9
	VLD1 (R9), [V1.B16]  // LEFT
	SUB R1, R0, R10      // R10 -> TOP row
	VLD1 (R10), [V2.B16] // TOP
	SUB $4, R10, R9
	VLD1 (R9), [V3.B16]  // TL
	ADD $4, R10, R9
	VLD1 (R9), [V4.B16]  // TR

	// mode 0: 0xff000000 per pixel
	VDUP R4, V5.S4
	ERR(V5, V18)

	// modes 1..4: LEFT, TOP, TR, TL
	ERR(V1, V19)
	ERR(V2, V20)
	ERR(V4, V21)
	ERR(V3, V22)

	// mode 5: AVG(AVG(LEFT,TR), TOP)
	AVG(V1, V4, V5, V6)
	AVG(V5, V2, V5, V6)
	ERR(V5, V23)

	// mode 6: AVG(LEFT, TL)
	AVG(V1, V3, V5, V6)
	ERR(V5, V24)

	// mode 7: AVG(LEFT, TOP)
	AVG(V1, V2, V5, V6)
	ERR(V5, V25)

	// mode 8: AVG(TL, TOP)
	AVG(V3, V2, V5, V6)
	ERR(V5, V26)

	// mode 9: AVG(TOP, TR)
	AVG(V2, V4, V5, V6)
	ERR(V5, V27)

	// mode 10: AVG(AVG(LEFT,TL), AVG(TOP,TR))
	AVG(V1, V3, V5, V7)
	AVG(V2, V4, V6, V7)
	AVG(V5, V6, V5, V7)
	ERR(V5, V28)

	// mode 11: Select(LEFT, TOP, TL)
	// dT = |TOP-TL|, dL = |LEFT-TL| (true byte abs diff)
	VUMAX V2.B16, V3.B16, V5.B16
	VUMIN V2.B16, V3.B16, V6.B16
	VSUB V6.B16, V5.B16, V5.B16    // dT
	VUMAX V1.B16, V3.B16, V6.B16
	VUMIN V1.B16, V3.B16, V7.B16
	VSUB V7.B16, V6.B16, V6.B16    // dL
	// leftDist per pixel (from dT) -> V9.S4
	VUXTL V5.B8, V7.H8
	VUXTL2 V5.B16, V8.H8
	VADDP V8.H8, V7.H8, V9.H8
	VADDP V9.H8, V9.H8, V9.H8
	VUXTL V9.H4, V9.S4
	// topDist per pixel (from dL) -> V10.S4
	VUXTL V6.B8, V7.H8
	VUXTL2 V6.B16, V8.H8
	VADDP V8.H8, V7.H8, V10.H8
	VADDP V10.H8, V10.H8, V10.H8
	VUXTL V10.H4, V10.S4
	// selLeft = leftDist < topDist  (per 32-bit pixel lane)
	VUMIN V9.S4, V10.S4, V11.S4
	VCMEQ V11.S4, V9.S4, V12.S4    // leftDist == min  (leftDist <= topDist)
	VCMEQ V9.S4, V9.S4, V7.S4      // all ones
	VCMEQ V9.S4, V10.S4, V13.S4    // leftDist == topDist
	VEOR V13.B16, V7.B16, V13.B16  // leftDist != topDist
	VAND V12.B16, V13.B16, V12.B16 // selLeft
	// pred = (selLeft & LEFT) | (~selLeft & TOP)
	VAND V12.B16, V1.B16, V8.B16
	VEOR V12.B16, V7.B16, V9.B16   // ~selLeft
	VAND V9.B16, V2.B16, V9.B16
	VORR V8.B16, V9.B16, V5.B16
	ERR(V5, V29)

	// mode 12: clip255(LEFT + TOP - TL)
	VUXTL V1.B8, V5.H8
	VUXTL V2.B8, V6.H8
	VUXTL V3.B8, V7.H8
	VADD V6.H8, V5.H8, V5.H8
	VSUB V7.H8, V5.H8, V5.H8       // s_lo
	VUXTL2 V1.B16, V8.H8
	VUXTL2 V2.B16, V9.H8
	VUXTL2 V3.B16, V10.H8
	VADD V9.H8, V8.H8, V8.H8
	VSUB V10.H8, V8.H8, V8.H8      // s_hi
	WORD $0x2E2128AB               // SQXTUN V11.8B, V5.8H
	WORD $0x6E21290B               // SQXTUN2 V11.16B, V8.8H
	ERR(V11, V30)

	// mode 13: clip255(avg + trunc((avg-TL)/2)), avg = AVG(LEFT,TOP)
	AVG(V1, V2, V5, V6)            // V5 = avg (bytes)
	VUXTL V5.B8, V6.H8             // avg lo
	VUXTL V3.B8, V7.H8             // TL lo
	VSUB V7.H8, V6.H8, V8.H8       // d
	VUSHR $15, V8.H8, V9.H8        // isneg(d)
	VADD V9.H8, V8.H8, V8.H8       // d2 = d + isneg
	VUSHR $15, V8.H8, V9.H8        // isneg(d2)
	VUSHR $1, V8.H8, V10.H8        // d2 >> 1 (logical)
	VSHL $15, V9.H8, V9.H8         // sign << 15
	VORR V9.B16, V10.B16, V10.B16  // arithmetic d2>>1 = dh
	VADD V10.H8, V6.H8, V6.H8      // r_lo = avg_lo + dh
	VUXTL2 V5.B16, V7.H8           // avg hi
	VUXTL2 V3.B16, V8.H8           // TL hi
	VSUB V8.H8, V7.H8, V9.H8       // d
	VUSHR $15, V9.H8, V10.H8       // isneg(d)
	VADD V10.H8, V9.H8, V9.H8      // d2
	VUSHR $15, V9.H8, V10.H8       // isneg(d2)
	VUSHR $1, V9.H8, V11.H8        // d2 >> 1
	VSHL $15, V10.H8, V10.H8
	VORR V10.B16, V11.B16, V11.B16 // dh
	VADD V11.H8, V7.H8, V7.H8      // r_hi = avg_hi + dh
	WORD $0x2E2128CC               // SQXTUN V12.8B, V6.8H
	WORD $0x6E2128EC               // SQXTUN2 V12.16B, V7.8H
	ERR(V12, V31)

	ADD $16, R0, R0
	SUBS $1, R2, R2
	BNE loop

done:
	// reduce each accumulator (sum of uint16 lanes) into costs[m]
	VUADDLV V18.H8, V0
	VMOV V0.S[0], R9
	MOVD 0(R3), R10
	ADD R9, R10, R10
	MOVD R10, 0(R3)
	VUADDLV V19.H8, V0
	VMOV V0.S[0], R9
	MOVD 8(R3), R10
	ADD R9, R10, R10
	MOVD R10, 8(R3)
	VUADDLV V20.H8, V0
	VMOV V0.S[0], R9
	MOVD 16(R3), R10
	ADD R9, R10, R10
	MOVD R10, 16(R3)
	VUADDLV V21.H8, V0
	VMOV V0.S[0], R9
	MOVD 24(R3), R10
	ADD R9, R10, R10
	MOVD R10, 24(R3)
	VUADDLV V22.H8, V0
	VMOV V0.S[0], R9
	MOVD 32(R3), R10
	ADD R9, R10, R10
	MOVD R10, 32(R3)
	VUADDLV V23.H8, V0
	VMOV V0.S[0], R9
	MOVD 40(R3), R10
	ADD R9, R10, R10
	MOVD R10, 40(R3)
	VUADDLV V24.H8, V0
	VMOV V0.S[0], R9
	MOVD 48(R3), R10
	ADD R9, R10, R10
	MOVD R10, 48(R3)
	VUADDLV V25.H8, V0
	VMOV V0.S[0], R9
	MOVD 56(R3), R10
	ADD R9, R10, R10
	MOVD R10, 56(R3)
	VUADDLV V26.H8, V0
	VMOV V0.S[0], R9
	MOVD 64(R3), R10
	ADD R9, R10, R10
	MOVD R10, 64(R3)
	VUADDLV V27.H8, V0
	VMOV V0.S[0], R9
	MOVD 72(R3), R10
	ADD R9, R10, R10
	MOVD R10, 72(R3)
	VUADDLV V28.H8, V0
	VMOV V0.S[0], R9
	MOVD 80(R3), R10
	ADD R9, R10, R10
	MOVD R10, 80(R3)
	VUADDLV V29.H8, V0
	VMOV V0.S[0], R9
	MOVD 88(R3), R10
	ADD R9, R10, R10
	MOVD R10, 88(R3)
	VUADDLV V30.H8, V0
	VMOV V0.S[0], R9
	MOVD 96(R3), R10
	ADD R9, R10, R10
	MOVD R10, 96(R3)
	VUADDLV V31.H8, V0
	VMOV V0.S[0], R9
	MOVD 104(R3), R10
	ADD R9, R10, R10
	MOVD R10, 104(R3)
	RET
