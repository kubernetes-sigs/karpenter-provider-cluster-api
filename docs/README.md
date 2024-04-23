# Documentation Site

This directory contains the documentation for the Karpenter
provider Cluster API project. It is structured as an [mkdocs](https://www.mkdocs.org)
project with the files living in the `docs` subdirectory as 
markdown files.

The easiest method for building the docs site is by using the
included makefile and containerized build process. Typing
`make serve` in this directory will attempt to build and serve the
site locally. This requires a local installation of [podman](https://podman.io),
but will also work with [docker](https://docker.com) by specifying
`CONTAINER_ENGINE=docker make serve`.
