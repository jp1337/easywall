<div class="jumbotron text-center">
	<div class="container">
		<h1>Options</h1>
		<p class="lead">EasyWall is configurable down to the last detail.<br />The system offers a variety of built-in safety rules. We recommend using the standard rules.</p>
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
							<a class="nav-link" data-toggle="tab" href="#easywall" role="tab">EasyWall</a>
						</li>
						<li class="nav-item">
							<a class="nav-link" data-toggle="tab" href="#logpane" role="tab">Log</a>
						</li>
						<li class="nav-item">
							<a class="nav-link" data-toggle="tab" href="#binaries" role="tab">Binaries</a>
						</li>
					</ul>
				</div>
				
				<div class="card-block">
					<div class="tab-content">
					
						<div class="tab-pane active" id="iptables" role="tabpanel">
							<form action="options.php" method="post">
								<div class="form-group checkbox">
									<label class="form-check-label" for="IPV6">
										<input type="hidden" value="false" name="IPV6">
										<input type="checkbox" id="IPV6" name="IPV6" <?php if ($config->getValue('IPV6') != false) { if ($config->getValue('IPV6') == 'true') { echo 'checked'; } } ?> aria-describedby="IPV6Help">
										Enable IPv6
									</label>
									<small id="IPV6Help" class="form-text text-muted">Please only activate the function if <strong>IPv6</strong> is <strong>configured and activated</strong> on the server. If you disable the feature, EasyWall will disable IPv6 entirely on the server for security reasons.</small>
								</div>
								<button type="submit" class="btn btn-primary">Save</button>
							</form>
						</div>
						
						<div class="tab-pane" id="easywall" role="tabpanel">
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
						
						<div class="tab-pane" id="logpane" role="tabpanel">
							<form action="options.php" method="post">
								<div class="form-group checkbox">
									<label class="form-check-label" for="LOG">
										<input type="hidden" value="false" name="LOG">
										<input type="checkbox" id="LOG" name="LOG" <?php if ($config->getValue('LOG') != false) { if ($config->getValue('LOG') == 'true') { echo 'checked'; } } ?> aria-describedby="LOGHelp">
										Enable Logging
									</label>
									<small id="LOGHelp" class="form-text text-muted">Logging stores different information every time the firewall is changed. In addition, the newly applied rules are stored in a file.</small>
								</div>
								<div class="form-group">
									<label for="logdir">Log Directory</label>
									<input type="text" class="form-control" id="logdir" name="LOGDIR" aria-describedby="logdirHelp" placeholder="Example: /log" <?php if ($config->getValue('LOGDIR') != false) { echo 'value="' . $config->getValue('LOGDIR') . '"'; } ?>>
									<small id="logdirHelp" class="form-text text-muted">Please specify the log directory. We recommend using the local <code>../log/</code> directory. The use of the global <code>/log</code> directory is also possible.</small>
								</div>
								<div class="form-group">
									<label for="logfile">Log File</label>
									<input type="text" class="form-control" id="logfile" name="LOGFILE" aria-describedby="logfileHelp" placeholder="Example: easywall.log" <?php if ($config->getValue('LOGFILE') != false) { echo 'value="' . $config->getValue('LOGFILE') . '"'; } ?>>
									<small id="logfileHelp" class="form-text text-muted">Please specify the filename for the logfile.</small>
								</div>
								<div class="form-group">
									<label for="LOGIPV4">IPv4 Rules File</label>
									<input type="text" class="form-control" id="LOGIPV4" name="LOGIPV4" aria-describedby="LOGIPV4Help" placeholder="Example: rules.v4.log" <?php if ($config->getValue('LOGIPV4') != false) { echo 'value="' . $config->getValue('LOGIPV4') . '"'; } ?>>
									<small id="LOGIPV4Help" class="form-text text-muted">After every change to the firewall, the current IPv4 rules are stored in this file. Specify the name for the file.</small>
								</div>
								<div class="form-group">
									<label for="LOGIPV6">IPv6 Rules File</label>
									<input type="text" class="form-control" id="LOGIPV6" name="LOGIPV6" aria-describedby="LOGIPV6Help" placeholder="Example: rules.v6.log" <?php if ($config->getValue('LOGIPV6') != false) { echo 'value="' . $config->getValue('LOGIPV6') . '"'; } ?>>
									<small id="LOGIPV6Help" class="form-text text-muted">After every change to the firewall, the current IPv6 rules are stored in this file. Specify the name for the file.</small>
								</div>
								<button type="submit" class="btn btn-primary">Save</button>
							</form>
						</div>
						
						<div class="tab-pane" id="binaries" role="tabpanel">
							<form action="options.php" method="post">
								<div class="form-group">
									<label for="iptablesbin">IPTables Binary</label>
									<input type="text" class="form-control" id="iptablesbin" name="IPTABLES" aria-describedby="iptablesbinHelp" placeholder="Example: /sbin/iptables" <?php if ($config->getValue('IPTABLES') != false) { echo 'value="' . $config->getValue('IPTABLES') . '"'; } ?>>
									<small id="iptablesbinHelp" class="form-text text-muted">Please enter the full path to the IPTables Binary.<br />You can read out the path in the command line with the command <code>whereis iptables</code>.</small>
								</div>
								<div class="form-group">
									<label for="iptablessavebin">IPTables-Save Binary</label>
									<input type="text" class="form-control" id="iptablessavebin" name="IPTABLES_SAVE" aria-describedby="iptablessavebinHelp" placeholder="Example: /sbin/iptables-save" <?php if ($config->getValue('IPTABLES_SAVE') != false) { echo 'value="' . $config->getValue('IPTABLES_SAVE') . '"'; } ?>>
									<small id="iptablessavebinHelp" class="form-text text-muted">Please enter the full path to the IPTables-Save Binary.<br />This is needed for saving the current iptables rules. You can read out the path in the command line with the command <code>whereis iptables-save</code>.</small>
								</div>
								<div class="form-group">
									<label for="ip6tablesbin">IP6Tables Binary</label>
									<input type="text" class="form-control" id="ip6tablesbin" name="IP6TABLES" aria-describedby="ip6tablesbinHelp" placeholder="Example: /sbin/ip6tables" <?php if ($config->getValue('IP6TABLES') != false) { echo 'value="' . $config->getValue('IP6TABLES') . '"'; } ?>>
									<small id="ip6tablesbinHelp" class="form-text text-muted">Please enter the full path to the IP6Tables Binary.<br />You can read out the path in the command line with the command <code>whereis ip6tables</code>.</small>
								</div>
								<div class="form-group">
									<label for="ip6tablessavebin">IP6Tables-Save Binary</label>
									<input type="text" class="form-control" id="ip6tablessavebin" name="IP6TABLES_SAVE" aria-describedby="ip6tablessavebinHelp" placeholder="Example: /sbin/ip6tables-save" <?php if ($config->getValue('IP6TABLES_SAVE') != false) { echo 'value="' . $config->getValue('IP6TABLES_SAVE') . '"'; } ?>>
									<small id="ip6tablessavebinHelp" class="form-text text-muted">Please enter the full path to the IP6Tables-Save Binary.<br />This is needed for saving the current ip6tables rules. You can read out the path in the command line with the command <code>whereis ip6tables-save</code>.</small>
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