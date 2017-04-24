<div class="jumbotron text-center">
	<div class="container">
		<h1>Apply Configuration</h1>
		<p class="lead">The rules and ports you have defined are for security reasons not applied automatically.<br>On this page you can apply your rules safely.<br>EasyWall has a builtin two step activation.</p>
	</div>
</div>

<div class="container pt-0 mb-3">
	<div class="row">
		<div class="col">
			<div class="card w-100 text-center">
				<div class="card-header">
					Information
				</div>
				<div class="card-block">
					<?php
						if ($_SERVER['REQUEST_METHOD'] != 'POST') {
							echo 'The firewall rules were applied <span class="badge badge-success">' . humanTiming(filemtime("../rules/.applied")) . '</span> ago.<br />';
							echo '<br />';
							echo '<div class="alert alert-warning" role="alert">';
							echo '	<strong>Warning!</strong> Are you sure you want to apply the rules?<br>';
							echo '	After pushing the button you have 30 seconds to check if the server responses properly.<br>';
							echo '	The most important things to check are ssh and web access.';
							echo '</div>';
						}
						
						if (isset($_POST['apply_step1'])) {
							echo 'You still have <span id="countdown" class="badge badge-warning"></span> seconds left to check the firewall rules.<br />';
							echo '<br />';
							echo '<div class="alert alert-info" role="alert">';
							echo '	Now you have <strong>30 seconds</strong> to press the button and activate the settings.';
							echo '	Check if all needed applications can connect.';
							echo '</div>';
						}
						
						if (isset($_POST['apply_step2'])) {
							echo '<div class="alert alert-success" role="alert">';
							echo '	You have successfully activated the firewall rules.<br>';
							echo '	You can now use <code>iptables-save</code> or <code>ip6tables-save</code> in command prompt to view the new firewall rules.';
							echo '</div>';
						}
						
						if (isset($_POST['apply_timeout'])) {
							echo '<div class="alert alert-danger" role="alert">';
							echo '	<strong>Attention!</strong> You got into the 30 seconds timeout.<br>';
							echo '	The firewall has been automatically disabled. All ports are now open.<br>';
							echo '  Please correct the firewall rules and re-activate them as soon as possible.<br>';
							echo '</div>';
						}
					?>
				</div>
			</div>
		</div>
	</div>
	
	<script type="text/javascript">
		// set the date we're counting down to
		var target = new Date();
		target.setSeconds(target.getSeconds() + 30);
		var target = target.getTime();
		 
		// variables for time units
		var seconds, now, distance;
		 
		// get tag element
		var countdown = document.getElementById('countdown');
		 
		// update the tag with id "countdown" every 1 second
		if (countdown != null) {
			setInterval(function () {
				now = new Date().getTime();
				distance = target - now;
				seconds = Math.floor((distance % (1000 * 60)) / 1000);
				countdown.innerHTML = seconds;
			}, 1000);
			
			setInterval(function () {
				var form = document.createElement('form');
				form.method = 'post';
				form.action = 'apply.php';
				var input = document.createElement('input');
				input.type = 'text';
				input.name = 'apply_timeout';
				form.appendChild(input);
				document.body.appendChild(form);
				form.submit();
			}, 30000);
		}
	</script>
	
	<div class="row mt-4">
		<div class="col text-center">
			<form action="apply.php" method="post">
				<button type="submit" name="<?php if (isset($_POST['apply_step1'])) { echo 'apply_step2'; } else { echo 'apply_step1'; } ?>" class="btn btn-danger"><?php if (isset($_POST['apply_step1'])) { echo 'Yes, I checked the server access'; } else { echo 'Apply firewall rules'; } ?></button>
			</form>
		</div>
	</div>
</div>