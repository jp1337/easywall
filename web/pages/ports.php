<div class="jumbotron text-center">
	<div class="container">
		<h1>Open Ports</h1>
		<p class="lead">On this page you can add the ports for incomin connections.<br>You can add tcp and udp ports.<br>Please check periodically if you need the ports. If you don't need them, close them.<br>You can list all listening ports on linux with the command <code>netstat --listen</code></p>
	</div>
</div>

<div class="container pt-0 mb-3">
	<div class="row mb-3">
		<div class="col">
			<div class="card w-100">
			<div class="card-header">
				Open Port
			</div>
			<div class="card-block">
				<form action="ports.php" method="post">
					<div class="form-group">
						<div class="form-check form-check-inline">
							<label class="form-check-label">
								<input class="form-check-input" aria-describedby="tcpudpHelp" type="radio" name="tcpudp" id="tcpudp1" value="tcp"> TCP
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
						<input type="text" class="form-control" id="port" name="port" aria-describedby="portHelp" placeholder="22 / 22:80">
						<small id="portHelp" class="form-text text-muted">Please enter the Port-Number you want to open on your server. You can open a single port or a port range. To open a port range just type <code>startport:endport</code>.</small>
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
			<ul class="list-group">
				<li class="list-group-item justify-content-between">
					<span class="badge badge-default badge-pill">tcp</span>
					80
					<button type="button" class="btn btn-danger btn-sm">Remove</button>
				</li>
				<li class="list-group-item justify-content-between">
					<span class="badge badge-default badge-pill">tcp - ssh</span>
					22
					<button type="button" class="btn btn-danger btn-sm">Remove</button>
				</li>
			</ul>
		</div>
		<div class="col-md">
			<ul class="list-group">
				<li class="list-group-item justify-content-between">
					<span class="badge badge-default badge-pill">udp</span>
					15000:16000
					<button type="button" class="btn btn-danger btn-sm">Remove</button>
				</li>
				<li class="list-group-item justify-content-between">
					<span class="badge badge-default badge-pill">udp</span>
					541
					<button type="button" class="btn btn-danger btn-sm">Remove</button>
				</li>
			</ul>
		</div>
	</div>
</div>