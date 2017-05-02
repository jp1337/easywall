<?php

require('pages/head.php');

if ($_SERVER['REQUEST_METHOD']=='POST')
{
	$saved = false;
	
	foreach ($_POST as $key => $value) {
		if ($value === "on")
			$value = "true";
		
		$saved = $config->setValue($key, $value);
	}
}

require('pages/options.php');

require('pages/footer.php');

?>

