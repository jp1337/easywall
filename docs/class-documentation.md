<a name="easywall"></a>
# easywall

TODO: Doku.

<a name="easywall.acceptance"></a>
# easywall.acceptance

this module exports a class for checking the user acceptance of a process using a acceptance file.

<a name="easywall.acceptance.Acceptance"></a>
## Acceptance Objects

```python
class Acceptance()
```

the Acceptance class exports functions to check the user acceptance.

the functions have to be executed in the following order:
1. init class
2. execute "start"
3. execute "wait"
4. execute "status"

the functions can be executed as often as you want
since they check the internal status of the class

<a name="easywall.acceptance.Acceptance.__init__"></a>
#### \_\_init\_\_

```python
 | __init__(cfg: Config) -> None
```

TODO: Doku.

<a name="easywall.acceptance.Acceptance.start"></a>
#### start

```python
 | start() -> None
```

the start of the acceptance process is triggered by this function
the function checks the internal status of the class.
the internal status can be ready, accepted or not accepted.
if the status is disabled the function does nothing

<a name="easywall.acceptance.Acceptance.wait"></a>
#### wait

```python
 | wait() -> None
```

this function executes a sleep for the configured duration.
the sleep is only executed when the start function was triggered before
and not if the status is disabled.

<a name="easywall.acceptance.Acceptance.status"></a>
#### status

```python
 | status() -> str
```

this function returns the current status of the acceptance process.
this is useful for calls of external software.
when the status is waited the file content is read and the final acceptance status is
determined here. the temporary file is also deleted in this function.

possible status values:
- ready
- disabled
- started
- waiting
- waited
- accepted
- not accepted

<a name="easywall.acceptance.Acceptance.set_status"></a>
#### set\_status

```python
 | set_status(status: str) -> None
```

TODO: Doku.

<a name="easywall.config"></a>
# easywall.config

This module exports a generic class for configuration.

[Classes] Config

<a name="easywall.config.Config"></a>
## Config Objects

```python
class Config()
```

This class is a generic class for configuration.

It is a wrapper around the default configparser and contains basic functionality.

[Methods]
get_value: retrieve a value from a config file
set_value: set a value in the configuration and write the config file to disk
get_sections: get a list of all possible config sections

[Raises]
FileNotFoundError: When the configuration file was not found a exception is thrown.
Exception: when the configparser failed to read the config file a exception is thrown.

<a name="easywall.config.Config.__init__"></a>
#### \_\_init\_\_

```python
 | __init__(config_file_path: str) -> None
```

TODO: Doku.

<a name="easywall.config.Config.read_config_file"></a>
#### read\_config\_file

```python
 | read_config_file() -> None
```

TODO: Doku.

<a name="easywall.config.Config.get_value"></a>
#### get\_value

```python
 | get_value(section: str, key: str) -> Union[bool, int, float, str]
```

Return a value from a given section of the configuration.

[Data Types] String, Float, Integer, Boolean

<a name="easywall.config.Config.set_value"></a>
#### set\_value

```python
 | set_value(section: str, key: str, value: str) -> bool
```

Write a key, value pair into memory configuration and writes it to config file.

[Data Types] bool

<a name="easywall.config.Config.get_sections"></a>
#### get\_sections

```python
 | get_sections() -> list
```

Return a list of the configuration section names/keys.

[WARNING] The name [DEFAULT] is excluded here if used!

[Data Types] list

<a name="easywall.config.Config.get_keys"></a>
#### get\_keys

```python
 | get_keys(section: str) -> AbstractSet[str]
```

TODO: Doku.

<a name="easywall.easywall"></a>
# easywall.easywall

TODO: Doku.

<a name="easywall.easywall.Easywall"></a>
## Easywall Objects

```python
class Easywall()
```

the class contains the main functions for the easywall core
such as applying a new configuration or listening on rule file changes

<a name="easywall.easywall.Easywall.__init__"></a>
#### \_\_init\_\_

```python
 | __init__(config: Config) -> None
```

TODO: Doku.

<a name="easywall.easywall.Easywall.apply"></a>
#### apply

