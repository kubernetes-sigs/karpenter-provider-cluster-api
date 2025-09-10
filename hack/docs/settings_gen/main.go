/*
Copyright 2024 The Kubernetes Authors.

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
	"log"
	"os"
	"strings"

	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/operator/options"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: go run main.go <path/to/settings.md>")
	}
	outputFileName := os.Args[1]

	title := "# Settings\n\n"
	comment := "[comment]: <> (the content below is generated from hack/docs/settings_gen/main.go)\n\n"
	description := "Karpenter exposes environment variables and CLI flags that allow you to configure controller behavior. The available settings are outlined below.\n\n"

	fs := &coreoptions.FlagSet{
		FlagSet: flag.NewFlagSet("karpenter", flag.ContinueOnError),
	}
	(&coreoptions.Options{}).AddFlags(fs)
	(&options.Options{}).AddFlags(fs)

	envVarsTable := "| Environment Variable | CLI Flag | Description |\n"
	envVarsTable += "|--|--|--|\n"
	fs.VisitAll(func(f *flag.Flag) {
		if f.DefValue == "" {
			envVarsTable += fmt.Sprintf("| %s | %s | %s|\n", strings.ReplaceAll(strings.ToUpper(f.Name), "-", "_"), "\\-\\-"+f.Name, f.Usage)
		} else {
			envVarsTable += fmt.Sprintf("| %s | %s | %s (default = %s)|\n", strings.ReplaceAll(strings.ToUpper(f.Name), "-", "_"), "\\-\\-"+f.Name, f.Usage, f.DefValue)
		}
	})

	log.Println("writing output to", outputFileName)
	err := os.WriteFile(outputFileName, []byte(title+comment+description+envVarsTable), 0644)
	if err != nil {
		log.Fatalf("failed to write file %s, %s", outputFileName, err)
	}
}
