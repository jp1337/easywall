#!/bin/bash

set -e # exit when any command fails

pip3 install --upgrade pydoc-markdown mkdocs

pydoc-markdown

cp build/docs/content/*.md docs
