# Notes on development

All scripts noted below can also be called from the accompanying [Makefile](../Makefile).

## Generating Kubernetes client code

The following script will generate the necessary boilerplate Kubernetes code to support the CRDs:

    $ ./hack/generate_code.sh

## Protofbufs

The following command will generate the necessary code for the plugins:

    $ ./hack/generate_protobuf.sh
