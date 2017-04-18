<div class="jumbotron text-center">
	<div class="container">
		<h1>Whitelist</h1>
		<p class="lead">On the whitelist page you can add ip-addresses that are allowed to connect to any port on your server.<br>Use this carefully and check periodically if the addresses are nessesary any more.<br>You can add IPv4 and IPv6 adresses here.</p>
	</div>
</div>

<div class="container pt-0 mb-3">
	<div class="row">
		<div class="col pb-3">
		
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
			
			<?php
			if (count($rules->getAll()) === 0) {
				echo '<ul class="list-group">';
				echo '<li class="list-group-item justify-content-between">';
				echo 'No Whitelisted IP-Adresses found.';
				echo '</li>';
				echo '</ul>';
			}
			?>
			
			<form action="whitelist.php" method="post">
				<ul class="list-group">
					<?php
					foreach ($rules->getAll() as $ipadr) {
						echo '<li class="list-group-item justify-content-between">';
						echo $ipadr;
						echo '<button type="submit" name="' . $ipadr . '" class="btn btn-danger btn-sm">Remove</button>';
						echo '</li>';
					}
					?>
				</ul>
			</form>
		</div>
		<div class="col">
			<div class="card w-100">
				<div class="card-header">
					Whitelist IP-Address
				</div>
				<div class="card-block">
					<form action="whitelist.php" method="post">
						<div class="form-group">
							<label for="ipadr">IP-Address</label>
							<input type="text" class="form-control" id="ipadr" name="ipadr" aria-describedby="ipadrHelp">
							<small id="ipadrHelp" class="form-text text-muted">Please enter the IP-Adress to whitelist. If you enter an IPv6-Address it <strong>has to be expanded</strong>. Please do not add hostnames here.<br><br><strong>Examples:</strong><br>IPV4: 192.168.178.44<br>IPV6: 1234:1234:1234:1234:1234:1234:1234:1234</small>
						</div>
						<button type="submit" class="btn btn-primary">Save</button>
					</form>
				</div>
			</div>
		</div>
	</div>
</div>