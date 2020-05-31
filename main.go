package main

import (
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cmd"

	"fmt"
)

func main()  {
	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(cmd.NewLogger()),
	)
	fmt.Println(provider)
	clusters, err := provider.List()
	if err != nil {
		fmt.Println("error")
	}
	if len(clusters) == 0 {
		fmt.Println("no cluster")
	}
	fmt.Println(clusters)
	//create := cluster.CreateWithConfigFile("/home/raj/go/src/github.com/prometheus/test-infra/config.yml")
	//fmt.Println(create)
	//create_1 := provider.Create("kind-1", create)
	//fmt.Println(create_1)

}