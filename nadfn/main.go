package main

import (
	"os"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	"github.com/henderiw-nephio/pkg-examples/nadfn/mutator"
)

func main() {

	if err := fn.AsMain(fn.ResourceListProcessorFunc(mutator.Run)); err != nil {
		os.Exit(1)
	}
	//if err := fn.AsMain(fn.ResourceListProcessorFunc(nfdeployfn.Run)); err != nil {
	//	os.Exit(1)
	//}
	//if err := fn.AsMain(fn.ResourceListProcessorFunc(nadfn.Run)); err != nil {
	//	os.Exit(1)
	//}
	//if err := fn.AsMain(fn.ResourceListProcessorFunc(dnnfn.Run)); err != nil {
	//	os.Exit(1)
	//}
}
