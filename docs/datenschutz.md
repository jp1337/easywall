---
layout: default
title: Datenschutzerklärung
description: Was easywall-project.org, die öffentliche Demo und die freiwillige Installationszählung über dich verarbeiten — und was nicht.
permalink: /datenschutz/
---

<div lang="de" markdown="1">

# Datenschutz

Informationen nach Art. 13 und 14 DSGVO.

## Verantwortlich

<p>Jochen Pylypiw<br>
c/o IP-Management #12226<br>
Ludwig-Erhard-Straße 18<br>
20459 Hamburg</p>

E-Mail: [info@easywall-project.org](mailto:info@easywall-project.org)

## Worum es geht

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

## easywall-project.org

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

## demo.easywall-project.org

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

## Installationszählung

Aus, solange der Betreiber einer easywall-Installation sie nicht einschaltet.
Einmal am Tag sendet die Installation eine zufällige 32-stellige Kennung und
ihre Version an `telemetry.wdkro.de` — denselben Hetzner-Server. Der empfangende
Webserver schreibt Zeit, Kennung und Version, **keine IP-Adresse**, in ein
Protokoll, das 36 Tage aufbewahrt wird; länger bleibt nur eine Zahl je Version.
Die Kennung entsteht auf deinem Server und beschreibt niemanden; wer
`telemetry.json` löscht, bekommt eine neue. Rechtsgrundlage: Einwilligung
(Art. 6 Abs. 1 lit. a DSGVO), widerrufen durch Ausschalten unter *System* — die
nächste Meldung wird dann nicht mehr gesendet.

## E-Mail

Nur, um dir zu antworten. Rechtsgrundlage: Art. 6 Abs. 1 lit. b DSGVO, soweit
es um eine Leistung geht, die du anfragst, sonst unser berechtigtes Interesse,
Anfragen zu beantworten (lit. f). Gespeichert, solange die Anfrage es braucht,
länger nur, wenn ein Gesetz es verlangt.

## Postanschrift über Impressum-Privatschutz

Post an die Anschrift oben nimmt die IMPRESSUM-PRIVATSCHUTZ GmbH,
Ludwig-Erhard-Str. 18, 20459 Hamburg, entgegen und leitet sie weiter; mit ihr
besteht ein Vertrag zur Auftragsverarbeitung. Rechtsgrundlage: berechtigtes
Interesse an einer zuverlässigen Postanschrift (Art. 6 Abs. 1 lit. f).
[Ihre Datenschutzerklärung](https://impressum-privatschutz.de/datenschutzerklaerung/).

## Empfänger

Die oben genannten Dienstleister, und Behörden, wo ein Gesetz uns verpflichtet
(Art. 6 Abs. 1 lit. c). Nichts wird verkauft oder für Werbung weitergegeben. Die
einzige Übermittlung außerhalb der EU ist die zu GitHub, wie beschrieben.

## Deine Rechte

Auskunft (Art. 15 DSGVO), Berichtigung (16), Löschung (17), Einschränkung (18),
Datenübertragbarkeit (20) und **Widerspruch** gegen Verarbeitung auf Grundlage
berechtigten Interesses (21). Eine Einwilligung kannst du jederzeit für die
Zukunft widerrufen (Art. 7 Abs. 3). Schreib an die Anschrift oder E-Mail oben.
Du kannst dich außerdem bei einer Datenschutz-Aufsichtsbehörde beschweren
(Art. 77).

Stand: 2. Oktober 2026. Quelle: Impressum-Privatschutz, angepasst an diese Website.

In English: [Privacy policy]({{ '/privacy/' | relative_url }}).

</div>