```python
 | apply() -> None
```

TODO: Doku.

<a name="easywall.easywall.Easywall.apply_iptables"></a>
#### apply\_iptables

```python
 | apply_iptables() -> None
```

TODO: Doku.

<a name="easywall.easywall.Easywall.apply_forwarding"></a>
#### apply\_forwarding

```python
 | apply_forwarding() -> None
```

TODO: Doku.

<a name="easywall.easywall.Easywall.apply_ssh_brute"></a>
#### apply\_ssh\_brute

```python
 | apply_ssh_brute() -> None
```

TODO: Doku.

<a name="easywall.easywall.Easywall.apply_invalid_packets_drop"></a>
#### apply\_invalid\_packets\_drop

```python
 | apply_invalid_packets_drop() -> None
```

TODO: Doku.

<a name="easywall.easywall.Easywall.apply_port_scan_prevention"></a>
#### apply\_port\_scan\_prevention

```python
 | apply_port_scan_prevention() -> None
```

TODO: Doku.

<a name="easywall.easywall.Easywall.apply_icmp_flood"></a>
#### apply\_icmp\_flood

```python
 | apply_icmp_flood() -> None
```

TODO: Doku.

<a name="easywall.easywall.Easywall.apply_icmp"></a>
#### apply\_icmp

```python
 | apply_icmp() -> None
```

this function adds rules to iptables for incoming ICMP requests

<a name="easywall.easywall.Easywall.apply_cast"></a>
#### apply\_cast

```python
 | apply_cast() -> None
```

TODO: Doku.

<a name="easywall.easywall.Easywall.apply_blacklist"></a>
#### apply\_blacklist

```python
 | apply_blacklist() -> None
```

this function adds rules to iptables which block incoming traffic
from a list of ip addresses

<a name="easywall.easywall.Easywall.apply_whitelist"></a>
#### apply\_whitelist

```python
 | apply_whitelist() -> None
```

this function adds rules to iptables which explicitly accepts a connection
from this list ip addresses

<a name="easywall.easywall.Easywall.apply_rules"></a>
#### apply\_rules

```python
 | apply_rules(ruletype: str) -> None
```

this function adds rules for incoming tcp and udp connections to iptables
which accept a connection to this list of ports

[INFO] the function also processes port ranges split by ":" separator.

<a name="easywall.easywall.Easywall.apply_custom_rules"></a>
#### apply\_custom\_rules

```python
 | apply_custom_rules() -> None
```

TODO: Doku.

<a name="easywall.easywall.Easywall.rotate_backup"></a>
#### rotate\_backup

```python
 | rotate_backup() -> None
```

TODO: Doku.

<a name="easywall.easywall.Easywall.rename_backup_file"></a>
#### rename\_backup\_file

```python
 | rename_backup_file() -> None
```

TODO: Doku.

<a name="easywall.iptables_handler"></a>
# easywall.iptables\_handler

TODO: Doku.

<a name="easywall.iptables_handler.Target"></a>
## Target Objects

```python
class Target(Enum)
```

TODO: Doku.

<a name="easywall.iptables_handler.Chain"></a>
## Chain Objects

```python
class Chain(Enum)
```

TODO: Doku.

<a name="easywall.iptables_handler.Iptables"></a>
## Iptables Objects

```python
class Iptables()
```

TODO: Doku.

<a name="easywall.iptables_handler.Iptables.__init__"></a>
#### \_\_init\_\_

```python
 | __init__(cfg: Config) -> None
```

TODO: Doku.

<a name="easywall.iptables_handler.Iptables.add_policy"></a>
#### add\_policy

```python
 | add_policy(chain: Chain, target: Target) -> None
```

Create a new policy in iptables firewall by using the os command.

<a name="easywall.iptables_handler.Iptables.add_chain"></a>
#### add\_chain

```python
 | add_chain(chain: str) -> None
```

Create a new custom chain in iptables.

<a name="easywall.iptables_handler.Iptables.add_append"></a>
#### add\_append

```python
 | add_append(chain: Chain, rule: str, onlyv6: bool = False, onlyv4: bool = False, table: str = "") -> None
```

