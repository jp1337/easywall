		<!-- Help Modal -->
		<div class="modal fade" id="helpModal" tabindex="-1" role="dialog" aria-labelledby="helpModalLabel" aria-hidden="true">
			<div class="modal-dialog" role="document">
				<div class="modal-content">
					<div class="modal-header">
						<h5 class="modal-title" id="helpModalLabel">Help</h5>
						<button type="button" class="close" data-dismiss="modal" aria-label="Close">
							<span aria-hidden="true">&times;</span>
						</button>
					</div>
					<div class="modal-body">
						...
					</div>
					<div class="modal-footer">
						<button type="button" class="btn btn-secondary" data-dismiss="modal">Close</button>
					</div>
				</div>
			</div>
		</div>
		
		<!-- Version Modal -->
		<div class="modal fade" id="versionModal" tabindex="-1" role="dialog" aria-labelledby="versionModalLabel" aria-hidden="true">
			<div class="modal-dialog" role="document">
				<div class="modal-content">
					<div class="modal-header">
						<h5 class="modal-title" id="versionModalLabel">Version</h5>
						<button type="button" class="close" data-dismiss="modal" aria-label="Close">
							<span aria-hidden="true">&times;</span>
						</button>
					</div>
					<div class="modal-body">
						<p>
							<strong><?php if (version_compare(getCurrentVersion(), getLatestVersion()) >= 0) { echo 'You have installed the current version.'; } else { echo 'New version of netdata available!'; } ?></strong>
						</p>
						<hr>
						Your EasyWall version: <b><code><?php echo getCurrentVersion(); ?></code></b><br />
						Latest EasyWall version: <b><code><?php echo getLatestVersion(); ?></code></b><br />
						<hr>
						Latest commit: <b><code><?php echo getLatestCommitSha(); ?></code></b><br />
						Commit date: <b><code><?php echo getLatestCommitDate(); ?> ago</code></b><br />
						<hr>
						<p><a href="https://github.com/kingjp/easywall/wiki/update" target="_blank">Click here for directions on updating</a> your EasyWall installation.</p>
						<p>Please keep in mind to always keep your installation up to date. You have to inform yourself independently about the closure of possible security gaps.</p>
					</div>
					<div class="modal-footer">
						<button type="button" class="btn btn-secondary" data-dismiss="modal">Close</button>
					</div>
				</div>
			</div>
		</div>
		
		<!-- Host Modal -->
		<div class="modal fade" id="hostModal" tabindex="-1" role="dialog" aria-labelledby="hostModalLabel" aria-hidden="true">
			<div class="modal-dialog" role="document">
				<div class="modal-content">
					<div class="modal-header">
						<h5 class="modal-title" id="hostModalLabel">Host Details</h5>
						<button type="button" class="close" data-dismiss="modal" aria-label="Close">
							<span aria-hidden="true">&times;</span>
						</button>
					</div>
					<div class="modal-body">
						<ul class="list-group" style="word-break: break-word">
							<?php
								foreach($_SERVER as $key => $value) {
									if (!startsWith($key, 'PHP_AUTH')) {
										echo '<li class="list-group-item align-items-start">';
										echo '	<p class="mb-1 w-100"><strong>' . $key . '</strong></p>';
										echo '	<p class="mb-0">' . $value . '</p>';
										echo '</li>';
									}
								}
							?>
						</ul>
					</div>
					<div class="modal-footer">
						<button type="button" class="btn btn-secondary" data-dismiss="modal">Close</button>
					</div>
				</div>
			</div>
		</div>

		<!-- jQuery first, then Tether, then Bootstrap JS. -->
		<script src="vendors/jquery-3.1.1/jquery-3.1.1.slim.min.js" integrity="sha384-A7FZj7v+d/sdmMqp/nOQwliLvUsJfDHW+k9Omg/a/EheAdgtzNs3hpfag6Ed950n" crossorigin="anonymous"></script>
		<script src="vendors/tether-1.4.0/js/tether.min.js" integrity="sha256-DbVrJ6TqTEoNLNIMTka94bhP3K3c+qiSus9nZJRwz58=" crossorigin="anonymous"></script>
		<script src="vendors/bootstrap-4.0.0-alpha.6/js/bootstrap.min.js" integrity="sha384-vBWWzlZJ8ea9aCX4pEW3rVHjgjt7zpkNpZk+02D9phzyeVkE+jo0ieGizqPLForn" crossorigin="anonymous"></script>
	</body>
</html>