package options

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
)

func init() {
	coreoptions.Injectables = append(coreoptions.Injectables, &Options{})
}

type optionsKey struct{}

type Options struct {
	ClusterAPIKubeConfigFile string
}

func (o *Options) AddFlags(fs *coreoptions.FlagSet) {
	fs.StringVar(&o.ClusterAPIKubeConfigFile, "cluster-api-kubeconfig", "", "The path to the cluster api manager cluster kubeconfig file.  Empty string for in cluster config.")
}

func (o *Options) Parse(fs *coreoptions.FlagSet, args ...string) error {
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