Create a new append in iptables.

<a name="easywall.iptables_handler.Iptables.insert"></a>
#### insert

```python
 | insert(chain: Chain, rule: str, onlyv6: bool = False, onlyv4: bool = False, table: str = "") -> None
```

TODO: Doku.

<a name="easywall.iptables_handler.Iptables.add_custom"></a>
#### add\_custom

```python
 | add_custom(rule: str) -> None
```

TODO: Doku.

<a name="easywall.iptables_handler.Iptables.flush"></a>
#### flush

```python
 | flush(chain: str = "", table: str = "") -> None
```

Flush chain or all chains in iptables firewall.

<a name="easywall.iptables_handler.Iptables.delete_chain"></a>
#### delete\_chain

```python
 | delete_chain(chain: str = "") -> None
```

Delete a chain or all chains in iptables firewall.

<a name="easywall.iptables_handler.Iptables.reset"></a>
#### reset

```python
 | reset() -> None
```

Reset iptables and allows all connections to the system and from the system.

<a name="easywall.iptables_handler.Iptables.status"></a>
#### status

```python
 | status() -> str
```

List the iptables configuration as string.

[WARNING] this is not machine readable!

<a name="easywall.iptables_handler.Iptables.save"></a>
#### save

```python
 | save() -> None
```

Save the current iptables state into a file.

<a name="easywall.iptables_handler.Iptables.restore"></a>
#### restore

```python
 | restore() -> None
```

Restore a backup of a previously saved backup.

<a name="easywall.log"></a>
# easywall.log

Wrapper around the logging module.

It supports the simple configuration of the outputs.

<a name="easywall.log.Log"></a>
## Log Objects

```python
class Log()
```

This class is the main class of the log module.

All logging information is required as inputs.

<a name="easywall.log.Log.__init__"></a>
#### \_\_init\_\_

```python
 | __init__(loglevel: str, to_stdout: bool, to_files: bool, logpath: str, logfile: str)
```

TODO: Docu.

<a name="easywall.log.Log.close_logging"></a>
#### close\_logging

```python
 | close_logging() -> None
```

Gently closes all handlers before exiting the software.

<a name="easywall.log.Log.correct_level"></a>
#### correct\_level

```python
 | correct_level(loglevel: str) -> int
```

Determine the loglevel of the logging class.

<a name="easywall.rules_handler"></a>
# easywall.rules\_handler

TODO: Doku.

<a name="easywall.rules_handler.RulesHandler"></a>
## RulesHandler Objects

```python
class RulesHandler()
```

TODO: Doku.

<a name="easywall.rules_handler.RulesHandler.get_current_rules"></a>
#### get\_current\_rules

```python
 | get_current_rules(ruletype: str) -> List[str]
```

TODO: Doku.

<a name="easywall.rules_handler.RulesHandler.get_new_rules"></a>
#### get\_new\_rules

```python
 | get_new_rules(ruletype: str) -> List[str]
```

TODO: Doku.

<a name="easywall.rules_handler.RulesHandler.get_rules_for_web"></a>
#### get\_rules\_for\_web

```python
 | get_rules_for_web(ruletype: str) -> List[str]
```

TODO: Doku.

<a name="easywall.rules_handler.RulesHandler.backup_current_rules"></a>
#### backup\_current\_rules

```python
 | backup_current_rules() -> None
```

TODO: Doku.

<a name="easywall.rules_handler.RulesHandler.apply_new_rules"></a>
#### apply\_new\_rules

```python
 | apply_new_rules() -> None
```

TODO: Doku.

<a name="easywall.rules_handler.RulesHandler.rollback_from_backup"></a>
#### rollback\_from\_backup

```python
 | rollback_from_backup() -> None
```

TODO: Doku.

<a name="easywall.rules_handler.RulesHandler.copy_rules"></a>
#### copy\_rules

```python
 | copy_rules(source: str, dest: str) -> None
```

TODO: Doku.

<a name="easywall.rules_handler.RulesHandler.ensure_files_exist"></a>
#### ensure\_files\_exist

