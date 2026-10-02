---
layout: default
title: Privacy policy
description: What easywall-project.org, the public demo and the opt-in installation count process about you — and what they do not. Deutsche Fassung inklusive.
permalink: /privacy/
---

# Privacy policy

Information under Art. 13 and 14 GDPR. Datenschutzerklärung auf Deutsch:
[Deutsche Fassung](#deutsche-fassung).

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

---

## Deutsche Fassung

Datenschutzerklärung — Informationen nach Art. 13 und 14 DSGVO.

### Verantwortlich

<p>Jochen Pylypiw<br>
c/o IP-Management #12226<br>
Ludwig-Erhard-Straße 18<br>
20459 Hamburg</p>

E-Mail: [info@easywall-project.org](mailto:info@easywall-project.org)

### Worum es geht

- **easywall-project.org**, diese Dokumentation: deine IP-Adresse, bei GitHub
- **demo.easywall-project.org**, die öffentliche Demo: ein Server-Protokoll mit gekürzter IP-Adresse
- **Installationszählung**, freiwillig, gesendet von einer easywall-Installation: eine Zufallskennung und die Version, nie eine Adresse
- **E-Mail** an die Adresse oben: was du schreibst

**easywall auf deinem eigenen Server gehört dir.** Für alles, was es
verarbeitet, bist du verantwortlich; zu uns gelangt nichts, solange du die
Installationszählung nicht einschaltest. Seine übrigen ausgehenden Anfragen
(Update-Prüfung, Benachrichtigungen, ACME) gehen von deinem Server an die
Dienste, die du einstellst, nicht an uns — aufgelistet unter
[Security]({{ '/docs/security/' | relative_url }}).

Keine Seite setzt ein Tracking-Cookie, lädt etwas von Dritten oder nutzt einen
Analysedienst.

### easywall-project.org

Ausgeliefert von **GitHub Pages** — GitHub, Inc., 88 Colin P. Kelly Jr. St., San
Francisco, CA 94107, USA. GitHub protokolliert und speichert nach eigener Angabe
die IP-Adresse jedes Besuchers einer Pages-Seite *zu Sicherheitszwecken*. Diese
Protokolle erhalten wir nicht. GitHub ist nach dem EU-US Data Privacy Framework
zertifiziert; darauf beruht die Übermittlung in die USA (Art. 45 DSGVO). Die
Speicherdauer bestimmt GitHub, siehe
[GitHub Privacy Statement](https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement).
Rechtsgrundlage: unser berechtigtes Interesse, die Dokumentation zuverlässig zu
veröffentlichen (Art. 6 Abs. 1 lit. f DSGVO).

Schriften und Suche kommen von dieser Website selbst. Deine Wahl zwischen hell
und dunkel liegt im lokalen Speicher deines Browsers und wird nirgendwohin
gesendet.

### demo.easywall-project.org

Läuft auf einem Server der Hetzner Online GmbH, Industriestr. 25, 91710
Gunzenhausen, mit Vertrag zur Auftragsverarbeitung (Art. 28 DSGVO).

- **Webserver-Protokoll** — IP-Adresse, gekürzt bevor sie geschrieben wird
  (IPv4: letzte zwei Byte entfernt; IPv6: nur die ersten 48 Bit), Zeit, Host,
  aufgerufene Adresse, Status, verweisende Seite, Browserkennung. Für die
  Auslieferung und um Angriffe und Fehler zu erkennen. **Bis zu 15 Tage** auf dem
  Server, dazu eine Kopie auf einem von uns selbst betriebenen Protokollserver auf
  eigener Hardware in Deutschland, dort nach **einem Jahr** gelöscht.
- **Cookie `easywall`** — deine Demo-Sitzung. Läuft 10 Minuten nach der letzten
  Nutzung ab, oder wenn du dich abmeldest.
- **Cookies `easywall_pending`, `easywall_passkey`, `easywall_login_passkey`** —
  nur während du die Seiten zum zweiten Faktor ausprobierst. 1 bis 5 Minuten.
- **Cookie `easywall_lang`** — nur wenn du die Sprache wechselst. Ein Jahr.
- **Was du eingibst** — Regeln, Adressen, Einstellungen. Alle 6 Stunden
  zurückgesetzt.

Die Demo schreibt **keine** IP-Adresse in ihr Audit-Log oder ihr eigenes
Protokoll, und nichts, was du eingibst, erreicht eine echte Firewall. Ein
Fehlerprotokoll schreibt der Webserver für die Demo nicht. Diese Cookies sind für
das, was du von der Demo verlangst, unbedingt erforderlich (§ 25 Abs. 2 Nr. 2
TDDDG); keines davon verfolgt dich. Rechtsgrundlage: berechtigtes Interesse, die
Software zu zeigen und sicher zu betreiben (Art. 6 Abs. 1 lit. f DSGVO).

### Installationszählung

Aus, solange der Betreiber einer easywall-Installation sie nicht einschaltet.
Einmal am Tag sendet die Installation eine zufällige 32-stellige Kennung und
ihre Version an `telemetry.wdkro.de` — denselben Hetzner-Server. Der empfangende
Webserver schreibt Zeit, Kennung und Version, **keine IP-Adresse**, in ein
Protokoll, das 36 Tage aufbewahrt wird; länger bleibt nur eine Zahl je Version.
Die Kennung entsteht auf deinem Server und beschreibt niemanden; wer
`telemetry.json` löscht, bekommt eine neue. Rechtsgrundlage: Einwilligung
(Art. 6 Abs. 1 lit. a DSGVO), widerrufen durch Ausschalten unter *System* — die
nächste Meldung wird dann nicht mehr gesendet.

### E-Mail

Nur, um dir zu antworten. Rechtsgrundlage: Art. 6 Abs. 1 lit. b DSGVO, soweit
es um eine Leistung geht, die du anfragst, sonst unser berechtigtes Interesse,
Anfragen zu beantworten (lit. f). Gespeichert, solange die Anfrage es braucht,
länger nur, wenn ein Gesetz es verlangt.

### Postanschrift über Impressum-Privatschutz

Post an die Anschrift oben nimmt die IMPRESSUM-PRIVATSCHUTZ GmbH,
Ludwig-Erhard-Str. 18, 20459 Hamburg, entgegen und leitet sie weiter; mit ihr
besteht ein Vertrag zur Auftragsverarbeitung. Rechtsgrundlage: berechtigtes
Interesse an einer zuverlässigen Postanschrift (Art. 6 Abs. 1 lit. f).
[Ihre Datenschutzerklärung](https://impressum-privatschutz.de/datenschutzerklaerung/).

### Empfänger

Die oben genannten Dienstleister, und Behörden, wo ein Gesetz uns verpflichtet
(Art. 6 Abs. 1 lit. c). Nichts wird verkauft oder für Werbung weitergegeben. Die
einzige Übermittlung außerhalb der EU ist die zu GitHub, wie beschrieben.

### Deine Rechte

Auskunft (Art. 15 DSGVO), Berichtigung (16), Löschung (17), Einschränkung (18),
Datenübertragbarkeit (20) und **Widerspruch** gegen Verarbeitung auf Grundlage
berechtigten Interesses (21). Eine Einwilligung kannst du jederzeit für die
Zukunft widerrufen (Art. 7 Abs. 3). Schreib an die Anschrift oder E-Mail oben.
Du kannst dich außerdem bei einer Datenschutz-Aufsichtsbehörde beschweren
(Art. 77).

Stand: 2. Oktober 2026. Quelle: Impressum-Privatschutz, angepasst an diese Website.
