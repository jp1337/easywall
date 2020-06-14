# for usage see https://github.com/marketplace/actions/run-markdownlint-mdl
all

# Don't enforce line length
exclude_rule 'MD013'

# Don't force ordered lists with 1. 1. 1.
rule 'MD029', :style => :ordered
