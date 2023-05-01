# pkg examples

contain example of NF blueprint packages.

kpt fn eval --type mutator ./data/pkg-upf  -i europe-docker.pkg.dev/srlinux/eu.gcr.io/interface-fn:latest --truncate-output=false

kpt fn eval --type mutator ./data/pkg-upf  -i europe-docker.pkg.dev/srlinux/eu.gcr.io/ipam-fn:latest --truncate-output=false

kpt fn eval --type mutator ./data/pkg-upf  -i europe-docker.pkg.dev/srlinux/eu.gcr.io/nad-fn:latest --truncate-output=false