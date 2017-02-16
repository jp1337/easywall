<div class="jumbotron text-center">
	<div class="container">
		<h1>Apply Configuration</h1>
		<p class="lead">Here is the page where you can apply your changes. For security Reasons the changes are not applied automatically.<br>After pressing the Apply Button you have 30 seconds time to commit the changes.<br>If you dont, they automatically revert to firewall start state.</p>
	</div>
</div>

<div class="container pt-0 mb-3">
	<div class="row">
		<div class="col">
			<div class="card w-100 text-center">
				<div class="card-header">
					Apply Changes
				</div>
				<div class="card-block">
					<div class="alert alert-warning" role="alert">
						<strong>Warning!</strong> Are you sure you want to apply the settings?<br>
						You have 30 seconds to check, if the server responses properly.
					</div>
					<form action="apply.php" method="post">
						<button type="submit" class="btn btn-danger">Apply Firewall Settings</button>
					</form>
				</div>
			</div>
		</div>
	</div>
</div>