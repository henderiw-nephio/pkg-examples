package main

import (
	"os"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	"github.com/example.com/foo/nfdeployfn"
)

func main() {
	if err := fn.AsMain(fn.ResourceListProcessorFunc(nfdeployfn.Run)); err != nil {
		os.Exit(1)
	}
	//if err := fn.AsMain(fn.ResourceListProcessorFunc(nadfn.Run)); err != nil {
	//	os.Exit(1)
	//}
	//if err := fn.AsMain(fn.ResourceListProcessorFunc(dnnfn.Run)); err != nil {
	//	os.Exit(1)
	//}
}
