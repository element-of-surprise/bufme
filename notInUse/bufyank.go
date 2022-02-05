package main

// This file is basically yanked from https://github.com/element-of-surprise/buf/blob/main/private/buf/cmd/buf/command/generate/generate.go
// with modifications to eliminate most of the flag stuff.

import (
	"context"

	"github.com/element-of-surprise/buf/private/buf/bufcli"
	"github.com/element-of-surprise/buf/private/buf/buffetch"
	"github.com/element-of-surprise/buf/private/buf/bufgen"
	"github.com/element-of-surprise/buf/private/bufpkg/bufanalysis"
	"github.com/element-of-surprise/buf/private/bufpkg/bufimage"
	"github.com/element-of-surprise/buf/private/pkg/app"
	"github.com/element-of-surprise/buf/private/pkg/app/appflag"
	"github.com/element-of-surprise/buf/private/pkg/command"
	"github.com/element-of-surprise/buf/private/pkg/storage/storageos"
	"go.uber.org/zap"
)

// adpated from flags in this file: https://github.com/element-of-surprise/buf/blob/main/private/buf/cmd/buf/command/generate/generate.go
var (
	disableSymlinks = true
	inputHashing    = ""
	paths           []string
	excludePaths    []string
	includeImports  = false
	includeWKT      = false
	template        = ""
	baseOutDirPath  = "."
	errorFormat     = "text"
	config          = ""
)

/*
func run(logger *zap.Logger){
	builder := appflag.NewBuilder(
		name,
		appflag.BuilderWithTimeout(120*time.Second),
		appflag.BuilderWithTracing(),
	)
	f := builder.NewRunFunc(
		func(ctx context.Context, container appflag.Container) error {
			generate(ctx, logger)
		},
		bufcli.NewErrorInterceptor(),
	}
	return f(ctx, nil)
}
*/

// generate generates the proto files by using the buf CLI routines.
func generate(ctx context.Context, logger *zap.Logger) error {
	base, err := app.NewContainerForOS()
	if err != nil {
		return err
	}
	container, err := appflag.NewContainer(base, "bufme", logger, nil)
	if err != nil {
		return err
	}

	input, err := bufcli.GetInputValue(container, inputHashing, ".")
	if err != nil {
		return err
	}

	ref, err := buffetch.NewRefParser(logger, buffetch.RefParserWithProtoFileRefAllowed()).GetRef(ctx, input)
	if err != nil {
		return err
	}

	storageosProvider := bufcli.NewStorageosProvider(disableSymlinks)
	runner := command.NewRunner()
	readWriteBucket, err := storageosProvider.NewReadWriteBucket(
		".",
		storageos.ReadWriteBucketWithSymlinksIfSupported(),
	)
	if err != nil {
		return err
	}
	genConfig, err := bufgen.ReadConfig(
		ctx,
		bufgen.NewProvider(logger),
		readWriteBucket,
		bufgen.ReadConfigWithOverride(template),
	)
	if err != nil {
		return err
	}
	registryProvider, err := bufcli.NewRegistryProvider(ctx, container)
	if err != nil {
		return err
	}
	imageConfigReader, err := bufcli.NewWireImageConfigReader(
		container,
		storageosProvider,
		runner,
		registryProvider,
	)
	if err != nil {
		return err
	}
	imageConfigs, fileAnnotations, err := imageConfigReader.GetImageConfigs(
		ctx,
		container,
		ref,
		config,
		paths,        // we filter on files
		excludePaths, // we exclude these paths
		false,        // input files must exist
		false,        // we must include source info for generation
	)
	if err != nil {
		return err
	}
	if len(fileAnnotations) > 0 {
		if err := bufanalysis.PrintFileAnnotations(container.Stderr(), fileAnnotations, errorFormat); err != nil {
			return err
		}
		return bufcli.ErrFileAnnotation
	}
	images := make([]bufimage.Image, 0, len(imageConfigs))
	for _, imageConfig := range imageConfigs {
		images = append(images, imageConfig.Image())
	}
	image, err := bufimage.MergeImages(images...)
	if err != nil {
		return err
	}
	generateOptions := []bufgen.GenerateOption{
		bufgen.GenerateWithBaseOutDirPath(baseOutDirPath),
	}
	if includeImports {
		generateOptions = append(
			generateOptions,
			bufgen.GenerateWithIncludeImports(),
		)
	}
	if includeWKT {
		generateOptions = append(
			generateOptions,
			bufgen.GenerateWithIncludeWellKnownTypes(),
		)
	}
	return bufgen.NewGenerator(
		logger,
		storageosProvider,
		runner,
		registryProvider,
	).Generate(
		ctx,
		container,
		genConfig,
		image,
		generateOptions...,
	)
}
