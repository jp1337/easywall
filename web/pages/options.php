<div class="jumbotron text-center">
	<div class="container">
		<h1>Options</h1>
		<p class="lead">Here you can edit general script and firewall settings.<br>First of all you have to choose the folder path to the EasyWall Firewall folder on your server.<br>Then you can edit the general firewall settings including some builtin rules.</p>
	</div>
</div>

<div class="container pt-0 mb-3">
	<div class="row">
		<div class="col">
		
			<?php
			if (isset($saved)) {
				if ($saved == true) {
					echo '<div class="alert alert-success" role="alert">';
					echo '	<strong>Well done!</strong> The Configuration was saved successfully.';
					echo '</div>';
				} else {
					echo '<div class="alert alert-danger" role="alert">';
					echo '	<strong>Oh snap!</strong> There was an error saving the configuration... Please review WebServer Logfile!';
					echo '</div>';
				}
			}
			?>
		
			<div class="card w-100">
				<div class="card-header">
					<ul class="nav nav-tabs card-header-tabs">
						<li class="nav-item">
							<a class="nav-link active" data-toggle="tab" href="#iptables" role="tab">IPTables</a>
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
						<div class="tab-pane active" id="iptables" role="tabpanel">
							<form action="options.php" method="post">
								<div class="form-group">
									<label for="enableipv6">Enable IPv6 Rules</label>
									<select class="form-control" id="enableipv6" name="IPV6" aria-describedby="enableipv6Help">
										<option value="true" <?php if ($config->getValue('IPV6') != false) { if ($config->getValue('IPV6') == 'true') { echo 'selected'; } } ?>>Yes</option>
										<option value="false" <?php if ($config->getValue('IPV6') != false) { if ($config->getValue('IPV6') == 'false') { echo 'selected'; } } ?>>No</option>
									</select>
									<small id="enableipv6Help" class="form-text text-muted">Enable this option if you have configured IPv6 on your server. Please ensure that your server and ISP supports IPv6.</small>
								</div>
								<div class="form-group">
									<label for="iptablesbin">IPTables Binary</label>
									<input type="text" class="form-control" id="iptablesbin" name="IPTABLES" aria-describedby="iptablesbinHelp" placeholder="Example: /sbin/iptables" <?php if ($config->getValue('IPTABLES') != false) { echo 'value="' . $config->getValue('IPTABLES') . '"'; } ?>>
									<small id="iptablesbinHelp" class="form-text text-muted">Enter the full path to your IPTables Binary. On command line you can find it with the command <code>whereis iptables</code>.</small>
								</div>
								<div class="form-group">
									<label for="iptablessavebin">IPTables-Save Binary</label>
									<input type="text" class="form-control" id="iptablessavebin" name="IPTABLES_SAVE" aria-describedby="iptablessavebinHelp" placeholder="Example: /sbin/iptables-save" <?php if ($config->getValue('IPTABLES_SAVE') != false) { echo 'value="' . $config->getValue('IPTABLES_SAVE') . '"'; } ?>>
									<small id="iptablessavebinHelp" class="form-text text-muted">Enter the full path to your IPTables-Save Binary. This is needed for saving the current iptables rules. On command line you can find it with the command <code>whereis iptables-save</code>.</small>
								</div>
								<div class="form-group">
									<label for="ip6tablesbin">IP6Tables Binary</label>
									<input type="text" class="form-control" id="ip6tablesbin" name="IP6TABLES" aria-describedby="ip6tablesbinHelp" placeholder="Example: /sbin/ip6tables" <?php if ($config->getValue('IP6TABLES') != false) { echo 'value="' . $config->getValue('IP6TABLES') . '"'; } ?>>
									<small id="ip6tablesbinHelp" class="form-text text-muted">Enter the full path to your IP6Tables Binary. On command line you can find it with the command <code>whereis ip6tables</code>.</small>
								</div>
								<div class="form-group">
									<label for="ip6tablessavebin">IP6Tables-Save Binary</label>
									<input type="text" class="form-control" id="ip6tablessavebin" name="IP6TABLES_SAVE" aria-describedby="ip6tablessavebinHelp" placeholder="Example: /sbin/ip6tables-save" <?php if ($config->getValue('IP6TABLES_SAVE') != false) { echo 'value="' . $config->getValue('IP6TABLES_SAVE') . '"'; } ?>>
									<small id="ip6tablessavebinHelp" class="form-text text-muted">Enter the full path to your IP6Tables-Save Binary. This is needed for saving the current ip6tables rules. On command line you can find it with the command <code>whereis ip6tables-save</code>.</small>
								</div>
								<button type="submit" class="btn btn-primary">Save</button>
							</form>
						</div>
						<div class="tab-pane" id="log" role="tabpanel">
							<form action="options.php" method="post">
								<div class="form-group">
									<label for="enablelog">Enable Logging</label>
									<select class="form-control" id="enablelog" name="LOG" aria-describedby="enablelogHelp">
										<option value="true" <?php if ($config->getValue('LOG') != false) { if ($config->getValue('LOG') == 'true') { echo 'selected'; } } ?>>Yes</option>
										<option value="false" <?php if ($config->getValue('LOG') != false) { if ($config->getValue('LOG') == 'false') { echo 'selected'; } } ?>>No</option>
									</select>
									<small id="enablelogHelp" class="form-text text-muted">Enable this if EasyWall should write a log file on cron run.</small>
								</div>
								<div class="form-group">
									<label for="logdir">Log Directory</label>
									<input type="text" class="form-control" id="logdir" name="LOGDIR" aria-describedby="logdirHelp" placeholder="Example: log/" <?php if ($config->getValue('LOGDIR') != false) { echo 'value="' . $config->getValue('LOGDIR') . '"'; } ?>>
									<small id="logdirHelp" class="form-text text-muted">Enter the log directory. You can use a full path or a relative path from the EasyWall directory.</small>
								</div>
								<div class="form-group">
									<label for="logfile">Log File</label>
									<input type="text" class="form-control" id="logfile" name="LOGFILE" aria-describedby="logfileHelp" placeholder="Example: easywall.log" <?php if ($config->getValue('LOGFILE') != false) { echo 'value="' . $config->getValue('LOGFILE') . '"'; } ?>>
									<small id="logfileHelp" class="form-text text-muted">Enter the filename for the logfile. This is important if you want to use an existing log file.</small>
								</div>
								<button type="submit" class="btn btn-primary">Save</button>
							</form>
						</div>
						<div class="tab-pane" id="files" role="tabpanel">
							<form action="options.php" method="post">
								<div class="form-group">
									<label for="blacklistfile">Blacklist File</label>
									<input type="text" class="form-control" id="blacklistfile" name="BLACKLIST" aria-describedby="blacklistfileHelp" placeholder="Example: rules/blacklist.txt" <?php if ($config->getValue('BLACKLIST') != false) { echo 'value="' . $config->getValue('BLACKLIST') . '"'; } ?>>
									<small id="blacklistfileHelp" class="form-text text-muted">Enter the file path to the blacklist file. We suggest a full path here. The relative Path must be reachable by Web and Cron directory.</small>
								</div>
								<div class="form-group">
									<label for="whitelistfile">Whitelist File</label>
									<input type="text" class="form-control" id="whitelistfile" name="WHITELIST" aria-describedby="whitelistfileHelp" placeholder="Example: rules/whitelist.txt" <?php if ($config->getValue('WHITELIST') != false) { echo 'value="' . $config->getValue('WHITELIST') . '"'; } ?>>
									<small id="whitelistfileHelp" class="form-text text-muted">Enter the file path to the whitelist file. We suggest a full path here. The relative Path must be reachable by Web and Cron directory.</small>
								</div>
								<div class="form-group">
									<label for="tcpfile">TCP Ports File</label>
									<input type="text" class="form-control" id="tcpfile" name="TCP" aria-describedby="tcpfileHelp" placeholder="Example: rules/tcp.txt" <?php if ($config->getValue('TCP') != false) { echo 'value="' . $config->getValue('TCP') . '"'; } ?>>
									<small id="tcpfileHelp" class="form-text text-muted">Enter the file path to the tcp file. We suggest a full path here. The relative Path must be reachable by Web and Cron directory.</small>
								</div>
								<div class="form-group">
									<label for="udpfile">UDP Ports File</label>
									<input type="text" class="form-control" id="udpfile" name="UDP" aria-describedby="udpfileHelp" placeholder="Example: rules/udp.txt" <?php if ($config->getValue('UDP') != false) { echo 'value="' . $config->getValue('UDP') . '"'; } ?>>
									<small id="udpfileHelp" class="form-text text-muted">Enter the file path to the udp file. We suggest a full path here. The relative Path must be reachable by Web and Cron directory.</small>
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