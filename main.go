package main

import (
	"os"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	"github.com/example.com/foo/mutator2"
)

func main() {
	if err := fn.AsMain(fn.ResourceListProcessorFunc(mutator2.Run)); err != nil {
		os.Exit(1)
	}
}
