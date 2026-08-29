---
layout: default
title: License
description: GPL-3.0, and what it asks of you — which is nothing at all until you hand easywall to somebody else.
---

# License

easywall is under the **GNU General Public License, version 3**. The full text
is [in the repository](https://github.com/jp1337/easywall/blob/main/LICENSE) and
at [gnu.org](https://www.gnu.org/licenses/gpl-3.0.html). This page is what it
means in practice; it is not legal advice, and where the two disagree the
licence is what counts.

## Running it

| | |
|---|---|
| On your own machine | Yes. No conditions at all |
| At work, on company hardware | Yes. Commercial use is use |
| On a server other people reach | Yes. GPL-3.0 is not the AGPL — serving something over a network is not distribution |
| Modified, for yourself | Yes. A change you keep is yours and nobody has to see it |
| Paying nothing, telling nobody | Yes |

Nothing on that list creates an obligation. That is the whole of it for almost
everybody who reads this page.

## Handing it to somebody else

Obligations begin at **distribution** — a binary, a package, a container image,
a modified source tree, anything that leaves your hands.

| You give them | You also owe them |
|---|---|
| An unmodified binary or package | The corresponding source, and this licence with it |
| A build with your changes in it | The same, including your changes, under GPL-3.0 |
| A container image with easywall in it | The same. An image is a distribution of what is inside it |

And in every case the copyright notices and the licence text stay where they
are. You may charge for the distribution itself; the source that comes with it
is not something you get to withhold or sell separately.

> **A network service is not distribution.** Running a modified easywall for
> other people to use over HTTP does not oblige you to publish anything. That
> obligation is the AGPL's, and easywall is not under it.

## Contributing

A pull request is contributed under GPL-3.0, the same as the rest. There is no
contributor licence agreement and nothing to sign — see
[Contributing]({{ '/docs/contributing/' | relative_url }}).

## Why this licence

easywall puts a web interface in front of a machine's packet filter. Anyone
running it is trusting the code with the one thing standing between their host
and the network. A licence that lets a modified copy be handed on with the
modification hidden is the wrong licence for that. GPL-3.0 means the version
somebody hands you is a version you can read.
