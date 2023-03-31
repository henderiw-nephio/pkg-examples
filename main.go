package main

import (
	"os"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	"github.com/example.com/foo/mutatordownstream"
)

func main() {
	if err := fn.AsMain(fn.ResourceListProcessorFunc(mutatordownstream.Run)); err != nil {
		os.Exit(1)
	}
}