```python
 | ensure_files_exist() -> None
```

TODO: Doku.

<a name="easywall.rules_handler.RulesHandler.diff_new_current"></a>
#### diff\_new\_current

```python
 | diff_new_current(ruletype: str) -> bool
```

TODO: Doku

True = There are differences between new and current
False = There are no differences between new and current

<a name="easywall.rules_handler.RulesHandler.save_new_rules"></a>
#### save\_new\_rules

```python
 | save_new_rules(ruletype: str, rules: list) -> None
```

TODO: Doku.

<a name="easywall.utility"></a>
# easywall.utility

TODO: Doku.

<a name="easywall.utility.create_folder_if_not_exists"></a>
#### create\_folder\_if\_not\_exists

```python
create_folder_if_not_exists(filepath: str) -> bool
```

Check if a folder exists and creates if it does not exist.

<a name="easywall.utility.create_file_if_not_exists"></a>
#### create\_file\_if\_not\_exists

```python
create_file_if_not_exists(fullpath: str) -> bool
```

Create a file if it does not already exist.

<a name="easywall.utility.delete_file_if_exists"></a>
#### delete\_file\_if\_exists

```python
delete_file_if_exists(fullpath: str) -> bool
```

Check if a file exists in a directory and deletes it if it exists.

<a name="easywall.utility.delete_folder_if_exists"></a>
#### delete\_folder\_if\_exists

```python
delete_folder_if_exists(fullpath: str) -> bool
```

Check if a folder exists and deletes it afterwards.

<a name="easywall.utility.file_get_contents"></a>
#### file\_get\_contents

```python
file_get_contents(filepath: str) -> str
```

Read the content of a file.

<a name="easywall.utility.write_into_file"></a>
#### write\_into\_file

```python
write_into_file(filepath: str, content: str) -> bool
```

Write text into a file as a wrapper around the os functions.

<a name="easywall.utility.get_abs_path_of_filepath"></a>
#### get\_abs\_path\_of\_filepath

```python
get_abs_path_of_filepath(filepath: str) -> str
```

Return the absolute path of a path containing a filename.

<a name="easywall.utility.rename_file"></a>
#### rename\_file

```python
rename_file(oldpath: str, newpath: str) -> bool
```

Rename a the file from the absolute path oldpath into the file from newpath.

<a name="easywall.utility.file_exists"></a>
#### file\_exists

```python
file_exists(filepath: str) -> bool
```

Check if a fiven file exists on the system.

[Data Types] boolean

<a name="easywall.utility.folder_exists"></a>
#### folder\_exists

```python
folder_exists(folder_path: str) -> bool
```

Check if a given folder exists on the system.

[Data Types] bool

<a name="easywall.utility.is_float"></a>
#### is\_float

```python
is_float(value: Any) -> bool
```

Try to convert input value into a float value.

[Data Types] bool

<a name="easywall.utility.is_int"></a>
#### is\_int

```python
is_int(value: Any) -> bool
```

Try to convert the input value into a int value.

[Data Types] bool

<a name="easywall.utility.csv_to_array"></a>
#### csv\_to\_array

```python
csv_to_array(inputstr: str, delimiter: str) -> List[str]
```

Convert a CSV string into a Python compatible array.

[Data Types] List[str]

<a name="easywall.utility.urlencode"></a>
#### urlencode

```python
urlencode(inputstr: str) -> str
```

Convert a String to a URL Encoded String.

[Data Types] str

<a name="easywall.utility.format_exception"></a>
#### format\_exception

```python
format_exception(exc: Exception) -> str
```

Convert a exception object to a readable string.

[Data Types] str

<a name="easywall.utility.time_duration_diff"></a>
#### time\_duration\_diff

```python
time_duration_diff(date1: datetime, date2: datetime) -> str
```

Calculate the difference between two dates and returns them as a string.

[Data Types] str

<a name="easywall.utility.execute_os_command"></a>
#### execute\_os\_command

```python
execute_os_command(command: str) -> bool
```

Execute a command on the operating system.

[Data Types] bool

