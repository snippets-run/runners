#!/bin/bash

[[ -z "$1" ]] && echo "Missing snippet name. Try 'run.sh <snippet-name>'" && exit 1

is_supported()
{
  command -v $1 >/dev/null 2>&1
}

curl -sS --output $TMPDIR/snippet.sh https://registry.snippets.run/s/shell/$1
bash $TMPDIR/snippet.sh
