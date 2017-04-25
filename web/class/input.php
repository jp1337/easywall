<?php

class Input
{
	private $privatePost;
	private $privateGet;
	private $privateSession;
	private $privateServer;

	public function __construct()
	{
		$this->privatePost = $_POST;
		$this->privateGet = $_GET;
		$this->privateSession = $_SESSION;
		$this->privateServer = $_SERVER;
	}

	public function post($key = null, $default = null)
	{
		return $this->checkGlobal($this->privatePost, $key, $default);
	}

	public function get($key = null, $default = null)
	{
		return $this->checkGlobal($this->privateGet, $key, $default);
	}

	public function sessionGet($key = null, $default = null)
	{
		return $this->checkGlobal($this->privateSession, $key, $default);
	}
	
	public function sessionSet($key = null, $value = "")
	{
		if ($key) {
			$_SESSION[$key] = $value;
			$this->privateSession[$key] = $value;
			return true;
		}
		return false;
	}
	
	public function server($key = null, $default = null)
	{
		return $this->checkGlobal($this->privateServer, $key, $default);
	}

	private function checkGlobal($global, $key = null, $default = null)
	{
		if ($key) {
			if (isset($global[$key])) {
				return $global[$key];
			}
			return $default ?: null;
		}
		return $global;
	}
}

?>