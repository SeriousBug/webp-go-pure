#include "textflag.h"

// func elosslessScorePredictorRowAsm(actPtr unsafe.Pointer, rowBytes, groups int, costs *[14]uint64)
//
// SSE port of the predictor-mode scorer: 4 pixels per iteration, all 14 modes.
// x86 has only 16 XMM registers, too few to hold 14 accumulators, so each mode's
// per-group error is reduced with PSADBW and added straight into costs memory.
// Wrapped per-byte error is min((a-p)&0xff,(p-a)&0xff); Average2 is
// (a&b)+(((a^b)&0xfe)>>1) per byte; the two clamped modes narrow signed words to
// saturated unsigned bytes with PACKUSWB.
//
// X0..X4 : ACT, LEFT, TOP, TL, TR
// X5     : zero (PSADBW / byte->word widening)
// X6     : 0xFE... Average2 mask
// X15    : 0xff000000 per pixel (mode 0)

DATA avgMask<>+0(SB)/8, $0xfefefefefefefefe
DATA avgMask<>+8(SB)/8, $0xfefefefefefefefe
GLOBL avgMask<>(SB), RODATA|NOPTR, $16
DATA predAlpha<>+0(SB)/8, $0xff000000ff000000
DATA predAlpha<>+8(SB)/8, $0xff000000ff000000
GLOBL predAlpha<>(SB), RODATA|NOPTR, $16

// DST = Average2(A, B) per byte; clobbers T (T must differ from A and B)
#define AVGX(A, B, DST, T) \
	MOVOU A, T          \
	PAND B, T           \
	MOVOU A, DST        \
	PXOR B, DST         \
	PAND X6, DST        \
	PSRLW $1, DST       \
	PADDB T, DST

// costs[OFF/8] += sum over the 4 pixels of the wrapped error against PRED
#define ERRX(PRED, OFF) \
	MOVOU X0, X10        \
	PSUBB PRED, X10      \
	MOVOU PRED, X11      \
	PSUBB X0, X11        \
	PMINUB X11, X10      \
	PSADBW X5, X10       \
	MOVQ X10, AX         \
	PSHUFD $0xEE, X10, X11 \
	MOVQ X11, DX         \
	ADDQ DX, AX          \
	ADDQ AX, OFF(DI)

TEXT ·elosslessScorePredictorRowAsm(SB), NOSPLIT, $0-32
	MOVQ actPtr+0(FP), SI
	MOVQ rowBytes+8(FP), BX
	MOVQ groups+16(FP), CX
	MOVQ costs+24(FP), DI
	PXOR X5, X5
	MOVOU avgMask<>(SB), X6
	MOVOU predAlpha<>(SB), X15
	TESTQ CX, CX
	JZ done

