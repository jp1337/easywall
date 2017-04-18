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
					<?php
					if (!isset($_POST['apply_step1'])) {
						echo '<div class="alert alert-warning" role="alert">';
						echo '	<strong>Warning!</strong> Are you sure you want to apply the settings?<br>';
						echo '	You have 30 seconds to check, if the server responses properly.';
						echo '</div>';
					} else {
						echo '<div class="alert alert-info" role="alert">';
						echo '	Now you have <strong>30 seconds</strong> to press the button and activate the settings.';
						echo '	Check if all needed applications can connect.';
						echo '</div>';
					}
					?>
					<form action="apply.php" method="post">
						<button type="submit" name="<?php if (isset($_POST['apply_step1'])) { echo 'apply_step2'; } else { echo 'apply_step1'; } ?>" class="btn btn-danger"><?php if (isset($_POST['apply_step1'])) { echo 'Commit Firewall Changes'; } else { echo 'Apply Firewall Settings'; } ?></button>
					</form>
				</div>
			</div>
		</div>
	</div>
</div>