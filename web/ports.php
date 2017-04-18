<?php

require('pages/head.php');

require('class/rules.php');
$tcp = new Rules($config->getValue('TCP'));
$udp = new Rules($config->getValue('UDP'));

if ($_SERVER['REQUEST_METHOD']=='POST')
{
	$saved = false;
	$mode = "";
	
	print_r($_POST);
	
	if (isset($_POST['tcpudp'])) {
		$mode = $_POST['tcpudp'];
		
		if ($mode == 'tcp') {
			$port = $_POST['port'];
			
			if (isset($_POST['ssh'])) {
				$port .= ';ssh';
			}
		
			$saved = $tcp->add($port);
		} else if ($mode == 'udp') {
			$saved = $udp->add($_POST['port']);
		}
	}
	
	if (isset($_POST['remove'])) {
		$mode = $_POST['remove'];
		unset($_POST['remove']);
		
		if ($mode == 'tcp') {
			foreach ($_POST as $key => $value) {
				$saved = $tcp->remove($key);
			}
		} else if ($mode == 'udp') {
			foreach ($_POST as $key => $value) {
				$saved = $udp->remove($key);
			}
		}
		

	}
}

require('pages/ports.php');

require('pages/footer.php');

?>