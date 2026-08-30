package main

import (
	"fmt"
	"os"

	"github.com/nintechio/depprism"
)

func main() {
	before := []byte(`{"name":"demo","lockfileVersion":3,"packages":{}}`)
	after := []byte(`{"name":"demo","lockfileVersion":3,"packages":{"node_modules/tiny":{"version":"1.0.0","resolved":"https://registry.npmjs.org/tiny/-/tiny-1.0.0.tgz","integrity":"sha512-demo"}}}`)

	oldGraph, err := depprism.Parse("package-lock.json", before, depprism.ParseOptions{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	newGraph, err := depprism.Parse("package-lock.json", after, depprism.ParseOptions{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	report, err := depprism.Compare(oldGraph, newGraph, depprism.DefaultPolicy())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	review := depprism.NewReview()
	review.Add(report)
	if err := depprism.Render(os.Stdout, review, depprism.FormatJSON); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
