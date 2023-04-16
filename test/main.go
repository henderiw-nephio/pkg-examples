package main

import (
	"fmt"
	"os"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
)

func main() {
	b, err := os.ReadFile("../data/pkg-upf/ipallocation_n4.yaml")
	if err != nil {
		panic(err)
	}
	o, err := fn.ParseKubeObject(b)
	if err != nil {
		panic(err)
	}

	spec := &map[string]any{}
	ok, err := o.NestedResource(spec, "spec")
	if err != nil {
		panic(err)
	}

	fmt.Println(ok)
	fmt.Println(spec)
}
