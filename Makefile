.PHONY: codegen
codegen:
	${GOPATH}/src/k8s.io/code-generator/generate-groups.sh all \
	github.com/alice02/crd-controller-sample/pkg/client github.com/alice02/crd-controller-sample/pkg/apis \
	mysamplecontroller:v1
