---
layout: default
title: Privacy policy
description: What easywall-project.org, the public demo and the opt-in installation count process about you — and what they do not.
permalink: /privacy/
---

# Privacy policy

Information under Art. 13 and 14 GDPR. Auf Deutsch:
[Datenschutzerklärung]({{ '/datenschutz/' | relative_url }}).

## Who is responsible

<p>Jochen Pylypiw<br>
c/o IP-Management #12226<br>
Ludwig-Erhard-Straße 18<br>
20459 Hamburg, Germany</p>

E-mail: [info@easywall-project.org](mailto:info@easywall-project.org)

## What this covers

- **easywall-project.org**, this documentation: your IP address, at GitHub
- **demo.easywall-project.org**, the public demo: a server log with a shortened IP address
- **Installation count**, opt-in, sent by an easywall installation: a random identifier and the version, never an address
- **E-mail** to the address above: what you write

**easywall installed on your own server is yours.** You are the controller of
everything it processes; nothing reaches us unless you switch the installation
count on. Its other outbound requests (update check, notifications, ACME) go
from your server to the services you configure, not to us — listed in
[Security]({{ '/docs/security/' | relative_url }}).

No page sets a tracking cookie, loads anything from a third party, or uses
an analytics service.

## easywall-project.org

Hosted by **GitHub Pages** — GitHub, Inc., 88 Colin P. Kelly Jr. St., San
Francisco, CA 94107, USA. GitHub states that it logs and stores the IP address
of every visitor to a Pages site *for security purposes*. We do not receive
those logs. GitHub is certified under the EU-U.S. Data Privacy Framework, the
basis for the transfer to the USA (Art. 45 GDPR). Retention is GitHub's: see the
[GitHub Privacy Statement](https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement).
Legal basis: our legitimate interest in publishing the documentation reliably
(Art. 6(1)(f) GDPR).

Fonts and search are served from this site itself. Your light/dark choice is
kept in your browser's local storage and never sent anywhere.

## demo.easywall-project.org

Runs on a server of Hetzner Online GmbH, Industriestr. 25, 91710 Gunzenhausen,
Germany, under a data processing agreement (Art. 28 GDPR).

- **Web server log** — IP address shortened before it is written (IPv4: last
  two bytes removed; IPv6: first 48 bits only), time, host, address requested,
  status, referring page, browser identifier. For delivery and for detecting
  attacks and faults. Kept **up to 15 days** on the server, and copied to a log
  server we operate ourselves on our own hardware in Germany, where it is
  deleted after **one year**.
- **Cookie `easywall`** — your demo session. Expires 10 minutes after last use,
  or when you sign out.
- **Cookies `easywall_pending`, `easywall_passkey`, `easywall_login_passkey`** —
  only while you try the second-factor pages. 1 to 5 minutes.
- **Cookie `easywall_lang`** — only if you switch the language. One year.
- **What you enter** — rules, addresses, settings. Reset every 6 hours.

The demo records **no** IP address in its audit log or its own log, and nothing
you enter reaches a real firewall. The web server writes no error log for the
demo. These cookies are strictly necessary for what you ask the demo to do
(§ 25(2) no. 2 TDDDG); none of them tracks you. Legal basis: legitimate interest in
showing the software and running it securely (Art. 6(1)(f) GDPR).

## Installation count

Off unless the operator of an easywall installation switches it on. Once a
day the installation sends a random 32-character identifier and its version to
`telemetry.wdkro.de` — the same Hetzner server. The receiving web server writes
time, identifier and version, and **no IP address**, to a log kept for 36 days;
what stays longer is a count per version. The identifier is generated on your
server and describes nobody; deleting `telemetry.json` replaces it.
Legal basis: consent (Art. 6(1)(a) GDPR), withdrawn by switching the count off
under *System* — the next report is then not sent.
[What is sent, exactly]({{ '/docs/configuration/' | relative_url }}).

## E-mail

Used only to answer you. Legal basis: Art. 6(1)(b) GDPR where you ask about
something we would do for you, otherwise our legitimate interest in answering
(Art. 6(1)(f)). Kept as long as the conversation needs it, longer only where the
law requires.

## Postal address via Impressum-Privatschutz

Post for the address above is received and forwarded by IMPRESSUM-PRIVATSCHUTZ
GmbH, Ludwig-Erhard-Str. 18, 20459 Hamburg, under a data processing agreement.
Legal basis: legitimate interest in a reliable postal address (Art. 6(1)(f)).
[Their privacy policy](https://impressum-privatschutz.de/datenschutzerklaerung/).

## Recipients

The service providers named above, and public authorities where the law obliges
us (Art. 6(1)(c)). Nothing is sold or shared for advertising. The only transfer
outside the EU is GitHub's, as described.

## Your rights

Access (Art. 15 GDPR), rectification (16), erasure (17), restriction (18), data
portability (20), and **objection** to processing based on legitimate interest
(21). Consent can be withdrawn at any time with effect for the future (Art. 7(3)).
Write to the address or e-mail above. You may also complain to a data protection
supervisory authority (Art. 77).

As of 2 October 2026. Source: Impressum-Privatschutz, adapted to this site.

