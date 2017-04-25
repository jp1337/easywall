<?php

class Cache
{
	public function __construct() {}
	
	public function getValue($key) {
		if (isset($_SESSION[$key])) {
			if ($_SESSION[$key] != null) {
				return $_SESSION[$key];
			}
		}
		return false;
    }
	
	public function setValue($key, $value) {
		return $_SESSION[$key] = $value;
	}
}

?>