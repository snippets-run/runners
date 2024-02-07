#!/bin/bash

[[ -z "$1" ]] && echo "Missing snippet name. Try 'run.sh <snippet-name>'" && exit 1

curl -sS --output snippet.sh https://registry.snippets.run/shell/$1
bash snippet.sh