package main

import (
	"os"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	"github.com/henderiw-nephio/pkg-examples/interfacefn/mutator"
)

func main() {

	if err := fn.AsMain(fn.ResourceListProcessorFunc(mutator.Run)); err != nil {
		os.Exit(1)
	}
}