<a name="easywall.web"></a>
# easywall.web

<a name="easywall.web.apply"></a>
# easywall.web.apply

the module contains functions for the apply rules route

<a name="easywall.web.apply.apply"></a>
#### apply

```python
apply(saved: bool = False, step: int = 1) -> str
```

the function returns the apply page when the user is logged in

<a name="easywall.web.apply.apply_save"></a>
#### apply\_save

```python
apply_save() -> str
```

the function applies the configuration and copies the rules to easywall core

<a name="easywall.web.apply.apply_step_one"></a>
#### apply\_step\_one

```python
apply_step_one() -> None
```

the function triggeres the easywall core to apply the new firewall rules

<a name="easywall.web.apply.apply_step_two"></a>
#### apply\_step\_two

```python
apply_step_two() -> None
```

the function writes true into the accept file from easywall core

<a name="easywall.web.blacklist"></a>
# easywall.web.blacklist

the module contains functions for the blacklist route

<a name="easywall.web.blacklist.blacklist"></a>
#### blacklist

```python
blacklist(saved: bool = False) -> str
```

the function returns the blacklist page when the user is logged in

<a name="easywall.web.blacklist.blacklist_save"></a>
#### blacklist\_save

```python
blacklist_save() -> str
```

the function saves the blacklist rules into the corresponding rulesfile

<a name="easywall.web.custom"></a>
# easywall.web.custom

the module contains functions for the custom rules route

<a name="easywall.web.custom.custom"></a>
#### custom

```python
custom(saved: bool = False) -> str
```

the function returns the custom rules page when the user is logged in

<a name="easywall.web.custom.custom_save"></a>
#### custom\_save

```python
custom_save() -> str
```

the function saves the custom rules into the corresponding rulesfile

<a name="easywall.web.defaultpayload"></a>
# easywall.web.defaultpayload

the module contains a empty class which is used as object

<a name="easywall.web.defaultpayload.DefaultPayload"></a>
## DefaultPayload Objects

```python
class DefaultPayload(object)
```

the class is a empty skeleton for generating objects

<a name="easywall.web.error"></a>
# easywall.web.error

the module contains functions for custom error routes

<a name="easywall.web.error.page_not_found"></a>
#### page\_not\_found

```python
page_not_found(error: str) -> Union[str, Tuple[str, int]]
```

the function returns the 404 error page when the user is logged in

<a name="easywall.web.error.forbidden"></a>
#### forbidden

```python
forbidden(error: str) -> Union[str, Tuple[str, int]]
```

the function returns the 403 error page when the user is logged in

<a name="easywall.web.firstrun"></a>
# easywall.web.firstrun

TODO: Doku.

<a name="easywall.web.firstrun.firstrun"></a>
#### firstrun

```python
firstrun(message: Union[None, str] = None, messagetype: Union[None, str] = None) -> Union[Response, str]
```

TODO: Doku.

<a name="easywall.web.firstrun.firstrun_save"></a>
#### firstrun\_save

```python
firstrun_save() -> Union[Response, str]
```

TODO: Doku.

<a name="easywall.web.forwarding"></a>
# easywall.web.forwarding

TODO: Doku.

<a name="easywall.web.forwarding.forwarding"></a>
#### forwarding

```python
forwarding(saved: bool = False) -> str
```

TODO: Doku.

<a name="easywall.web.forwarding.forwarding_save"></a>
#### forwarding\_save

```python
forwarding_save() -> str
```

TODO: Doku.

<a name="easywall.web.forwarding.add_forwarding"></a>
#### add\_forwarding

```python
add_forwarding(source_port: str, dest_port: str, ruletype: str) -> None
```

TODO: Doku.

<a name="easywall.web.forwarding.remove_forwarding"></a>
#### remove\_forwarding

```python
remove_forwarding(source_port: str, dest_port: str, ruletype: str) -> None
```

TODO: Doku.

<a name="easywall.web.index"></a>
# easywall.web.index

The module contains functions for the index route.

<a name="easywall.web.index.index"></a>
#### index

```python
index() -> str
```

