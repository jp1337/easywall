<div class="jumbotron text-center">
	<div class="container">
		<h1>Whitelist</h1>
		<p class="lead">On the whitelist page you can add ip-addresses that are allowed to connect to any port on your server.<br>Use this carefully and check periodically if the addresses are nessesary any more.<br>You can add IPv4 and IPv6 adresses here.</p>
	</div>
</div>

<div class="container pt-0 mb-3">
	<div class="row">
		<div class="col pb-3">
			<ul class="list-group">
				<li class="list-group-item justify-content-between">
					000.000.000.000
					<button type="button" class="btn btn-danger btn-sm">Remove</button>
				</li>
				<li class="list-group-item justify-content-between">
					2003:de:2bc0:4a00:654b:17df:ce53:c7ca
					<button type="button" class="btn btn-danger btn-sm">Remove</button>
				</li>
			</ul>
		</div>
		<div class="col">
			<div class="card w-100">
				<div class="card-header">
					Allow IP-Address
				</div>
				<div class="card-block">
						<form action="blacklist.php" method="post">
							<div class="form-group">
								<label for="ipadr">IP-Address</label>
								<input type="text" class="form-control" id="ipadr" name="ipadr" aria-describedby="ipadrHelp" placeholder="123.123.123.123 / 1234:1234:1234:1234:1234:1234:1234:1234">
								<small id="ipadrHelp" class="form-text text-muted">Please enter the IP-Adress to whitelist. If you enter an IPv6-Address it <strong>has to be expanded</strong>. Please do not add hostnames here.</small>
							</div>
							<button type="submit" class="btn btn-primary">Save</button>
						</form>
				</div>
			</div>
		</div>
	</div>
</div>