package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Colvin-Y/kubernetes-ontology/internal/owl"
)

func main() {
	output := flag.String("output", "", "write OWL RDF/XML to this file instead of stdout")
	flag.Parse()

	var writer io.Writer = os.Stdout
	var file *os.File
	if *output != "" {
		var err error
		file, err = os.Create(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create output: %v\n", err)
			os.Exit(1)
		}
		writer = file
	}

	if err := owl.WriteStaticOntology(writer); err != nil {
		if file != nil {
			_ = file.Close()
		}
		fmt.Fprintf(os.Stderr, "write ontology: %v\n", err)
		os.Exit(1)
	}
	if file != nil {
		if err := file.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close output: %v\n", err)
			os.Exit(1)
		}
	}
}