Return the index page when the user is logged in.

<a name="easywall.web.login"></a>
# easywall.web.login

Create functions for user login and logout.

<a name="easywall.web.login.login"></a>
#### login

```python
login(message: Union[None, str] = None, messagetype: Union[None, str] = None) -> str
```

Return the login page which shows messages.

also the function updates the last commit informations in the config file

<a name="easywall.web.login.login_post"></a>
#### login\_post

```python
login_post(ip_ban: IpBan) -> Union[Response, str]
```

Handle the login post request and if all information are correct.

a session variable is set to store the login information

<a name="easywall.web.login.logout"></a>
#### logout

```python
logout() -> str
```

Remove the logged_in session variable if the user is logged in.

<a name="easywall.web.options"></a>
# easywall.web.options

the module contains functions for the options route

<a name="easywall.web.options.options"></a>
#### options

```python
options(saved: bool = False, error: str = "") -> str
```

the function returns the options page when the user is logged in

<a name="easywall.web.options.options_save"></a>
#### options\_save

```python
options_save() -> str
```

the function saves the options from a section using the config class
for example the Enabled flag in the IPv6 section is saved to the config file

<a name="easywall.web.options.correct_value_checkbox"></a>
#### correct\_value\_checkbox

```python
correct_value_checkbox(key: str) -> str
```

the function corrects the value of a checkbox

<a name="easywall.web.passwd"></a>
# easywall.web.passwd

the module creates a new password and writes the password into the config file

<a name="easywall.web.passwd.Passwd"></a>
## Passwd Objects

```python
class Passwd(object)
```

the class contains the password generation and saving

<a name="easywall.web.passwd.Passwd.__init__"></a>
#### \_\_init\_\_

```python
 | __init__() -> None
```

the init function creates the config variable and calls the user input

<a name="easywall.web.passwd.Passwd.savepasswd"></a>
#### savepasswd

```python
 | savepasswd(password: str) -> None
```

the function saves the password into the config file using the config class

<a name="easywall.web.passwd.Passwd.saveuser"></a>
#### saveuser

```python
 | saveuser(username: str) -> None
```

the function saves the username into the config file using the config class

<a name="easywall.web.passwd.Passwd.ask_user"></a>
#### ask\_user

```python
 | ask_user() -> None
```

the function asks the user for the username and password

<a name="easywall.web.ports"></a>
# easywall.web.ports

the module contains functions for the ports route

<a name="easywall.web.ports.ports"></a>
#### ports

```python
ports(saved: bool = False) -> str
```

the function returns the ports page when the user is logged in

<a name="easywall.web.ports.ports_save"></a>
#### ports\_save

```python
ports_save() -> str
```

the function saves the tcp and udp rules into the corresponding rulesfiles

<a name="easywall.web.ports.add_port"></a>
#### add\_port

```python
add_port(port: str, ruletype: str) -> None
```

The function adds a port to the list of open ports.

<a name="easywall.web.ports.remove_port"></a>
#### remove\_port

```python
remove_port(port: str, ruletype: str) -> None
```

The function deletes a port from the list of open ports.

<a name="easywall.web.webutils"></a>
# easywall.web.webutils

Create a helper class for all web routes which contains shared functions.

<a name="easywall.web.webutils.Webutils"></a>
## Webutils Objects

```python
class Webutils(object)
```

Create a couple of shared functions used in the route functions.

<a name="easywall.web.webutils.Webutils.__init__"></a>
#### \_\_init\_\_

```python
 | __init__() -> None
```

TODO: Doku.

<a name="easywall.web.webutils.Webutils.check_login"></a>
#### check\_login

```python
 | check_login(request: Request) -> bool
```

Check if the user/session is logged in.

<a name="easywall.web.webutils.Webutils.check_first_run"></a>
#### check\_first\_run

```python
 | check_first_run() -> bool
```

Check if the webinterface is run for the first time.

<a name="easywall.web.webutils.Webutils.get_default_payload"></a>
#### get\_default\_payload

```python
 | get_default_payload(title: str, css: str = "easywall") -> DefaultPayload
```

