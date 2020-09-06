# for usage see https://github.com/marketplace/actions/run-markdownlint-mdl
all

# Don't enforce line length
exclude_rule 'MD013'

# exclude "Ordered list item prefix"
exclude_rule 'MD029'

# exclude "Multiple headers with the same content"
exclude_rule 'MD024'

# Allow inline HTML
exclude_rule 'MD033'

# Allow heading increase
exclude_rule 'MD001'

# Allow "Multiple top level headers in the same document"
exclude_rule 'MD025'