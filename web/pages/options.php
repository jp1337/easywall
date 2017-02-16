<div class="jumbotron text-center">
	<div class="container">
		<h1>Options</h1>
		<p class="lead">Here you can edit general script and firewall settings.<br>First of all you have to choose the folder path to the EasyWall Firewall folder on your server.<br>Then you can edit the general firewall settings including some builtin rules.</p>
	</div>
</div>

<div class="container pt-0 mb-3">
	<div class="row">
		<div class="col">
			<div class="card w-100">
				<div class="card-header">
					<ul class="nav nav-tabs card-header-tabs">
						<li class="nav-item">
							<a class="nav-link active" data-toggle="tab" href="#general" role="tab">General</a>
						</li>
						<li class="nav-item">
							<a class="nav-link" data-toggle="tab" href="#iptables" role="tab">IPTables</a>
						</li>
						<li class="nav-item">
							<a class="nav-link" data-toggle="tab" href="#log" role="tab">Log</a>
						</li>
						<li class="nav-item">
							<a class="nav-link" data-toggle="tab" href="#files" role="tab">Files</a>
						</li>
					</ul>
				</div>
				<div class="card-block">
					<div class="tab-content">
						<div class="tab-pane active" id="general" role="tabpanel">
							<form action="options.php" method="post">
								<div class="form-group">
									<label for="homedir">EasyWall Directory</label>
									<input type="text" class="form-control" id="homedir" name="homedir" aria-describedby="homedirHelp" placeholder="/usr/local/EasyWall/" <?php if (!empty($config->homedir)) {echo('value="'.$config->homedir.'"');} ?>>
									<small id="homedirHelp" class="form-text text-muted">Enter the full path to your EasyWall folder on your server. We will read and write all files from this config value.</small>
								</div>
								<button type="submit" class="btn btn-primary">Save</button>
							</form>
						</div>
						<div class="tab-pane" id="iptables" role="tabpanel">
							<form action="options.php" method="post">
								<div class="form-group">
									<label class="form-check-label">
										<input type="checkbox" class="form-check-input" id="enableipv6" name="enableipv6" aria-describedby="enableipv6Help">
										Enable IPv6 Rules
									</label>
									<small id="enableipv6Help" class="form-text text-muted">Enable this option if you have configured IPv6 on your server. Please ensure that your server and ISP supports IPv6.</small>
								</div>
								<div class="form-group">
									<label for="iptablesbin">IPTables Binary</label>
									<input type="text" class="form-control" id="iptablesbin" name="iptablesbin" aria-describedby="iptablesbinHelp" placeholder="/sbin/iptables">
									<small id="iptablesbinHelp" class="form-text text-muted">Enter the full path to your IPTables Binary. On command line you can find it with the command <code>whereis iptables</code>.</small>
								</div>
								<div class="form-group">
									<label for="iptablessavebin">IPTables-Save Binary</label>
									<input type="text" class="form-control" id="iptablessavebin" name="iptablessavebin" aria-describedby="iptablessavebinHelp" placeholder="/sbin/iptables-save">
									<small id="iptablessavebinHelp" class="form-text text-muted">Enter the full path to your IPTables-Save Binary. This is needed for saving the current iptables rules. On command line you can find it with the command <code>whereis iptables-save</code>.</small>
								</div>
								<div class="form-group">
									<label for="ip6tablesbin">IP6Tables Binary</label>
									<input type="text" class="form-control" id="ip6tablesbin" name="ip6tablesbin" aria-describedby="ip6tablesbinHelp" placeholder="/sbin/ip6tables">
									<small id="ip6tablesbinHelp" class="form-text text-muted">Enter the full path to your IP6Tables Binary. On command line you can find it with the command <code>whereis ip6tables</code>.</small>
								</div>
								<div class="form-group">
									<label for="ip6tablessavebin">IP6Tables-Save Binary</label>
									<input type="text" class="form-control" id="ip6tablessavebin" name="ip6tablessavebin" aria-describedby="ip6tablessavebinHelp" placeholder="/sbin/ip6tables-save">
									<small id="ip6tablessavebinHelp" class="form-text text-muted">Enter the full path to your IP6Tables-Save Binary. This is needed for saving the current ip6tables rules. On command line you can find it with the command <code>whereis ip6tables-save</code>.</small>
								</div>
								<button type="submit" class="btn btn-primary">Save</button>
							</form>
						</div>
						<div class="tab-pane" id="log" role="tabpanel">
							<form action="options.php" method="post">
								<div class="form-group">
									<label class="form-check-label">
										<input type="checkbox" class="form-check-input" id="enablelog" name="enablelog" aria-describedby="enablelogHelp">
										Enable Logging
									</label>
									<small id="enablelogHelp" class="form-text text-muted">Enable this if EasyWall should write a log file on cron run.</small>
								</div>
								<div class="form-group">
									<label for="logdir">Log Directory</label>
									<input type="text" class="form-control" id="logdir" name="logdir" aria-describedby="logdirHelp" placeholder="log/">
									<small id="logdirHelp" class="form-text text-muted">Enter the log directory. You can use a full path or a relative path from the EasyWall directory.</small>
								</div>
								<div class="form-group">
									<label for="logfile">Log File</label>
									<input type="text" class="form-control" id="logfile" name="logfile" aria-describedby="logfileHelp" placeholder="easywall.log">
									<small id="logfileHelp" class="form-text text-muted">Enter the filename for the logfile. This is important if you want to use an existing log file.</small>
								</div>
								<button type="submit" class="btn btn-primary">Save</button>
							</form>
						</div>
						<div class="tab-pane" id="files" role="tabpanel">
							<form action="options.php" method="post">
								<div class="form-group">
									<label for="blacklistfile">Blacklist File</label>
									<input type="text" class="form-control" id="blacklistfile" name="blacklistfile" aria-describedby="blacklistfileHelp" placeholder="rules/blacklist.txt">
									<small id="blacklistfileHelp" class="form-text text-muted">Enter the file path to the blacklist file. You can use a full path or a relative path starting from EasyWall directory.</small>
								</div>
								<div class="form-group">
									<label for="whitelistfile">Whitelist File</label>
									<input type="text" class="form-control" id="whitelistfile" name="whitelistfile" aria-describedby="whitelistfileHelp" placeholder="rules/whitelist.txt">
									<small id="whitelistfileHelp" class="form-text text-muted">Enter the file path to the whitelist file. You can use a full path or a relative path starting from EasyWall directory.</small>
								</div>
								<div class="form-group">
									<label for="tcpfile">TCP Ports File</label>
									<input type="text" class="form-control" id="tcpfile" name="tcpfile" aria-describedby="tcpfileHelp" placeholder="rules/tcp.txt">
									<small id="tcpfileHelp" class="form-text text-muted">Enter the file path to the tcp ports file. You can use a full path or a relative path starting from EasyWall directory.</small>
								</div>
								<div class="form-group">
									<label for="udpfile">UDP Ports File</label>
									<input type="text" class="form-control" id="udpfile" name="udpfile" aria-describedby="udpfileHelp" placeholder="rules/udp.txt">
									<small id="udpfileHelp" class="form-text text-muted">Enter the file path to the udp ports file. You can use a full path or a relative path starting from EasyWall directory.</small>
								</div>
								<button type="submit" class="btn btn-primary">Save</button>
							</form>
						</div>
					</div>
				</div>
			</div>
		</div>
	</div>
</div>