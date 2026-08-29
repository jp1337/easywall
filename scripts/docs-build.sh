#!/usr/bin/env bash
# Builds docs/_site and its search index.
#
# There is no ruby, gem, bundler or jekyll on this machine and there will not
# be — Bazzite's root filesystem is immutable — so the Jekyll half runs in a
# container. Rootless podman maps container root to the invoking user, so what
# lands in docs/_site is owned by you and not by root.
#
# The gem cache is a named volume. Without it every run re-resolves the Gemfile
# from rubygems.org, which is forty seconds of nothing.
#
# JEKYLL_ENV=production matches both jobs in .github/workflows/docs.yml. A
# development build differs — jekyll-seo-tag emits different tags — and a check
# against it proves something about a site nobody ships.
set -euo pipefail
cd "$(dirname "$0")/.."

podman run --rm \
  -v "$PWD/docs:/srv:Z" \
  -v easywall-docs-gems:/usr/local/bundle \
  -w /srv \
  docker.io/library/ruby:3.4 \
  sh -c 'bundle install --quiet && JEKYLL_ENV=production bundle exec jekyll build'

# The exact version .github/actions/build-search-index pins, with the exact same
# three flags. A floating range, or one flag out of step, means the index these
# checks run against is not the index easywall-project.org serves.
npx --yes pagefind@1.5.2 --site docs/_site \
  --glob "docs/**/*.html" \
  --root-selector "main.content" \
  --force-language en