Create a object of information that are needed on every page.

<a name="easywall.web.webutils.Webutils.get_machine_infos"></a>
#### get\_machine\_infos

```python
 | get_machine_infos() -> dict
```

Retrieve some information about the host and returns them as a list.

<a name="easywall.web.webutils.Webutils.get_config_version_mismatch"></a>
#### get\_config\_version\_mismatch

```python
 | get_config_version_mismatch(cfgtype: str) -> bool
```

TODO: Doku.

<a name="easywall.web.webutils.Webutils.get_commit_date"></a>
#### get\_commit\_date

```python
 | get_commit_date(datestring: str) -> str
```

Compare a datetime with the current date.

for comparing the datestring parameter is in UTC timezone

<a name="easywall.web.webutils.Webutils.update_last_commit_infos"></a>
#### update\_last\_commit\_infos

```python
 | update_last_commit_infos() -> None
```

Retrieve the last commit information after a specific waiting time.

after retrieving the information they are saved into the config file

<a name="easywall.web.webutils.Webutils.get_latest_commit"></a>
#### get\_latest\_commit

```python
 | get_latest_commit() -> Any
```

Retrieve the informations of the last commit from github as json.

also converts the information into a python object
for example the object contains the last commit date and the last commit sha
This function should not be called very often, because GitHub has a rate limit implemented

<a name="easywall.web.webutils.Webutils.get_latest_version"></a>
#### get\_latest\_version

```python
 | get_latest_version() -> str
```

Retrieve the latest version from github and returns the version string.

<a name="easywall.web.webutils.Webutils.get_last_accept_time"></a>
#### get\_last\_accept\_time

```python
 | get_last_accept_time() -> str
```

Retrieve the modify time of the acceptance file.

also compares the time to the current time

<a name="easywall.web.webutils.Webutils.get_acceptance_status"></a>
#### get\_acceptance\_status

```python
 | get_acceptance_status() -> str
```

Get the status of the current acceptance.

<a name="easywall.web.whitelist"></a>
# easywall.web.whitelist

the module contains functions for the whitelist route

<a name="easywall.web.whitelist.whitelist"></a>
#### whitelist

```python
whitelist(saved: bool = False) -> str
```

the function returns the whitelist page when the user is logged in

<a name="easywall.web.whitelist.whitelist_save"></a>
#### whitelist\_save

```python
whitelist_save() -> str
```

the function saves the whitelist rules into the corresponding rulesfile

<a name="easywall.web.__main__"></a>
# easywall.web.\_\_main\_\_

The app module contains all information of the Flask app.

<a name="easywall.web.__main__.apply_headers"></a>
#### apply\_headers

```python
@APP.after_request
apply_headers(response: wrappers.Response) -> wrappers.Response
```

TODO: Doku.

<a name="easywall.web.__main__.index_route"></a>
#### index\_route

```python
@APP.route('/')
index_route() -> Union[Response, str]
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.options_route"></a>
#### options\_route

```python
@APP.route('/options')
options_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.options_save_route"></a>
#### options\_save\_route

```python
@APP.route('/options-save', methods=['POST'])
options_save_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.blacklist_route"></a>
#### blacklist\_route

```python
@APP.route('/blacklist')
blacklist_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.blacklist_save_route"></a>
#### blacklist\_save\_route

```python
@APP.route('/blacklist-save', methods=['POST'])
blacklist_save_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.whitelist_route"></a>
#### whitelist\_route

```python
@APP.route('/whitelist')
whitelist_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.whitelist_save_route"></a>
#### whitelist\_save\_route

```python
@APP.route('/whitelist-save', methods=['POST'])
whitelist_save_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.forwarding_route"></a>
#### forwarding\_route

```python
@APP.route('/forwarding')
forwarding_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.forwarding_save_route"></a>
#### forwarding\_save\_route

```python
@APP.route('/forwarding-save', methods=['POST'])
forwarding_save_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.ports_route"></a>
#### ports\_route

```python
@APP.route('/ports')
ports_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.ports_save_route"></a>
#### ports\_save\_route

