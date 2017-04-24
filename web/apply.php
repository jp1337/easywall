<?php

require('pages/head.php');

if ($_SERVER['REQUEST_METHOD']=='POST')
{
	if (isset($_POST['apply_step1'])) {
		$config->setValue('APPLIED', 'false');
		exec('../iptables/apply > /dev/null &');
		exec('../iptables/timer > /dev/null &');
		touch('../rules/.applied');
	}
	if (isset($_POST['apply_step2'])) {
		$config->setValue('APPLIED', 'true');
	}
}

require('pages/apply.php');

require('pages/footer.php');

?>
