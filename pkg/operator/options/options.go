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

package options

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	karpoptions "sigs.k8s.io/karpenter/pkg/operator/options"
)

func init() {
	karpoptions.Injectables = append(karpoptions.Injectables, &Options{})
}

type optionsKey struct{}

type Options struct {
	ClusterAPIKubeConfigFile string
}

func (o *Options) AddFlags(fs *karpoptions.FlagSet) {
	fs.StringVar(&o.ClusterAPIKubeConfigFile, "cluster-api-kubeconfig", "", "The path to the cluster api manager cluster kubeconfig file.  Defaults to service account credentials if not specified.")
}

func (o *Options) Parse(fs *karpoptions.FlagSet, args ...string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		return fmt.Errorf("parsing flags, %w", err)
	}

	if err := o.Validate(); err != nil {
		return fmt.Errorf("validating options, %w", err)
	}

	return nil
}

func (o *Options) Validate() error {
	return nil
}

func (o *Options) ToContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, optionsKey{}, o)
}

func FromContext(ctx context.Context) *Options {
	retval := ctx.Value(optionsKey{})
	if retval == nil {
		return nil
	}
	return retval.(*Options)
}