```python
@APP.route('/ports-save', methods=['POST'])
ports_save_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.custom_route"></a>
#### custom\_route

```python
@APP.route('/custom')
custom_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.custom_save_route"></a>
#### custom\_save\_route

```python
@APP.route('/custom-save', methods=['POST'])
custom_save_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.apply_route"></a>
#### apply\_route

```python
@APP.route('/apply')
apply_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.apply_save_route"></a>
#### apply\_save\_route

```python
@APP.route('/apply-save', methods=['POST'])
apply_save_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.login_post_route"></a>
#### login\_post\_route

```python
@APP.route('/login', methods=['POST'])
login_post_route() -> Union[Response, str]
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.logout_route"></a>
#### logout\_route

```python
@APP.route("/logout")
logout_route() -> str
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.firstrun_route"></a>
#### firstrun\_route

```python
@APP.route("/firstrun")
firstrun_route() -> Union[Response, str]
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.firstrun_save_route"></a>
#### firstrun\_save\_route

```python
@APP.route("/firstrun", methods=['POST'])
firstrun_save_route() -> Union[Response, str]
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.page_not_found_route"></a>
#### page\_not\_found\_route

```python
@APP.errorhandler(404)
page_not_found_route(error: str) -> Union[str, Tuple[str, int]]
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.forbidden_route"></a>
#### forbidden\_route

```python
@APP.errorhandler(403)
forbidden_route(error: str) -> Union[str, Tuple[str, int]]
```

Call the corresponding function from the appropriate module.

<a name="easywall.web.__main__.before_request_func"></a>
#### before\_request\_func

```python
@APP.before_request
before_request_func() -> None
```

TODO: Doku.

<a name="easywall.web.__main__.DefaultConfig"></a>
## DefaultConfig Objects

```python
class DefaultConfig(object)
```

TODO: Doku.

<a name="easywall.web.__main__.ProductionConfig"></a>
## ProductionConfig Objects

```python
class ProductionConfig(DefaultConfig)
```

TODO: Doku.

<a name="easywall.web.__main__.DevelopmentConfig"></a>
## DevelopmentConfig Objects

```python
class DevelopmentConfig(DefaultConfig)
```

TODO: Doku.

<a name="easywall.web.__main__.Main"></a>
## Main Objects

```python
class Main(object)
```

TODO: Doku.

<a name="easywall.web.__main__.Main.__init__"></a>
#### \_\_init\_\_

```python
 | __init__(debug: bool = False) -> None
```

TODO: Doku.

<a name="easywall.web.__main__.Main.run_debug"></a>
#### run\_debug

```python
 | run_debug() -> None
```

TODO: Doku.

<a name="easywall.__main__"></a>
# easywall.\_\_main\_\_

TODO: Doku.

<a name="easywall.__main__.ModifiedHandler"></a>
## ModifiedHandler Objects

```python
class ModifiedHandler(FileSystemEventHandler)
```

TODO: Doku.

<a name="easywall.__main__.ModifiedHandler.__init__"></a>
#### \_\_init\_\_

```python
 | __init__(apply: Callable) -> None
```

TODO: Doku.

<a name="easywall.__main__.ModifiedHandler.on_created"></a>
#### on\_created

```python
 | on_created(event: FileSystemEvent) -> None
```

TODO: Doku.

<a name="easywall.__main__.Main"></a>
## Main Objects

```python
class Main()
```

TODO: Doku.

<a name="easywall.__main__.Main.__init__"></a>
#### \_\_init\_\_

```python
 | __init__() -> None
```

TODO: Doku.

<a name="easywall.__main__.Main.apply"></a>
#### apply

```python
 | apply(filename: str) -> None
```

TODO: Doku.

<a name="easywall.__main__.Main.start_observer"></a>
#### start\_observer

```python
 | start_observer() -> None
```

Keep the main process running until it should be stopped.

if someone is pressing ctrl + C the software will initiate the stop process

<a name="easywall.__main__.Main.shutdown"></a>
#### shutdown

```python
 | shutdown() -> None
```

Stop all threads and shut the software down gracefully.

