<div class="jumbotron text-center">
	<div class="container">
		<h1>Open Ports</h1>
		<p class="lead">On the Open Ports page you can open incoming connections for your listening ports.<br>You can add tcp and udp ports.<br>Please check periodically if you need the ports. If you don't need them, close them.<br>You can list all listening ports on linux with the command <code>netstat -ln</code></p>
	</div>
</div>

<div class="container pt-0 mb-3">
	<div class="row mb-3">
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
				Open Port
			</div>
			<div class="card-block">
				<form action="ports.php" method="post">
					<div class="form-group">
						<div class="form-check form-check-inline">
							<label class="form-check-label">
								<input class="form-check-input" aria-describedby="tcpudpHelp" type="radio" name="tcpudp" id="tcpudp1" value="tcp" checked="checked"> TCP
							</label>
						</div>
						<div class="form-check form-check-inline">
							<label class="form-check-label">
								<input class="form-check-input" aria-describedby="tcpudpHelp" type="radio" name="tcpudp" id="tcpudp2" value="udp"> UDP
							</label>
						</div>
						<small id="tcpudpHelp" class="form-text text-muted">Select if you want to open a TCP or UDP port.</small>
					</div>
					<div class="form-group">
						<label for="port">Port</label>
						<input type="text" class="form-control" id="port" name="port" aria-describedby="portHelp">
						<small id="portHelp" class="form-text text-muted">
							Please enter the Port you want to open on your server. You can enter a single port or a port range.<br />
							<strong>Example:</strong> Single Port: <code>22</code> Port Range: <code>25017:25020</code>.
						</small>
					</div>
					<div class="form-check">
						<label class="form-check-label">
							<input type="checkbox" id="ssh" name="ssh" aria-describedby="sshHelp" class="form-check-input">
							SSH
						</label>
						<small id="sshHelp" class="form-text text-muted">Check this, if the port you want to open is the ssh port. There are some special rules executed on the ssh port.</small>
					</div>
					<button type="submit" class="btn btn-primary">Save</button>
				</form>
			</div>
		</div>
	</div>
	</div>
	<div class="row pb-3">
		<div class="col-md">
		
			<?php
			if (count($tcp->getAll()) === 0) {
				echo '<ul class="list-group">';
				echo '<li class="list-group-item justify-content-between">';
				echo 'No opened TCP Ports found.';
				echo '</li>';
				echo '</ul>';
			}
			?>
		
			<form action="ports.php" method="post">
				<input type="hidden" name="remove" value="tcp">
				<ul class="list-group">
					<?php
					foreach ($tcp->getAll() as $port) {
						echo '<li class="list-group-item justify-content-between">';
						if (count(explode(';', $port)) > 1)
						{
							echo '<span class="badge badge-default badge-pill">tcp - ' . explode(';', $port)[1] . '</span>';
							echo explode(';', $port)[0];
						} else {
							echo '<span class="badge badge-default badge-pill">tcp</span>';
							echo $port;
						}
						echo '<button type="submit" name="' . $port . '" class="btn btn-danger btn-sm">Remove</button>';
						echo '</li>';
					}
					?>
				</ul>
			</form>
		</div>
		<div class="col-md">
		
			<?php
			if (count($udp->getAll()) === 0) {
				echo '<ul class="list-group">';
				echo '<li class="list-group-item justify-content-between">';
				echo 'No opened UDP Ports found.';
				echo '</li>';
				echo '</ul>';
			}
			?>
		
			<form action="ports.php" method="post">
				<input type="hidden" name="remove" value="udp">
				<ul class="list-group">
					<?php
					foreach ($udp->getAll() as $port) {
						echo '<li class="list-group-item justify-content-between">';
						echo '<span class="badge badge-default badge-pill">udp</span>';
						echo $port;
						echo '<button type="submit" name="' . $port . '" class="btn btn-danger btn-sm">Remove</button>';
						echo '</li>';
					}
					?>
				</ul>
			</form>
		</div>
	</div>
</div>