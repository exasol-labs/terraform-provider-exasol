package main

import (
	"context"
	"flag"
	"log"
	"os"

	"terraform-provider-exasol/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version = "0.2.0"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "start provider in debug mode")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/exasol/exasol",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Printf("provider Serve failed: %v", err)
		os.Exit(1)
	}
}
