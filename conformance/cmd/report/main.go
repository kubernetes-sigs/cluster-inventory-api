/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"

	"sigs.k8s.io/cluster-inventory-api/conformance"
)

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: report validate <report.yaml>... | report matrix --reports-dir <directory>")
	}
	switch args[0] {
	case "validate":
		if len(args) == 1 {
			return fmt.Errorf("validate requires at least one report")
		}
		for _, path := range args[1:] {
			r, err := conformance.ReadReport(path)
			if err != nil {
				return err
			}
			if err = conformance.ValidateReport(r, true); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			if _, err := fmt.Fprintf(os.Stdout, "Valid: %s (%s)\n", path, r.Result); err != nil {
				return err
			}
		}
		return nil
	case "matrix":
		fs := flag.NewFlagSet("matrix", flag.ContinueOnError)
		dir := fs.String("reports-dir", "", "directory of accepted YAML reports")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *dir == "" || fs.NArg() != 0 {
			return fmt.Errorf("matrix requires --reports-dir and no positional arguments")
		}
		return conformance.GenerateMatrix(*dir, os.Stdout)
	default:
		return fmt.Errorf("unknown report command %q", args[0])
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
