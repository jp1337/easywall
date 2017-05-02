<?php
	session_start();

	require('class/input.php');
	$input = new Input();

	require('class/config.php');
	$config = new Config();
	
	require('class/functions.php');
	
	if ($config->getValue('WEBDEBUG') === "true") {
		ini_set('display_errors', 1);
		ini_set('display_startup_errors', 1);
		error_reporting(E_ALL);
		require( 'bower_components/PHP-Error/src/php_error.php' );
		\php_error\reportErrors();
	}
?>
<!DOCTYPE html>
<html lang="en">
	<head>
		<!-- Required meta tags -->
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
		<link rel="icon" href="favicon.png">
		
		<title>EasyWall</title>

		<!-- Bootstrap core CSS -->
		<link rel="stylesheet" href="bower_components/bootstrap/dist/css/bootstrap.min.css" integrity="sha384-rwoIResjU2yc3z8GV/NPeZWAv56rSmLldC3R/AZzGRnGxQQKnKkoFVhFQhNUwEyJ">
		
		<!-- Bootstrap Toggle CSS -->
		<link rel="stylesheet" href="bower_components/bootstrap-toggle/css/bootstrap-toggle.min.css" integrity="sha-256-rDWX6XrmRttWyVBePhmrpHnnZ1EPmM6WQRQl6h0h7J8=">
		
		<!-- Font Awesome CSS -->
		<link rel="stylesheet" href="bower_components/font-awesome/css/font-awesome.min.css" integrity="sha256-eZrrJcwDc/3uDhsdt61sL2oOBY362qM3lon1gyExkL0=">
		
		<!-- EasyWall CSS -->
		<link rel="stylesheet" href="css/easywall.css" integrity="sha256-Z29OgkmN607jVmSig90OXePgoW5V+ievEynMbw438Rs=">
	</head>
	<body>
		<nav class="navbar navbar-toggleable-md navbar-inverse bg-inverse fixed-top">
			<button class="navbar-toggler navbar-toggler-right" type="button" data-toggle="collapse" data-target="#topNavBar" aria-controls="topNavBar" aria-expanded="false" aria-label="Toggle navigation">
				<span class="navbar-toggler-icon"></span>
			</button>
			
			<a class="navbar-brand" href="index.php">
				<i class="fa fa-shield" aria-hidden="true"></i>&nbsp;EasyWall
			</a>
			
			<div class="collapse navbar-collapse" id="topNavBar">
				<ul class="navbar-nav mr-auto">
					<li class="nav-item <?php if (getReqUri() == "index.php" || getReqUri() == "") { echo "active"; } ?>">
						<a class="nav-link btn" href="index.php">Home</a>
					</li>
					<li class="nav-item <?php if (getReqUri() == "options.php") { echo "active"; } ?>">
						<a class="nav-link btn" href="options.php">Options</a>
					</li>
					<li class="nav-item <?php if (getReqUri() == "blacklist.php") { echo "active"; } ?>">
						<a class="nav-link btn" href="blacklist.php">Blacklist</a>
					</li>
					<li class="nav-item <?php if (getReqUri() == "whitelist.php") { echo "active"; } ?>">
						<a class="nav-link btn" href="whitelist.php">Whitelist</a>
					</li>
					<li class="nav-item <?php if (getReqUri() == "ports.php") { echo "active"; } ?>">
						<a class="nav-link btn" href="ports.php">Ports</a>
					</li>
					<li class="nav-item <?php if (getReqUri() == "apply.php") { echo "active"; } ?>">
						<a class="nav-link btn" href="apply.php">Apply</a>
					</li>
				</ul>
				
				<ul class="navbar-nav navbar-right">
					<li class="nav-item">
						<a class="nav-link btn" title="EasyWall on GitHub" href="https://kingjp.github.io/EasyWall/" target="_blank">
							<i class="fa fa-github" aria-hidden="true"></i>&nbsp;
							<span class="hidden-sm-down">GitHub</span>
							<span class="hidden-lg-up">EasyWall on GitHub</span>
						</a>
					</li>
					<li class="nav-item">
						<a class="nav-link btn" title="Need help?" href="#" data-toggle="modal" data-target="#helpModal">
							<i class="fa fa-support" aria-hidden="true"></i>&nbsp;
							<span class="hidden-sm-down">Help</span>
							<span class="hidden-lg-up">Need help?</span>
						</a>
					</li>
					<li class="nav-item">
						<a class="nav-link btn" title="Version and Update" href="#" data-toggle="modal" data-target="#versionModal">
							<i class="fa fa-cloud-download" aria-hidden="true"></i>&nbsp;
							<span class="hidden-sm-down">Update</span>
							<span class="hidden-lg-up">Version and Update</span>
							<?php
								if (version_compare(getCurrentVersion(), getLatestVersion()) < 0) {
									echo '<span class="badge badge-pill badge-warning">!</span>';
								}
							?>
						</a>
					</li>
					<li class="nav-item">
						<a class="nav-link btn" title="Host Information" href="#" data-toggle="modal" data-target="#hostModal">
							<i class="fa fa-server" aria-hidden="true"></i>&nbsp;
							<span class="hidden-sm-down">Host</span>
							<span class="hidden-lg-up">Host Information</span>
						</a>
					</li>
				</ul>
			</div>
		</nav>
