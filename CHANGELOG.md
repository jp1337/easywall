# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2020-06-21

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.0.4...v0.1.0)

### Added

- This version is almost completely tested by unit tests.
- The documentation was completely revised and can now be found in the `docs` folder.
- The configuration has been shortened and simplified.
- The installation, uninstallation and an update can now be carried out via scripts.
- The web interface installation now creates self-signed SSL certificates and can only be used over HTTPS.

## [0.0.4] - 2019-10-04

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.0.3...v0.0.4)

### Added

- added possibility to apply custom IPTables rules
- full implemented webinterface - old PHP sources are history
- rule changes made in the webinterface are only written temporary into web directory
- rules can be applied in the webinterface
- a lot of code improvements
- this is kind the first "stable" version ready for testing
- I will test this on my webserver a lot, so the next versions will be more stable

### Changed

- too many, I can't count them
- there was a long time since the last version

## [0.0.3] - 2019-06-30

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.0.2...v0.0.3)

### Added

- added easywall-Web using flask
- added old php templates to web
- improved install script a lot and added so many features to it
- simplified code using codacy and code climate
- ICMP Support added after testing on a server of mine
- added a daemon script for running easywall-Web
- 404 error page added to web
- for a production use of easywall-Web I added uwsgi instead of the small development server of flask
- logout button added to web
- added a password generator script and added it to install script

### Changed

- improved exception handling in several files
- the `.running` file was not deleted properly
- moved the system `os.system` to a single function where security checks can be implemented in the future

## [0.0.2] - 2019-06-08

[Full Changelog](https://github.com/jp1337/easywall/compare/v0.0.1...v0.0.2)

### Added

- Changed branch master to old python branch
- Renamed old master branch to php-old
- Bumped version
- Changed documentation

### Changed

- Information of the user in install.sh if not running as root or using sudo
- Removed quiet option in install.sh for apt-get and pip3 for better user experience

## [0.0.1] - 2019-04-24

### Added

- Incomplete Rework of Branch php-old
- easywall is split in two parts in the new concept
- easywall Firewall Core Part running as root user finished
- The New easywall will be one part running as root and one part running as easywall user which has access to config files.

[unreleased]: https://github.com/jp1337/easywall/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/jp1337/easywall/compare/v0.4.0...v0.1.0
[0.0.4]: https://github.com/jp1337/easywall/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/jp1337/easywall/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/jp1337/easywall/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/jp1337/easywall/releases/tag/v0.0.1
