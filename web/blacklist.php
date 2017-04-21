<?php

require('pages/head.php');

require('class/rules.php');
$rules = new Rules($config->getValue('BLACKLIST'));

if ($_SERVER['REQUEST_METHOD']=='POST')
{
	$saved = false;
	
	if (isset($_POST['ipadr'])) {
		$saved = $rules->add($_POST['ipadr']);
	}
	else
	{
		foreach ($_POST as $key => $value) {
			$saved = $rules->remove(str_replace('_', '.', $key));
		}
	}
}

require('pages/blacklist.php');

require('pages/footer.php');

?>