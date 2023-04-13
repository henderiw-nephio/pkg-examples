package main

import (
	"os"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	"github.com/example.com/foo/interfacefn"
)

func main() {
	if err := fn.AsMain(fn.ResourceListProcessorFunc(interfacefn.Run)); err != nil {
		os.Exit(1)
	}
}
