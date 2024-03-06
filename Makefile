HUGO_CMD = go run ${PWD}/vendor/github.com/gohugoio/hugo
date := $(shell date --iso-8601)
NEW_CONTENT_NAME ?= new-docs-page-${date}.md

docs-build:
	pushd docs && ${HUGO_CMD}

docs-new-content:
	pushd docs && ${HUGO_CMD} new content ${NEW_CONTENT_NAME}

docs-serve:
	pushd docs && ${HUGO_CMD} server -D
