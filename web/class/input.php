<?php

class Input
{
	private $_post;
	private $_get;
	private $_session;
	private $_server;

	public function __construct()
	{
		$this->_post = $_POST;
		$this->_get = $_GET;
		$this->_session = $_SESSION;
		$this->_server = $_SERVER;
	}

	public function post($key = null, $default = null)
	{
		return $this->checkGlobal($this->_post, $key, $default);
	}

	public function get($key = null, $default = null)
	{
		return $this->checkGlobal($this->_get, $key, $default);
	}

	public function sessionGet($key = null, $default = null)
	{
		return $this->checkGlobal($this->_session, $key, $default);
	}
	
	public function sessionSet($key = null, $value = "")
	{
		if ($key) {
			return $this->_session[$key] = $value;
		}
		return false;
	}
	
	public function server($key = null, $default = null)
	{
		return $this->checkGlobal($this->_server, $key, $default);
	}

	private function checkGlobal($global, $key = null, $default = null)
	{
		if ($key) {
			if (isset($global[$key])) {
				return $global[$key];
			} else {
				return $default ?: null;
			}
		}
		return $global;
	}
}

?>