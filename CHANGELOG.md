# Changelog

## 0.0.4 (2019-10-04)

Features:

- added possibility to apply custom IPTables rules
- full implemented webinterface - old PHP sources are history
- rule changes made in the webinterface are only written temporary into web directory
- rules can be applied in the webinterface
- a lot of code improvements
- this is kind the first "stable" version ready for testing
- I will test this on my webserver a lot, so the next versions will be more stable

Bugfixes:

- too many, I can't count them
- there was a long time since the last version

## 0.0.3 (2019-06-30)

Features:

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

Bugfixes:

- improved exception handling in several files
- the `.running` file was not deleted properly
- moved the system `os.system` to a single function where security checks can be implemented in the future

## 0.0.2 (2019-06-08)

Features:

- Changed branch master to old python branch
- Renamed old master branch to php-old
- Bumped version
- Changed documentation

Bugfixes:

- Information of the user in install.sh if not running as root or using sudo
- Removed quiet option in install.sh for apt-get and pip3 for better user experience

## 0.0.1 (2019-04-24)

Features:

- Incomplete Rework of Branch php-old
- easywall is split in two parts in the new concept
- easywall Firewall Core Part running as root user finished
- The New easywall will be one part running as root and one part running as easywall user which has access to config files.
