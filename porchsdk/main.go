package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	"github.com/henderiw-nephio/pkg-examples/pkg/utils"
	"sigs.k8s.io/kustomize/kyaml/kio"
)

func main() {

	files, err := utils.ReadFiles("../data/pkg-upf", []string{"*.yaml", "*.yml", "Kptfile"})
	if err != nil {
		log.Fatal(err)
	}

	inputs := []kio.Reader{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			panic(err)
		}
		//fmt.Println(string(b))
		inputs = append(inputs, &kio.ByteReader{
			Reader: strings.NewReader(string(b)),
			//SetAnnotations: map[string]string{
			//	kioutil.PathAnnotation: path,
			//},
			DisableUnwrapping: true,
		})
	}

	var pb kio.PackageBuffer
	err = kio.Pipeline{
		Inputs:  inputs,
		Filters: []kio.Filter{},
		Outputs: []kio.Writer{&pb},
	}.Execute()
	if err != nil {
		panic(err)
	}

	rl := fn.ResourceList{
		Items: fn.KubeObjects{},
	}
	for _, n := range pb.Nodes {
		s, err := n.String()
		if err != nil {
			panic(err)
		}
		o, err := fn.ParseKubeObject([]byte(s))
		if err != nil {
			panic(err)
		}
		if err := rl.UpsertObjectToItems(o, nil, true); err != nil {
			panic(err)
		}
	}
	fmt.Println(rl)
}
