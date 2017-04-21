<?php
	require('class/config.php');
	$config = new Config();
	$req_uri = basename($_SERVER['REQUEST_URI']);
?>
<!DOCTYPE html>
<html lang="en">
	<head>
		<!-- Required meta tags -->
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
		<link rel="icon" href="favicon.png">
		
		<title>EasyWall - Home</title>

		<!-- Bootstrap core CSS -->
		<link rel="stylesheet" href="vendors/bootstrap-4.0.0-alpha.6/css/bootstrap.min.css" integrity="sha384-rwoIResjU2yc3z8GV/NPeZWAv56rSmLldC3R/AZzGRnGxQQKnKkoFVhFQhNUwEyJ" crossorigin="anonymous">
		
		<!-- Font Awesome CSS -->
		<link rel="stylesheet" href="vendors/font-awesome-4.7.0/css/font-awesome.min.css" integrity="sha256-eZrrJcwDc/3uDhsdt61sL2oOBY362qM3lon1gyExkL0=" crossorigin="anonymous">
		
		<!-- EasyWall CSS -->
		<!--<link rel="stylesheet" href="css/easywall.css" integrity="sha256-tT7PVk4pTqLO82Am+XD7hzRQy7jCCkBm/bluNTNVzco=" crossorigin="anonymous">-->
		<link rel="stylesheet" href="css/easywall.css">
	</head>
	<body>
		<nav class="navbar navbar-toggleable-md navbar-inverse bg-inverse fixed-top">
			<button class="navbar-toggler navbar-toggler-right" type="button" data-toggle="collapse" data-target="#topNavBar" aria-controls="topNavBar" aria-expanded="false" aria-label="Toggle navigation">
				<span class="navbar-toggler-icon"></span>
			</button>
			<a class="navbar-brand" href="index.php">
				<i class="fa fa-lock" aria-hidden="true"></i>&nbsp;EasyWall
			</a>
			<div class="collapse navbar-collapse" id="topNavBar">
				<ul class="navbar-nav mr-auto">
					<li class="nav-item <?php if ($req_uri == "index.php") { echo "active"; } ?>">
						<a class="nav-link btn" href="index.php">Home</a>
					</li>
					<li class="nav-item <?php if ($req_uri == "options.php") { echo "active"; } ?>">
						<a class="nav-link btn" href="options.php">Options</a>
					</li>
					<li class="nav-item <?php if ($req_uri == "blacklist.php") { echo "active"; } ?>">
						<a class="nav-link btn" href="blacklist.php">Blacklist</a>
					</li>
					<li class="nav-item <?php if ($req_uri == "whitelist.php") { echo "active"; } ?>">
						<a class="nav-link btn" href="whitelist.php">Whitelist</a>
					</li>
					<li class="nav-item <?php if ($req_uri == "ports.php") { echo "active"; } ?>">
						<a class="nav-link btn" href="ports.php">Ports</a>
					</li>
					<li class="nav-item <?php if ($req_uri == "apply.php") { echo "active"; } ?>">
						<a class="nav-link btn" href="apply.php">Apply</a>
					</li>
				</ul>
				
				<ul class="navbar-nav navbar-right">
					<li class="nav-item">
						<a class="nav-link btn" title="EasyWall on GitHub" href="https://kingjp.github.io/EasyWall/" target="_blank">
							<i class="fa fa-github" aria-hidden="true"></i>
							<span class="hidden-lg-up">GitHub</span>
						</a>
					</li>
					<li class="nav-item">
						<a class="nav-link btn" title="Need help?" href="#" data-toggle="modal" data-target="#helpModal">
							<i class="fa fa-question" aria-hidden="true"></i>
							<span class="hidden-lg-up">Help</span>
						</a>
					</li>
					<li class="nav-item">
						<a class="nav-link btn" title="Show Version" href="#" data-toggle="modal" data-target="#versionModal">
							<i class="fa fa-code-fork" aria-hidden="true"></i>
							<span class="hidden-lg-up">Version</span>
						</a>
					</li>
					<li class="nav-item">
						<a class="nav-link btn" title="Show Host Details" href="#" data-toggle="modal" data-target="#hostModal">
							<i class="fa fa-server" aria-hidden="true"></i>
							<span class="hidden-lg-up">Hostname</span>
						</a>
					</li>
				</ul>
				
			</div>
		</nav>