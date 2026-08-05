// Package register makes image.Decode and image.DecodeConfig recognize WebP.
// Import it for its side effect:
//
//	import _ "github.com/SeriousBug/webp-go-pure/std/register"
//
// Registration lives in its own package because image.RegisterFormat is a
// process-wide global with no way to unregister and no rejection of duplicates.
// golang.org/x/image/webp and github.com/gen2brain/webp both claim the same
// magic, so a package that registers from its own init would decide, for every
// program that links it anywhere in its dependency tree, which decoder wins.
// Importing this package is that decision, made deliberately.
package register

import (
	"image"

	"github.com/SeriousBug/webp-go-pure/std"
)

func init() {
	// The magic matches VP8, VP8L and VP8X, the three things the fourth chunk
	// can be, so this covers lossy, lossless and extended containers alike.
	image.RegisterFormat("webp", "RIFF????WEBPVP8", std.Decode, std.DecodeConfig)
}
