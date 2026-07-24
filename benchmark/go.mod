// Separate module for benchmark tooling and cross-encoder compatibility tests.
// These pull in cgo (libwebp) and other third-party encoders; keeping them out
// of the root module means consumers of the library get zero dependencies.
module github.com/SeriousBug/webp-go-pure/benchmark

go 1.26

require (
	github.com/SeriousBug/webp-go-pure v0.0.0
	github.com/gen2brain/webp v0.6.4
	github.com/kolesa-team/go-webp v1.0.5
	golang.org/x/image v0.44.0
)

require github.com/ebitengine/purego v0.10.1 // indirect

replace github.com/SeriousBug/webp-go-pure => ../