loop:
	MOVOU (SI), X0      // ACT
	MOVOU -4(SI), X1    // LEFT
	MOVQ SI, AX
	SUBQ BX, AX         // AX -> TOP row
	MOVOU (AX), X2      // TOP
	MOVOU -4(AX), X3    // TL
	MOVOU 4(AX), X4     // TR

	ERRX(X15, 0)  // mode 0: 0xff000000
	ERRX(X1, 8)   // mode 1: LEFT
	ERRX(X2, 16)  // mode 2: TOP
	ERRX(X4, 24)  // mode 3: TR
	ERRX(X3, 32)  // mode 4: TL

	// mode 5: AVG(AVG(LEFT,TR), TOP)
	AVGX(X1, X4, X7, X8)
	AVGX(X7, X2, X7, X8)
	ERRX(X7, 40)

	// mode 6: AVG(LEFT, TL)
	AVGX(X1, X3, X7, X8)
	ERRX(X7, 48)

	// mode 7: AVG(LEFT, TOP)
	AVGX(X1, X2, X7, X8)
	ERRX(X7, 56)

	// mode 8: AVG(TL, TOP)
	AVGX(X3, X2, X7, X8)
	ERRX(X7, 64)

	// mode 9: AVG(TOP, TR)
	AVGX(X2, X4, X7, X8)
	ERRX(X7, 72)

	// mode 10: AVG(AVG(LEFT,TL), AVG(TOP,TR))
	AVGX(X1, X3, X7, X9)
	AVGX(X2, X4, X8, X9)
	AVGX(X7, X8, X7, X9)
	ERRX(X7, 80)

	// mode 11: Select(LEFT, TOP, TL)
	MOVOU X2, X7
	PMAXUB X3, X7
	MOVOU X2, X8
	PMINUB X3, X8
	PSUBB X8, X7        // dT = |TOP-TL|
	MOVOU X1, X8
	PMAXUB X3, X8
	MOVOU X1, X9
	PMINUB X3, X9
	PSUBB X9, X8        // dL = |LEFT-TL|
	// leftDist (per-pixel dword sums of dT) -> X9
	MOVOU X7, X9
	PUNPCKLBW X5, X9
	MOVOU X7, X10
	PUNPCKHBW X5, X10
	PHADDW X10, X9
	PHADDW X9, X9
	PMOVZXWD X9, X9
	// topDist (from dL) -> X10
	MOVOU X8, X10
	PUNPCKLBW X5, X10
	MOVOU X8, X11
	PUNPCKHBW X5, X11
	PHADDW X11, X10
	PHADDW X10, X10
	PMOVZXWD X10, X10
	// selLeft = leftDist < topDist (dword lanes) -> X12
	MOVOU X9, X11
	PMINUD X10, X11
	PCMPEQL X9, X11     // leftDist <= topDist
	MOVOU X9, X12
	PCMPEQL X10, X12    // leftDist == topDist
	PANDN X11, X12      // X12 = (~eq) & (<=)  = leftDist < topDist
	// pred = (selLeft & LEFT) | (~selLeft & TOP) -> X13
	MOVOU X12, X13
	PAND X1, X13
	MOVOU X12, X14
	PANDN X2, X14
	POR X14, X13
	ERRX(X13, 88)

	// mode 12: clip255(LEFT + TOP - TL)
	MOVOU X1, X7
	PUNPCKLBW X5, X7
	MOVOU X2, X8
	PUNPCKLBW X5, X8
	MOVOU X3, X9
	PUNPCKLBW X5, X9
	PADDW X8, X7
	PSUBW X9, X7        // s_lo
	MOVOU X1, X8
	PUNPCKHBW X5, X8
	MOVOU X2, X9
	PUNPCKHBW X5, X9
	MOVOU X3, X12
	PUNPCKHBW X5, X12
	PADDW X9, X8
	PSUBW X12, X8       // s_hi
	PACKUSWB X8, X7     // pred
	ERRX(X7, 96)

	// mode 13: clip255(avg + trunc((avg-TL)/2)), avg = AVG(LEFT,TOP)
	AVGX(X1, X2, X7, X8)   // X7 = avg (bytes)
	MOVOU X7, X8
	PUNPCKLBW X5, X8       // avg lo
	MOVOU X3, X9
	PUNPCKLBW X5, X9       // TL lo
	MOVOU X8, X12
	PSUBW X9, X12          // d
	MOVOU X12, X13
	PSRLW $15, X13         // isneg
	PADDW X13, X12         // d2 = d + isneg
	PSRAW $1, X12          // dh = arith d2>>1
	PADDW X12, X8          // r_lo = avg_lo + dh
	MOVOU X7, X9
	PUNPCKHBW X5, X9       // avg hi
	MOVOU X3, X12
	PUNPCKHBW X5, X12      // TL hi
	MOVOU X9, X13
	PSUBW X12, X13         // d
	MOVOU X13, X14
	PSRLW $15, X14         // isneg
	PADDW X14, X13         // d2
	PSRAW $1, X13          // dh
	PADDW X13, X9          // r_hi
	PACKUSWB X9, X8        // pred
	ERRX(X8, 104)

	ADDQ $16, SI
	DECQ CX
	JNE loop

done:
	RET
