//go:build !testbenchmark

package main

import "fmt"

func main() {
	fmt.Println("webpbench is test-only; build with -tags testbenchmark (needs cgo + libwebp + pkg-config)")
}
