package controllers

import (
	"fmt"

	// "github.com/go-logr/logr"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctrl "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/client"

	// lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"

	"github.com/scottdware/go-bigip"
)

func main() {
	// Establish our session to the BIG-IP
	f5 := bigip.NewSession("f5.company.com", "admin", "secret", nil)

	// Iterate over all the virtual servers, and display their names.
	vservers, err := f5.VirtualServers()
	if err != nil {
		fmt.Println(err)
	}

	for _, vs := range vservers.VirtualServers {
		fmt.Printf("Name: %s\n", vs.Name)
	}
}
